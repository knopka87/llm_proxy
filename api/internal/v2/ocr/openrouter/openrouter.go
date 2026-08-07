// Package openrouter реализует движок LLM через OpenRouter API (openrouter.ai).
//
// OpenRouter — прокси-агрегатор 300+ моделей с единым Chat Completions API.
// Модель для каждого шага задаётся через переменные окружения, без изменения кода.
//
// Отличия от OpenAI Responses API:
//   - Endpoint: /api/v1/chat/completions (не /v1/responses)
//   - Формат сообщений: messages[] (не input[])
//   - Structured output: response_format (не text.format)
//   - Ответ: choices[0].message.content
//   - Usage: usage.prompt_tokens / completion_tokens
package openrouter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v2/ocr/types"
	"llm-proxy/api/internal/v2/tmplrouter"
)

const (
	apiVersion   = "v2"
	promptSource = "gpt" // промпты те же, провайдер-агностичные
	baseURL      = "https://openrouter.ai/api/v1/chat/completions"
)

// StepModels хранит модели для каждого шага пайплайна.
// Все значения задаются через переменные окружения; ни одна не захардкожена.
type StepModels struct {
	Detect   string // OPENROUTER_DETECT_MODEL
	Parse    string // OPENROUTER_PARSE_MODEL
	Hint     string // OPENROUTER_HINT_MODEL
	Check    string // OPENROUTER_CHECK_MODEL
	Analogue string // OPENROUTER_ANALOGUE_MODEL
}

type Engine struct {
	apiKey     string
	models     StepModels
	httpc      *http.Client
	tmplRouter *tmplrouter.Router
}

// SetTemplateRouter injects the pedagogical template router.
// Call after New() before serving requests.
func (e *Engine) SetTemplateRouter(r *tmplrouter.Router) {
	e.tmplRouter = r
}

func New(apiKey string, models StepModels) *Engine {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
	}
	return &Engine{
		apiKey: strings.TrimSpace(apiKey),
		models: models,
		httpc:  &http.Client{Timeout: 0, Transport: tr},
	}
}

func (e *Engine) Name() string { return "openrouter" }

// ─── DETECT ───────────────────────────────────────────────────────────────────

func (e *Engine) Detect(ctx context.Context, in types.DetectRequest) (types.DetectResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("detect", 0)
	if err != nil {
		return types.DetectResponse{}, nil, fmt.Errorf("openrouter detect: %w", err)
	}
	userPrompt, _ := util.LoadUserPrompt("detect", promptSource, apiVersion, "detect")
	if strings.TrimSpace(userPrompt) == "" {
		userPrompt = "Верни ТОЛЬКО JSON по detect.schema v2.2.2."
	}

	imgBytes, mime, err := decodeImage(in.Image)
	if err != nil {
		return types.DetectResponse{}, nil, fmt.Errorf("openrouter detect: %w", err)
	}

	messages := []message{
		systemMsg(system),
		userMsgWithImage(userPrompt, mime, imgBytes),
	}

	var out types.DetectResponse
	stats, err := e.call(ctx, e.models.Detect, "detect", messages, schemaJSON, &out)
	return out, stats, err
}

// ─── PARSE ────────────────────────────────────────────────────────────────────

func (e *Engine) Parse(ctx context.Context, in types.ParseRequest) (types.ParseResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("parse", int(in.Grade))
	if err != nil {
		return types.ParseResponse{}, nil, fmt.Errorf("openrouter parse: %w", err)
	}

	imgBytes, mime, err := decodeImage(in.Image)
	if err != nil {
		return types.ParseResponse{}, nil, fmt.Errorf("openrouter parse: %w", err)
	}

	ctxData := map[string]any{
		"task_id":            in.TaskId,
		"grade":              in.Grade,
		"subject_candidate":  in.SubjectCandidate,
		"subject_confidence": in.SubjectConfidence,
	}
	if in.Locale != "" {
		ctxData["locale"] = in.Locale
	}
	ctxJSON, _ := json.Marshal(ctxData)

	userPrompt, _ := util.LoadUserPrompt("parse", promptSource, apiVersion, "parse")
	if strings.TrimSpace(userPrompt) == "" {
		userPrompt = "Верни ТОЛЬКО JSON по parse.schema v2.1.1."
	}
	userText := userPrompt + "\nINPUT_CONTEXT:\n" + string(ctxJSON)

	messages := []message{
		systemMsg(system),
		userMsgWithImage(userText, mime, imgBytes),
	}

	var pr types.ParseResponse
	stats, err := e.call(ctx, e.models.Parse, "parse", messages, schemaJSON, &pr)
	if err != nil {
		return types.ParseResponse{}, stats, err
	}
	stats.PromptHash = promptHash(system, userText, schemaJSON, e.models.Parse)
	if pr.ValidateItems() > 0 {
		retryMessages := appendCorrection(messages,
			"В предыдущем результате final_answer противоречил последнему выводу solution_steps. "+
				"Реши задачу заново независимо, проверь обратным действием и верни согласованный JSON. "+
				"Если подтвердить ответ нельзя, верни final_answer=null и unsafe_to_finalize_answer=true.")
		var retried types.ParseResponse
		retryStats, retryErr := e.call(ctx, e.models.Parse, "parse", retryMessages, schemaJSON, &retried)
		stats.Add(retryStats)
		if retryErr == nil {
			retried.ValidateItems()
			pr = retried
		} else {
			log.Printf("[openrouter] parse semantic retry failed: %v", retryErr)
		}
	}
	return pr, stats, nil
}

// ─── HINT ─────────────────────────────────────────────────────────────────────

func (e *Engine) Hint(ctx context.Context, in types.HintRequest) (types.HintResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("hint", in.Task.Grade)
	if err != nil {
		return types.HintResponse{}, nil, fmt.Errorf("openrouter hint: %w", err)
	}

	// Условная дозагрузка педагогического блока по типу задачи (шаг 09).
	// Каждый тип задачи имеет отдельный файл hint.advanced_{task_type}.system.txt
	// с расширенной педагогикой (частые ошибки, паттерны L1/L2/L3 для типа).
	var advancedTopics []string
	if len(in.Items) > 0 {
		taskType := types.NormalizeTaskType(in.Items[0].PedKeys.TaskType)
		in.Items[0].PedKeys.TaskType = taskType
		if block := types.HintAdvancedPromptBlock(taskType); block != "" {
			if advanced, aerr := loadHintAdvancedBlock(in.Task.Grade, block); aerr == nil && strings.TrimSpace(advanced) != "" {
				system = system + "\n\n" + advanced
				advancedTopics = append(advancedTopics, block)
			} else if aerr != nil {
				log.Printf("[openrouter] hint advanced block %q not loaded: %v", block, aerr)
			}
		}
	}

	// Select pedagogical template from the router (replaces child_bot routing).
	// Template field is deprecated: we ignore any value coming from child_bot and
	// always resolve the template here from task_text_clean + visual_kinds.
	if e.tmplRouter != nil && in.Task.TaskTextClean != "" {
		visualKinds := make([]string, 0, len(in.Task.VisualFacts))
		for _, vf := range in.Task.VisualFacts {
			if vf.Kind != "" {
				visualKinds = append(visualKinds, vf.Kind)
			}
		}
		if profile := e.tmplRouter.RouteProfile(in.Task.TaskTextClean, visualKinds); profile != "" {
			in.Template = profile
		}
	}

	inJSON, _ := json.Marshal(in)
	// Загружаем user-шаблон с учётом класса
	userTemplate, _ := loadHintUserPrompt(in.Task.Grade)
	var userText string
	if strings.Contains(userTemplate, "{{PARSE_OUTPUT_JSON}}") {
		userText = strings.ReplaceAll(userTemplate, "{{PARSE_OUTPUT_JSON}}", string(inJSON))
	} else {
		userText = "PARSE_OUTPUT_JSON:\n" + string(inJSON)
		if strings.TrimSpace(userTemplate) != "" {
			userText = userTemplate + "\n\n" + userText
		}
	}

	// Явно выносим grade в начало для лучшей адаптации языка моделью.
	grade := in.Task.Grade
	if grade > 0 {
		userText = fmt.Sprintf("Класс ученика: %d (1–4).\n\n", grade) + userText
	}

	messages := []message{systemMsg(system), userMsgText(userText)}

	var hr types.HintResponse
	stats, err := e.call(ctx, e.models.Hint, "hint", messages, schemaJSON, &hr)
	if stats != nil {
		stats.PromptHash = promptHash(system, userText, schemaJSON, e.models.Hint)
		stats.PromptBlocks = strings.Join(advancedTopics, ",")
	}
	if err == nil {
		if validationErr := hr.ValidateAgainstRequest(in); validationErr != nil {
			retryMessages := appendCorrection(messages,
				"Предыдущая подсказка не прошла semantic validation: "+validationErr.Error()+". "+
					"Покрой каждый шаг solution_internal.plan, выставь точный plan_coverage и верни все обязательные уровни L1/L2/L3.")
			var retried types.HintResponse
			retryStats, retryErr := e.call(ctx, e.models.Hint, "hint", retryMessages, schemaJSON, &retried)
			stats.Add(retryStats)
			if retryErr != nil {
				return types.HintResponse{}, stats, fmt.Errorf("openrouter hint semantic retry: %w", retryErr)
			}
			if retryValidationErr := retried.ValidateAgainstRequest(in); retryValidationErr != nil {
				return types.HintResponse{}, stats, fmt.Errorf("openrouter hint semantic validation: %w", retryValidationErr)
			}
			hr = retried
		}
	}
	// Устанавливаем метрики после вызова (LLM ответ перезаписывает поля)
	if grade > 0 {
		hr.PromptVersion = fmt.Sprintf("%d_class", grade)
	}
	if len(advancedTopics) > 0 {
		hr.AdvancedTopics = advancedTopics
	}
	return hr, stats, err
}

// ─── CHECK ────────────────────────────────────────────────────────────────────

func (e *Engine) CheckSolution(ctx context.Context, in types.CheckRequest) (types.CheckResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("check", int(in.Student.Grade))
	if err != nil {
		return types.CheckResponse{}, nil, fmt.Errorf("openrouter check: %w", err)
	}

	// Подставляем grade-специфичную секцию feedback
	if gradeSection, serr := loadCheckFeedbackSection(int(in.Student.Grade)); serr == nil {
		system = strings.ReplaceAll(system, "{{GRADE_FEEDBACK_SECTION}}", gradeSection)
	}

	// Композиция дополнительных блоков промпта
	system, checkBlocks := composeCheckBlocks(system, in.TaskStruct)

	imgBytes, mime, err := decodeImage(in.Image)
	if err != nil {
		return types.CheckResponse{}, nil, fmt.Errorf("openrouter check: %w", err)
	}

	reqForJSON := struct {
		TaskStruct       types.TaskStructCheck `json:"task_struct"`
		RawTaskText      string                `json:"raw_task_text"`
		Student          types.StudentCheck    `json:"student"`
		PhotoQualityHint string                `json:"photo_quality_hint"`
		AnswerImageRef   string                `json:"answer_image_ref"`
	}{
		TaskStruct:       in.TaskStruct,
		RawTaskText:      in.RawTaskText,
		Student:          in.Student,
		PhotoQualityHint: in.PhotoQualityHint,
		AnswerImageRef:   "attached_image",
	}
	reqJSON, _ := json.Marshal(reqForJSON)

	userTemplate, _ := util.LoadUserPrompt("check", promptSource, apiVersion, "check")
	var userText string
	if strings.Contains(userTemplate, "{{request_json}}") {
		userText = strings.ReplaceAll(userTemplate, "{{request_json}}", string(reqJSON))
	} else {
		userText = "INPUT_JSON:\n" + string(reqJSON)
		if strings.TrimSpace(userTemplate) != "" {
			userText = userTemplate + "\n\n" + userText
		}
	}

	messages := []message{systemMsg(system), userMsgWithImage(userText, mime, imgBytes)}

	var cr types.CheckResponse
	stats, err := e.call(ctx, e.models.Check, "check", messages, schemaJSON, &cr)
	if err != nil {
		return types.CheckResponse{}, stats, err
	}
	stats.PromptHash = promptHash(system, userText, schemaJSON, e.models.Check)
	stats.PromptBlocks = strings.Join(checkBlocks, ",")
	cr.NormalizeDecision()
	cr.SetIsCorrectFromDecision()
	if validationErr := cr.ValidateSemantics(in); validationErr != nil {
		retryMessages := appendCorrection(messages,
			"Предыдущая проверка не прошла semantic validation: "+validationErr.Error()+". "+
				"Независимо реши задачу до сравнения с ответом ученика, подтверди эталон вторым способом, "+
				"заполни все verification-поля и visual_evidence для визуальной задачи.")
		var retried types.CheckResponse
		retryStats, retryErr := e.call(ctx, e.models.Check, "check", retryMessages, schemaJSON, &retried)
		stats.Add(retryStats)
		if retryErr == nil {
			retried.NormalizeDecision()
			retried.SetIsCorrectFromDecision()
			if retryValidationErr := retried.ValidateSemantics(in); retryValidationErr == nil {
				return retried, stats, nil
			} else {
				log.Printf("[openrouter] check semantic retry invalid: %v", retryValidationErr)
			}
		} else {
			log.Printf("[openrouter] check semantic retry failed: %v", retryErr)
		}
		fallback := types.ConservativeCheckResponse()
		fallback.SetIsCorrectFromDecision()
		return fallback, stats, nil
	}
	return cr, stats, err
}

// ─── ANALOGUE ─────────────────────────────────────────────────────────────────

func (e *Engine) AnalogueSolution(ctx context.Context, in types.AnalogueRequest) (types.AnalogueResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("analogue", 0)
	if err != nil {
		return types.AnalogueResponse{}, nil, fmt.Errorf("openrouter analogue: %w", err)
	}

	inJSON, _ := json.Marshal(in)
	userTemplate, _ := util.LoadUserPrompt("analogue", promptSource, apiVersion, "analogue")
	var userText string
	if strings.TrimSpace(userTemplate) != "" {
		userText = userTemplate + "\n\nINPUT_JSON:\n" + string(inJSON)
	} else {
		userText = "INPUT_JSON:\n" + string(inJSON)
	}

	messages := []message{systemMsg(system), userMsgText(userText)}

	var ar types.AnalogueResponse
	stats, err := e.call(ctx, e.models.Analogue, "analogue", messages, schemaJSON, &ar)
	return ar, stats, err
}

// ─── HTTP + парсинг ───────────────────────────────────────────────────────────

// chatRequest — тело запроса Chat Completions API (используется OpenRouter и OpenAI).
type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []message       `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Temperature    *float64        `json:"temperature,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string или []contentPart
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type responseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *jsonSchema `json:"json_schema,omitempty"`
}

type jsonSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

// chatResponse — ответ Chat Completions API.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		Cost             float64 `json:"cost"`
	} `json:"usage"`
}

// isGeminiModel проверяет, является ли модель Google Gemini.
// Gemini использует constrained decoding для json_schema, который не справляется
// со сложными схемами (много enum-значений, глубокая вложенность).
// Для таких моделей используем json_object — мягкий JSON-режим без компиляции схемы.
func isGeminiModel(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "gemini") || strings.Contains(m, "google/")
}

// tempPtr возвращает указатель на float64 — для заполнения Temperature в chatRequest.
func tempPtr(t float64) *float64 { return &t }

func (e *Engine) call(
	ctx context.Context,
	model, op string,
	messages []message,
	schemaJSON string,
	dst any,
) (*types.LLMStats, error) {
	if e.apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY not set")
	}
	if model == "" {
		return nil, fmt.Errorf("openrouter %s: model not configured (set OPENROUTER_%s_MODEL)", op, strings.ToUpper(op))
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err == nil {
		util.FixJSONSchemaStrict(schema)
	}

	// Выбираем режим structured output в зависимости от модели:
	//   OpenAI и совместимые → json_schema (strict) — точное следование схеме
	//   Gemini → json_object — мягкий JSON-режим без компиляции схемы в конечный автомат.
	//     Gemini использует constrained decoding, и сложные схемы (54 enum + вложенные
	//     объекты) вызывают ошибку "too many states for serving". Схема уже есть в
	//     system-промпте, поэтому модель всё равно вернёт правильную структуру.
	var rf *responseFormat
	if isGeminiModel(model) {
		rf = &responseFormat{Type: "json_object"}
	} else {
		rf = &responseFormat{
			Type: "json_schema",
			JSONSchema: &jsonSchema{
				Name:   op,
				Strict: true,
				Schema: schema,
			},
		}
	}

	// Температура зависит от шага: низкая для детерминированных операций,
	// умеренная для творческих (подсказки).
	var temp *float64
	switch op {
	case "check", "check_ru", "parse", "parse_ru", "detect":
		temp = tempPtr(0.1) // максимальный детерминизм
	case "hint", "hint_ru":
		temp = tempPtr(0.2) // вариативность, но без случайности
	}

	reqBody := chatRequest{
		Model:          model,
		Messages:       messages,
		ResponseFormat: rf,
		Temperature:    temp,
	}

	// Для Gemini добавляем явную инструкцию в конец user-сообщения,
	// чтобы модель вернула строго JSON без markdown-оберток.
	if isGeminiModel(model) && len(reqBody.Messages) > 0 {
		last := &reqBody.Messages[len(reqBody.Messages)-1]
		switch c := last.Content.(type) {
		case string:
			last.Content = c + "\n\nВЕРНИ ТОЛЬКО ВАЛИДНЫЙ JSON. БЕЗ markdown, без ```json, без пояснений."
		case []contentPart:
			if len(c) > 0 {
				c[0].Text += "\n\nВЕРНИ ТОЛЬКО ВАЛИДНЫЙ JSON. БЕЗ markdown, без ```json, без пояснений."
			}
		}
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openrouter %s: marshal request: %w", op, err)
	}
	start := time.Now()
	var raw []byte
	for attempt := 0; attempt < 2; attempt++ {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(payload))
		if requestErr != nil {
			return nil, fmt.Errorf("openrouter %s: create request: %w", op, requestErr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
		req.Header.Set("HTTP-Referer", "https://vk.obyasnyatel.ru")
		req.Header.Set("X-Title", "Объяснятель ДЗ")

		resp, callErr := e.httpc.Do(req)
		if callErr != nil {
			return nil, fmt.Errorf("openrouter %s: %w", op, callErr)
		}
		raw, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		closeErr := resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("openrouter %s: read response: %w", op, err)
		}
		if closeErr != nil {
			log.Printf("[openrouter] close response body: %v", closeErr)
		}
		if resp.StatusCode == http.StatusOK {
			break
		}
		retryable := resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if attempt == 0 && retryable {
			delay := 250 * time.Millisecond
			if retryAfter, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && retryAfter > 0 {
				delay = min(time.Duration(retryAfter)*time.Second, 2*time.Second)
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, fmt.Errorf("openrouter %s retry: %w", op, ctx.Err())
			case <-timer.C:
			}
			continue
		}
		return nil, fmt.Errorf("openrouter %s %d: %s", op, resp.StatusCode, truncate(raw, 512))
	}
	latencyMs := time.Since(start).Milliseconds()

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return nil, fmt.Errorf("openrouter %s: parse response: %w", op, err)
	}
	if len(cr.Choices) == 0 || strings.TrimSpace(cr.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("openrouter %s: empty response; body=%s", op, truncate(raw, 512))
	}

	stats := &types.LLMStats{
		InputTokens:  cr.Usage.PromptTokens,
		OutputTokens: cr.Usage.CompletionTokens,
		LatencyMs:    latencyMs,
		Model:        model,
		CostUSD:      cr.Usage.Cost,
	}

	text := util.StripCodeFences(strings.TrimSpace(cr.Choices[0].Message.Content))

	// Исправляем частую ошибку LLM: {} вместо [] для array-полей.
	// Gemini в json_object режиме делает это систематически,
	// но другие модели тоже иногда возвращают объект вместо массива.
	text = fixEmptyArrayFields(text)

	if err := json.Unmarshal([]byte(text), dst); err != nil {
		return stats, fmt.Errorf("openrouter %s: bad JSON: %w", op, err)
	}

	log.Printf("[openrouter] %s model=%s latency=%dms in=%d out=%d",
		op, model, latencyMs, stats.InputTokens, stats.OutputTokens)

	return stats, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func systemMsg(text string) message {
	return message{Role: "system", Content: text}
}

func userMsgText(text string) message {
	return message{Role: "user", Content: text}
}

func userMsgWithImage(text, mime string, imgBytes []byte) message {
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)
	return message{
		Role: "user",
		Content: []contentPart{
			{Type: "text", Text: text},
			{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
		},
	}
}

func appendCorrection(messages []message, correction string) []message {
	result := make([]message, 0, len(messages)+1)
	result = append(result, messages...)
	result = append(result, userMsgText("CORRECTION_REQUIRED:\n"+correction))
	return result
}

func promptHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%x", sum[:8])
}

func gradeSubdir(grade int) string {
	switch grade {
	case 1:
		return "1_class"
	case 2:
		return "2_class"
	case 3:
		return "3_class"
	case 4:
		return "4_class"
	default:
		return ""
	}
}

// loadHintUserPrompt загружает пользовательский шаблон для подсказок с учётом класса.
func loadHintUserPrompt(grade int) (string, error) {
	if subdir := gradeSubdir(grade); subdir != "" {
		if p, err := util.LoadUserPrompt("hint", promptSource, apiVersion, "hint", subdir); err == nil {
			return p, nil
		}
	}
	return util.LoadUserPrompt("hint", promptSource, apiVersion, "hint")
}

func loadHintAdvancedBlock(grade int, block string) (string, error) {
	name := "hint.advanced_" + block + ".system.txt"
	baseRoot := os.Getenv("PROMPT_DIR")
	if baseRoot == "" {
		baseRoot = filepath.Join("api", "internal")
	}
	if subdir := gradeSubdir(grade); subdir != "" {
		path := filepath.Join(baseRoot, apiVersion, "prompt", "hint", subdir, name)
		if data, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}
	path := filepath.Join(baseRoot, apiVersion, "prompt", "hint", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("load hint block %s: %w", block, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func loadPrompt(name string, grade int) (string, error) {
	// Для подсказок и проверки используем поддиректорию класса
	if name == "hint" || name == "hint_ru" || name == "check" || name == "check_ru" {
		if subdir := gradeSubdir(grade); subdir != "" {
			if p, err := util.LoadSystemPrompt(name, promptSource, apiVersion, name, subdir); err == nil {
				return p, nil
			}
		}
	}
	return util.LoadSystemPrompt(name, promptSource, apiVersion, name)
}

func loadSystemWithSchema(name string, grade int) (system, schemaJSON string, err error) {
	sys, err := loadPrompt(name, grade)
	if err != nil {
		return "", "", fmt.Errorf("load system prompt %q: %w", name, err)
	}
	schema, err := util.LoadPromptSchema(name, apiVersion)
	if err != nil {
		return "", "", fmt.Errorf("load schema %q: %w", name, err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return "", "", fmt.Errorf("marshal schema %q: %w", name, err)
	}
	return sys, string(raw), nil
}

func decodeImage(image string) ([]byte, string, error) {
	imgBytes, mimeFromDataURL, _ := util.DecodeBase64MaybeDataURL(image)
	if len(imgBytes) == 0 {
		raw, err := base64.StdEncoding.DecodeString(image)
		if err != nil || len(raw) == 0 {
			return nil, "", fmt.Errorf("invalid image base64")
		}
		imgBytes = raw
	}
	mime := util.PickMIME("", mimeFromDataURL, imgBytes)
	if mime == "application/octet-stream" {
		mime = "image/jpeg"
	}
	return imgBytes, mime, nil
}

// fixEmptyArrayFields исправляет два вида ошибок LLM в JSON:
//
//  1. Пустые массивы: {} → [] для известных array-полей
//  2. Объекты вместо массивов: {"key":"val"} → [] (если поле должно быть массивом)
//  3. Объекты вместо строк в plan/solution_steps:
//     LLM иногда возвращает [{...}, ...] вместо ["...", ...]
//     Каждый объект-элемент сериализуется в JSON-строку.
func fixEmptyArrayFields(s string) string {
	// Быстрая замена пустых {} → [] для array-полей
	arrayFields := []string{
		"visual_facts", "items", "plan", "solution_steps",
		"flags", "constraints", "labels", "issues", "hints", "error_spans", "buttons", "visual_evidence",
	}
	for _, f := range arrayFields {
		s = strings.ReplaceAll(s, `"`+f+`":{}`, `"`+f+`":[]`)
		s = strings.ReplaceAll(s, `"`+f+`": {}`, `"`+f+`": []`)
	}

	// Глубокая нормализация: объекты вместо массивов → пустые массивы,
	// объекты в string-массивах → строки
	var doc interface{}
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		return s
	}
	fixArrayFields(doc, arrayFields)
	fixStringArrayElements(doc)
	if fixed, err := json.Marshal(doc); err == nil {
		return string(fixed)
	}
	return s
}

// fixArrayFields заменяет объекты на пустые [] для полей, которые должны быть массивами.
func fixArrayFields(v interface{}, arrayFields []string) {
	fieldSet := make(map[string]bool, len(arrayFields))
	for _, f := range arrayFields {
		fieldSet[f] = true
	}
	switch node := v.(type) {
	case map[string]interface{}:
		for key, val := range node {
			if fieldSet[key] {
				if _, isObj := val.(map[string]interface{}); isObj {
					node[key] = []interface{}{}
				}
			} else {
				fixArrayFields(val, arrayFields)
			}
		}
	case []interface{}:
		for _, elem := range node {
			fixArrayFields(elem, arrayFields)
		}
	}
}

// fixStringArrayElements рекурсивно конвертирует объекты-элементы
// в массивах plan/solution_steps в JSON-строки.
func fixStringArrayElements(v interface{}) {
	stringArrayFields := map[string]bool{"plan": true, "solution_steps": true}
	switch node := v.(type) {
	case map[string]interface{}:
		for key, val := range node {
			if stringArrayFields[key] {
				if arr, ok := val.([]interface{}); ok {
					for i, elem := range arr {
						if _, isStr := elem.(string); !isStr {
							if b, err := json.Marshal(elem); err == nil {
								arr[i] = string(b)
							}
						}
					}
				}
			} else {
				fixStringArrayElements(val)
			}
		}
	case []interface{}:
		for _, elem := range node {
			fixStringArrayElements(elem)
		}
	}
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}

// ─── PARSE_RU ─────────────────────────────────────────────────────────────────

func (e *Engine) ParseRU(ctx context.Context, in types.ParseRURequest) (types.ParseRUResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("parse_ru", 0)
	if err != nil {
		return types.ParseRUResponse{}, nil, fmt.Errorf("openrouter parse_ru: %w", err)
	}

	imgBytes, mime, err := decodeImage(in.Image)
	if err != nil {
		return types.ParseRUResponse{}, nil, fmt.Errorf("openrouter parse_ru: %w", err)
	}

	in.Image = ""
	userPrompt, _ := util.LoadUserPrompt("parse_ru", promptSource, apiVersion, "parse")
	if strings.TrimSpace(userPrompt) == "" {
		userPrompt = "Верни ТОЛЬКО JSON по parse_ru.output.schema."
	}

	inJSON, _ := json.Marshal(in)
	userText := userPrompt + "\nINPUT_JSON:\n" + string(inJSON)

	messages := []message{
		systemMsg(system),
		userMsgWithImage(userText, mime, imgBytes),
	}

	var out types.ParseRUResponse
	stats, err := e.call(ctx, e.models.Parse, "parse_ru", messages, schemaJSON, &out)
	return out, stats, err
}

// ─── HINT_RU ─────────────────────────────────────────────────────────────────

func (e *Engine) HintRU(ctx context.Context, in types.HintRUCompactInput) (types.HintRUResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("hint_ru", 0)
	if err != nil {
		return types.HintRUResponse{}, nil, fmt.Errorf("openrouter hint_ru: %w", err)
	}

	inJSON, _ := json.Marshal(in)
	userPrompt, _ := util.LoadUserPrompt("hint_ru", promptSource, apiVersion, "hint")
	var userText string
	if strings.Contains(userPrompt, "COMPACT_INPUT:") {
		userText = strings.ReplaceAll(userPrompt, "COMPACT_INPUT:", "COMPACT_INPUT:\n"+string(inJSON))
	} else {
		userText = "COMPACT_INPUT:\n" + string(inJSON)
		if strings.TrimSpace(userPrompt) != "" {
			userText = userPrompt + "\n\n" + userText
		}
	}

	messages := []message{systemMsg(system), userMsgText(userText)}

	var out types.HintRUResponse
	stats, err := e.call(ctx, e.models.Hint, "hint_ru", messages, schemaJSON, &out)
	return out, stats, err
}

// ─── CHECK_RU ────────────────────────────────────────────────────────────

func (e *Engine) CheckRU(ctx context.Context, in types.CheckRUCompactInput) (types.CheckRUResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("check_ru", in.Grade)
	if err != nil {
		return types.CheckRUResponse{}, nil, fmt.Errorf("openrouter check_ru: %w", err)
	}

	// Подставляем grade-специфичную секцию feedback
	if gradeSection, serr := loadCheckRUFeedbackSection(in.Grade); serr == nil {
		system = strings.ReplaceAll(system, "{{GRADE_FEEDBACK_SECTION}}", gradeSection)
	}

	// Композиция дополнительных блоков промпта (RU: пока без дополнительных блоков)
	system = composeCheckRUBlocks(system)

	inJSON, _ := json.Marshal(in)
	userPrompt, _ := util.LoadUserPrompt("check_ru", promptSource, apiVersion, "check")
	var userText string
	if strings.Contains(userPrompt, "COMPACT_INPUT:") {
		userText = strings.ReplaceAll(userPrompt, "COMPACT_INPUT:", "COMPACT_INPUT:\n"+string(inJSON))
	} else {
		userText = "COMPACT_INPUT:\n" + string(inJSON)
		if strings.TrimSpace(userPrompt) != "" {
			userText = userPrompt + "\n\n" + userText
		}
	}

	messages := []message{systemMsg(system), userMsgText(userText)}

	var out types.CheckRUResponse
	stats, err := e.call(ctx, e.models.Check, "check_ru", messages, schemaJSON, &out)
	return out, stats, err
}

// ─── EMBED ────────────────────────────────────────────────────────────────────

// Embed генерирует эмбеддинги для батча входных строк.
// OpenRouter не предоставляет API для эмбеддингов, поэтому операция не реализована.
func (e *Engine) Embed(ctx context.Context, in types.EmbedRequest) (types.EmbedResponse, *types.LLMStats, error) {
	return types.EmbedResponse{}, nil, fmt.Errorf("embed: not supported by openrouter engine")
}

func loadCheckFeedbackSection(grade int) (string, error) {
	subdir := gradeSubdir(grade)
	if subdir == "" {
		return "", fmt.Errorf("unknown grade: %d", grade)
	}

	baseRoot := os.Getenv("PROMPT_DIR")
	if baseRoot == "" {
		baseRoot = filepath.Join("api", "internal")
	}
	p := filepath.Join(baseRoot, apiVersion, "prompt", "check", subdir, "check.feedback.txt")
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("load check feedback %s: %w", subdir, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func loadCheckRUFeedbackSection(grade int) (string, error) {
	subdir := gradeSubdir(grade)
	if subdir == "" {
		return "", fmt.Errorf("unknown grade: %d", grade)
	}

	baseRoot := os.Getenv("PROMPT_DIR")
	if baseRoot == "" {
		baseRoot = filepath.Join("api", "internal")
	}
	p := filepath.Join(baseRoot, apiVersion, "prompt", "check", subdir, "check_ru.feedback.txt")
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("load check_ru feedback %s: %w", subdir, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// composeCheckBlocks добавляет к базовому промпту условные блоки для проверки ответа.
// Загружает: advanced (по task_type), format (по формату), conditional (visual, high_risk, multiple_subtasks).
func composeCheckBlocks(system string, taskStruct types.TaskStructCheck) (string, []string) {
	var blocks []string
	var blockNames []string
	appendBlock := func(name, content string) {
		blocks = append(blocks, content)
		blockNames = append(blockNames, name)
	}

	loadCheckBlock := func(name string) (string, error) {
		return util.LoadSystemPrompt(name, promptSource, apiVersion, "check", name)
	}

	if len(taskStruct.Items) > 0 {
		item := taskStruct.Items[0]

		// Advanced блок по типу задачи
		if block := types.CheckAdvancedPromptBlock(item.PedKeys.TaskType); block != "" {
			advancedName := "check.advanced_" + block
			if advanced, aerr := loadCheckBlock(advancedName); aerr == nil && strings.TrimSpace(advanced) != "" {
				appendBlock(advancedName, advanced)
			} else if aerr != nil {
				log.Printf("[openrouter] check block %q not loaded: %v", advancedName, aerr)
			}
		}

		// Format блок по формату
		if item.PedKeys.Format != "" {
			formatName := "check.format_" + item.PedKeys.Format
			if format, ferr := loadCheckBlock(formatName); ferr == nil && strings.TrimSpace(format) != "" {
				appendBlock(formatName, format)
			}
		}
	}

	// Conditional блоки
	if isVisualTask(taskStruct) {
		if visual, verr := loadCheckBlock("check.visual"); verr == nil && strings.TrimSpace(visual) != "" {
			appendBlock("check.visual", visual)
		}
	}

	if hasMultipleSubtasks(taskStruct) {
		if multi, merr := loadCheckBlock("check.multiple_subtasks"); merr == nil && strings.TrimSpace(multi) != "" {
			appendBlock("check.multiple_subtasks", multi)
		}
	}

	// Verify блоки по типу задачи (verify_transforms, verify_age, verify_tables, verify_arithmetic)
	if len(taskStruct.Items) > 0 {
		verification := types.VerificationPromptBlock(taskStruct.Items[0].PedKeys.TaskType)
		if verification != "" {
			name := "check.verify_" + verification
			if v, err := loadCheckBlock(name); err == nil && strings.TrimSpace(v) != "" {
				appendBlock(name, v)
			} else if err != nil {
				log.Printf("[openrouter] check verification block %q not loaded: %v", name, err)
			}
		}
		if isHighRiskTask(taskStruct.Items[0]) {
			if v, err := loadCheckBlock("check.high_risk"); err == nil && strings.TrimSpace(v) != "" {
				appendBlock("check.high_risk", v)
			} else if err != nil {
				log.Printf("[openrouter] check high-risk block not loaded: %v", err)
			}
		}
	}

	if len(blocks) > 0 {
		system = system + "\n\n" + strings.Join(blocks, "\n\n")
	}
	return system, blockNames
}

func isVisualTask(taskStruct types.TaskStructCheck) bool {
	if taskStruct.VisualReasoning != nil && strings.TrimSpace(*taskStruct.VisualReasoning) != "" {
		return true
	}
	if len(taskStruct.VisualFacts) > 0 {
		return true
	}
	if len(taskStruct.Items) == 0 {
		return false
	}
	item := taskStruct.Items[0]
	if item.PedKeys.Format == "drawing" || item.PedKeys.Format == "diagram" || item.PedKeys.Format == "visual" {
		return true
	}
	return slicesContain(item.PedKeys.Constraints, "needs_visual")
}

func hasMultipleSubtasks(taskStruct types.TaskStructCheck) bool {
	if len(taskStruct.Items) > 1 {
		return true
	}
	return len(taskStruct.Items) == 1 && slicesContain(taskStruct.Items[0].PedKeys.Constraints, "has_subparts")
}

func isHighRiskTask(item types.ParseItem) bool {
	taskType := types.NormalizeTaskType(item.PedKeys.TaskType)
	return taskType == types.TaskTypeLogic || taskType == types.TaskTypePatternsLogic ||
		taskType == types.TaskTypeSetsLogic || slicesContain(item.PedKeys.Constraints, "high_risk")
}

func slicesContain(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// composeCheckRUBlocks добавляет к базовому промпту условные блоки для проверки РКИ.
// RU prompt уже самодостаточен — функция-заглушка для будущих расширений.
func composeCheckRUBlocks(system string) string {
	return system
}
