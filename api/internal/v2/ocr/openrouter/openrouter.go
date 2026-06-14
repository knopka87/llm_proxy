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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v2/ocr/types"
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
	apiKey string
	models StepModels
	httpc  *http.Client
}

func New(apiKey string, models StepModels) *Engine {
	tr := &http.Transport{
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
	system, schemaJSON, err := loadSystemWithSchema("detect")
	if err != nil {
		return types.DetectResponse{}, nil, fmt.Errorf("openrouter detect: %w", err)
	}
	userPrompt, _ := util.LoadUserPrompt("detect", promptSource, apiVersion)
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
	system, schemaJSON, err := loadSystemWithSchema("parse")
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

	userPrompt, _ := util.LoadUserPrompt("parse", promptSource, apiVersion)
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
	pr.ValidateItems()
	return pr, stats, nil
}

// ─── HINT ─────────────────────────────────────────────────────────────────────

func (e *Engine) Hint(ctx context.Context, in types.HintRequest) (types.HintResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("hint")
	if err != nil {
		return types.HintResponse{}, nil, fmt.Errorf("openrouter hint: %w", err)
	}

	inJSON, _ := json.Marshal(in)
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

	messages := []message{systemMsg(system), userMsgText(userText)}

	var hr types.HintResponse
	stats, err := e.call(ctx, e.models.Hint, "hint", messages, schemaJSON, &hr)
	return hr, stats, err
}

// ─── CHECK ────────────────────────────────────────────────────────────────────

func (e *Engine) CheckSolution(ctx context.Context, in types.CheckRequest) (types.CheckResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("check")
	if err != nil {
		return types.CheckResponse{}, nil, fmt.Errorf("openrouter check: %w", err)
	}

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

	messages := []message{systemMsg(system), userMsgWithImage(userText, mime, imgBytes)}

	var cr types.CheckResponse
	stats, err := e.call(ctx, e.models.Check, "check", messages, schemaJSON, &cr)
	if err != nil {
		return types.CheckResponse{}, stats, err
	}
	cr.NormalizeDecision()
	cr.SetIsCorrectFromDecision()
	return cr, stats, err
}

// ─── ANALOGUE ─────────────────────────────────────────────────────────────────

func (e *Engine) AnalogueSolution(ctx context.Context, in types.AnalogueRequest) (types.AnalogueResponse, *types.LLMStats, error) {
	system, schemaJSON, err := loadSystemWithSchema("analogue")
	if err != nil {
		return types.AnalogueResponse{}, nil, fmt.Errorf("openrouter analogue: %w", err)
	}

	inJSON, _ := json.Marshal(in)
	userTemplate, _ := util.LoadUserPrompt("analogue", promptSource, apiVersion)
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
	Model          string         `json:"model"`
	Messages       []message      `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
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
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
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

	reqBody := chatRequest{
		Model:          model,
		Messages:       messages,
		ResponseFormat: rf,
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

	payload, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("HTTP-Referer", "https://vk.obyasnyatel.ru")
	req.Header.Set("X-Title", "Объяснятель ДЗ")

	start := time.Now()
	resp, err := e.httpc.Do(req)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("openrouter %s: %w", op, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter %s %d: %s", op, resp.StatusCode, truncate(raw, 512))
	}

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
	}

	text := util.StripCodeFences(strings.TrimSpace(cr.Choices[0].Message.Content))
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

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}
