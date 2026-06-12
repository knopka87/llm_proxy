package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v2/ocr/types"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	apiVersion   = "v2"
	promptSource = "gpt" // Переиспользуем промпты от gpt — содержимое одинаковое
)

// Engine вызывает Gemini API для всех шагов пайплайна.
// Каждый шаг использует оптимальную модель:
//   - Detect  → detectModel (gemini-2.0-flash-lite: простая классификация, дешевле)
//   - Parse   → parseModel  (gemini-2.5-flash: OCR рукописи + русский язык)
//   - Hint    → parseModel  (text-only, педагогический текст)
//   - Check   → parseModel  (vision + математическое рассуждение)
//   - Analogue → parseModel (text-only, генерация задания)
type Engine struct {
	apiKey      string
	detectModel string
	parseModel  string
}

func New(apiKey, detectModel, parseModel string) *Engine {
	if detectModel == "" {
		detectModel = "gemini-2.0-flash-lite"
	}
	if parseModel == "" {
		parseModel = "gemini-2.5-flash"
	}
	return &Engine{
		apiKey:      strings.TrimSpace(apiKey),
		detectModel: detectModel,
		parseModel:  parseModel,
	}
}

func (e *Engine) Name() string { return "gemini" }

// ─── DETECT ───────────────────────────────────────────────────────────────────

// Detect оценивает качество фото и определяет учебный предмет.
// Модель: detectModel (gemini-2.0-flash-lite) — задача простая, нужна скорость.
func (e *Engine) Detect(ctx context.Context, in types.DetectRequest) (types.DetectResponse, error) {
	if e.apiKey == "" {
		return types.DetectResponse{}, fmt.Errorf("GEMINI_API_KEY not set")
	}

	system, schema, err := loadSystemWithSchema("detect")
	if err != nil {
		return types.DetectResponse{}, fmt.Errorf("gemini detect: %w", err)
	}

	userPrompt, _ := util.LoadUserPrompt("detect", promptSource, apiVersion)
	if strings.TrimSpace(userPrompt) == "" {
		userPrompt = "Верни ТОЛЬКО JSON по detect.schema v2.2.2."
	}

	imgBytes, mime, err := decodeImage(in.Image)
	if err != nil {
		return types.DetectResponse{}, fmt.Errorf("gemini detect: %w", err)
	}

	parts := []genai.Part{
		genai.Text(userPrompt),
		&genai.Blob{MIMEType: mime, Data: imgBytes},
	}

	var out types.DetectResponse
	if err := e.call(ctx, e.detectModel, system, schema, 0, parts, &out, "detect"); err != nil {
		return types.DetectResponse{}, err
	}
	return out, nil
}

// ─── PARSE ────────────────────────────────────────────────────────────────────

// Parse читает задание с фото и строит структурированный JSON с планом решения.
// Модель: parseModel (gemini-2.5-flash) — лучший OCR рукописного текста + русский.
func (e *Engine) Parse(ctx context.Context, in types.ParseRequest) (types.ParseResponse, error) {
	if e.apiKey == "" {
		return types.ParseResponse{}, fmt.Errorf("GEMINI_API_KEY not set")
	}

	system, schema, err := loadSystemWithSchema("parse")
	if err != nil {
		return types.ParseResponse{}, fmt.Errorf("gemini parse: %w", err)
	}

	imgBytes, mime, err := decodeImage(in.Image)
	if err != nil {
		return types.ParseResponse{}, fmt.Errorf("gemini parse: %w", err)
	}

	// Контекст запроса (grade, subject и т.д.) передаём в user-части
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

	userPrompt, _ := util.LoadUserPrompt("parse", promptSource, apiVersion)
	if strings.TrimSpace(userPrompt) == "" {
		userPrompt = "Верни ТОЛЬКО JSON по parse.schema v2.1.1."
	}
	userText := userPrompt + "\nINPUT_CONTEXT:\n" + string(ctxJSON)

	parts := []genai.Part{
		genai.Text(userText),
		&genai.Blob{MIMEType: mime, Data: imgBytes},
	}

	var pr types.ParseResponse
	if err := e.call(ctx, e.parseModel, system, schema, 0.1, parts, &pr, "parse"); err != nil {
		return types.ParseResponse{}, err
	}
	pr.ValidateItems()
	return pr, nil
}

// ─── HINT ─────────────────────────────────────────────────────────────────────

// Hint генерирует педагогические подсказки L1/L2/L3 на основе разобранного задания.
// Модель: parseModel — text-only, требует качественный русский педагогический текст.
func (e *Engine) Hint(ctx context.Context, in types.HintRequest) (types.HintResponse, error) {
	if e.apiKey == "" {
		return types.HintResponse{}, fmt.Errorf("GEMINI_API_KEY not set")
	}

	system, schema, err := loadSystemWithSchema("hint")
	if err != nil {
		return types.HintResponse{}, fmt.Errorf("gemini hint: %w", err)
	}

	inJSON, _ := json.Marshal(in)

	// Подставляем PARSE_OUTPUT_JSON в user-промпт (плейсхолдер {{PARSE_OUTPUT_JSON}})
	userTemplate, _ := util.LoadUserPrompt("hint", promptSource, apiVersion)
	var userText string
	if strings.Contains(userTemplate, "{{PARSE_OUTPUT_JSON}}") {
		userText = strings.ReplaceAll(userTemplate, "{{PARSE_OUTPUT_JSON}}", string(inJSON))
	} else {
		userText = "PARSE_OUTPUT_JSON:\n" + string(inJSON)
		if strings.TrimSpace(userTemplate) != "" {
			userText = userTemplate + "\n\n" + userText
		}
	}

	parts := []genai.Part{genai.Text(userText)}

	var hr types.HintResponse
	if err := e.call(ctx, e.parseModel, system, schema, 1, parts, &hr, "hint"); err != nil {
		return types.HintResponse{}, err
	}
	return hr, nil
}

// ─── CHECK ────────────────────────────────────────────────────────────────────

// CheckSolution проверяет ответ ученика на фото против условия задачи.
// Модель: parseModel — vision + математическое рассуждение.
func (e *Engine) CheckSolution(ctx context.Context, in types.CheckRequest) (types.CheckResponse, error) {
	if e.apiKey == "" {
		return types.CheckResponse{}, fmt.Errorf("GEMINI_API_KEY not set")
	}

	system, schema, err := loadSystemWithSchema("check")
	if err != nil {
		return types.CheckResponse{}, fmt.Errorf("gemini check: %w", err)
	}

	imgBytes, mime, err := decodeImage(in.Image)
	if err != nil {
		return types.CheckResponse{}, fmt.Errorf("gemini check: %w", err)
	}

	// Формируем запрос без поля image (передаётся как Blob)
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

	// Подставляем {{request_json}} в user-промпт
	userTemplate, _ := util.LoadUserPrompt("check", promptSource, apiVersion)
	var userText string
	if strings.Contains(userTemplate, "{{request_json}}") {
		userText = strings.ReplaceAll(userTemplate, "{{request_json}}", string(reqJSON))
	} else {
		userText = "INPUT_JSON:\n" + string(reqJSON)
		if strings.TrimSpace(userTemplate) != "" {
			userText = userTemplate + "\n\n" + userText
		}
	}

	parts := []genai.Part{
		genai.Text(userText),
		&genai.Blob{MIMEType: mime, Data: imgBytes},
	}

	var cr types.CheckResponse
	if err := e.call(ctx, e.parseModel, system, schema, 0, parts, &cr, "check"); err != nil {
		return types.CheckResponse{}, err
	}
	cr.NormalizeDecision()
	cr.SetIsCorrectFromDecision()
	return cr, nil
}

// ─── ANALOGUE ─────────────────────────────────────────────────────────────────

// AnalogueSolution генерирует аналогичное задание тем же приёмом.
// Модель: parseModel — text-only, генерация на русском языке.
func (e *Engine) AnalogueSolution(ctx context.Context, in types.AnalogueRequest) (types.AnalogueResponse, error) {
	if e.apiKey == "" {
		return types.AnalogueResponse{}, fmt.Errorf("GEMINI_API_KEY not set")
	}

	system, schema, err := loadSystemWithSchema("analogue")
	if err != nil {
		return types.AnalogueResponse{}, fmt.Errorf("gemini analogue: %w", err)
	}

	inJSON, _ := json.Marshal(in)

	userTemplate, _ := util.LoadUserPrompt("analogue", promptSource, apiVersion)
	var userText string
	if strings.TrimSpace(userTemplate) != "" {
		userText = userTemplate + "\n\nINPUT_JSON:\n" + string(inJSON)
	} else {
		userText = "INPUT_JSON:\n" + string(inJSON)
	}

	parts := []genai.Part{genai.Text(userText)}

	var ar types.AnalogueResponse
	if err := e.call(ctx, e.parseModel, system, schema, 1, parts, &ar, "analogue"); err != nil {
		return types.AnalogueResponse{}, err
	}
	return ar, nil
}

// ─── внутренние хелперы ───────────────────────────────────────────────────────

// retryAfterRe ищет "retry in Xs" в тексте googleapi-ошибки Gemini.
var retryAfterRe = regexp.MustCompile(`retry in (\d+)s`)

// is429 проверяет, что ошибка — HTTP 429 (rate limit / quota exceeded).
func is429(err error) bool {
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == 429
	}
	// Fallback: строковая проверка для gRPC-пути
	return strings.Contains(err.Error(), "429") ||
		strings.Contains(err.Error(), "QuotaFailure") ||
		strings.Contains(err.Error(), "RESOURCE_EXHAUSTED")
}

// retryDelay возвращает паузу перед следующим retry:
//   - 429: берём значение из "retry in Xs" (+ 2 сек запас) или 30 сек по умолчанию
//   - другие транзиентные ошибки: экспоненциальный backoff 0.5 / 1 / 2 / 4 сек
func retryDelay(err error, attempt int) time.Duration {
	if is429(err) {
		if m := retryAfterRe.FindStringSubmatch(err.Error()); len(m) > 1 {
			if secs, e := strconv.Atoi(m[1]); e == nil && secs > 0 {
				return time.Duration(secs+2) * time.Second
			}
		}
		return 30 * time.Second
	}
	return time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond // 0.5, 1, 2, 4 сек
}

// call создаёт Gemini клиент, настраивает модель и выполняет запрос с умным retry.
// Для 429 (quota exceeded) ждёт retry-after из ответа Gemini, а не фиксированные мс.
func (e *Engine) call(
	ctx context.Context,
	model string,
	systemPrompt string,
	schemaJSON string,
	temperature float32,
	parts []genai.Part,
	dst any,
	op string,
) error {
	cl, err := genai.NewClient(ctx, option.WithAPIKey(e.apiKey))
	if err != nil {
		return fmt.Errorf("gemini %s: new client: %w", op, err)
	}
	defer cl.Close()

	m := cl.GenerativeModel(model)
	m.GenerationConfig = genai.GenerationConfig{
		Temperature:      ptrFloat32(temperature),
		ResponseMIMEType: "application/json",
	}
	m.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(systemPrompt),
			genai.Text("\nJSON schema для ответа (следуй строго):\n" + schemaJSON),
		},
	}

	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := m.GenerateContent(ctx, parts...)
		if err != nil {
			lastErr = err
			delay := retryDelay(err, attempt)
			if attempt < maxAttempts {
				select {
				case <-ctx.Done():
					return fmt.Errorf("gemini %s: context cancelled while waiting retry: %w", op, ctx.Err())
				case <-time.After(delay):
				}
			}
			continue
		}

		txt := firstText(resp)
		if strings.TrimSpace(txt) == "" {
			lastErr = fmt.Errorf("gemini %s: empty response", op)
			if attempt < maxAttempts {
				time.Sleep(500 * time.Millisecond)
			}
			continue
		}

		txt = util.StripCodeFences(strings.TrimSpace(txt))
		if err := json.Unmarshal([]byte(txt), dst); err != nil {
			return fmt.Errorf("gemini %s: bad JSON: %w", op, err)
		}
		return nil
	}
	return lastErr
}

// loadSystemWithSchema загружает system-промпт и схему, возвращает их как строки.
// Промпты берутся из директории gpt (содержимое одинаковое для обоих провайдеров).
func loadSystemWithSchema(name string) (system, schemaJSON string, err error) {
	sys, err := util.LoadSystemPrompt(name, promptSource, apiVersion)
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

func firstText(resp *genai.GenerateContentResponse) string {
	if resp == nil || len(resp.Candidates) == 0 {
		return ""
	}
	for _, c := range resp.Candidates {
		if c.Content == nil {
			continue
		}
		for _, p := range c.Content.Parts {
			if t, ok := p.(genai.Text); ok && strings.TrimSpace(string(t)) != "" {
				return string(t)
			}
		}
	}
	return ""
}

func ptrFloat32(v float32) *float32 { return &v }