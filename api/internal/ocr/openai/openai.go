package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"child-bot/api/internal/ocr"
	"child-bot/api/internal/prompt"
	"child-bot/api/internal/util"
)

type Engine struct {
	APIKey string
	Model  string
	httpc  *http.Client
}

func New(key, model string) *Engine {
	return &Engine{
		APIKey: key,
		Model:  model,
		httpc:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *Engine) Name() string { return "gpt" }

func (e *Engine) GetModel() string { return e.Model }

func (e *Engine) Detect(ctx context.Context, img []byte, mime string, gradeHint int) (ocr.DetectResult, error) {
	if e.APIKey == "" {
		return ocr.DetectResult{}, fmt.Errorf("OPENAI_API_KEY not set")
	}
	b64 := base64.StdEncoding.EncodeToString(img)
	dataURL := "data:" + mime + ";base64," + b64

	system := `Ты — внимательный ассистент 1–4 классов. НЕ решай задание.
Верни только JSON, строго соответствующий detect.schema.json. Любой текст вне JSON — ошибка.

Ниже содержимое detect.schema.json (используй его как спецификацию формата ответа):
` + prompt.DetectSchema

	user := "Ответ строго JSON по detect.schema.json. Без комментариев."
	if gradeHint >= 1 && gradeHint <= 4 {
		user += fmt.Sprintf(" grade_hint=%d", gradeHint)
	}

	body := map[string]any{
		"model": e.Model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": user},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL, "detail": "high"}},
				},
			},
		},
		"temperature": 0,
		// (опционально) можно включить жёсткий JSON-режим у OpenAI:
		"response_format": map[string]any{"type": "json_object"},
	}
	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.DetectResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		x, _ := io.ReadAll(resp.Body)
		return ocr.DetectResult{}, fmt.Errorf("openai detect %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.DetectResult{}, err
	}
	if len(raw.Choices) == 0 {
		return ocr.DetectResult{}, fmt.Errorf("openai detect: empty response")
	}
	out := strings.TrimSpace(raw.Choices[0].Message.Content)
	out = util.StripCodeFences(out)

	var r ocr.DetectResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return ocr.DetectResult{}, fmt.Errorf("openai detect: bad JSON: %w", err)
	}
	return r, nil
}

func (e *Engine) Parse(ctx context.Context, image []byte, opt ocr.ParseOptions) (ocr.ParseResult, error) {
	if e.APIKey == "" {
		return ocr.ParseResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.Model
	if opt.ModelOverride != "" {
		model = opt.ModelOverride
	}

	mime := util.SniffMimeHTTP(image)
	b64 := base64.StdEncoding.EncodeToString(image)
	dataURL := "data:" + mime + ";base64," + b64

	// Подсказки из DETECT/выбора пользователя
	var hints strings.Builder
	if opt.GradeHint >= 1 && opt.GradeHint <= 4 {
		fmt.Fprintf(&hints, " grade_hint=%d.", opt.GradeHint)
	}
	if s := strings.TrimSpace(opt.SubjectHint); s != "" {
		fmt.Fprintf(&hints, " subject_hint=%q.", s)
	}
	// если пользователь выбрал один из нескольких пунктов — добавим это как ориентир
	if opt.SelectedTaskIndex >= 0 || strings.TrimSpace(opt.SelectedTaskBrief) != "" {
		fmt.Fprintf(&hints, " selected_task=[index:%d, brief:%q].", opt.SelectedTaskIndex, opt.SelectedTaskBrief)
	}

	system := `Ты — школьный ассистент 1–4 классов. Перепиши выбранное задание полностью текстом, не додумывай.
Выдели вопрос задачи. Нечитаемые места помечай в квадратных скобках.
Соблюдай политику подтверждения:
- Автоподтверждение, если: confidence ≥ 0.80, meaning_change_risk ≤ 0.20, bracketed_spans_count = 0, needs_rescan=false.
- Иначе запрашивай подтверждение.
Верни только JSON по parse.schema.json. Любой текст вне JSON — ошибка.

parse.schema.json:
` + prompt.ParseSchema

	user := "Ответ строго JSON по parse.schema.json. Без комментариев." + hints.String()

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": user},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL, "detail": "high"}},
				},
			},
		},
		"temperature":     0,
		"response_format": map[string]any{"type": "json_object"},
	}

	payload, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.ParseResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.ParseResult{}, fmt.Errorf("openai parse %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.ParseResult{}, err
	}
	if len(raw.Choices) == 0 {
		return ocr.ParseResult{}, fmt.Errorf("openai parse: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var pr ocr.ParseResult
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return ocr.ParseResult{}, fmt.Errorf("openai parse: bad JSON: %w", err)
	}

	// Серверный гард (политика подтверждения из PROMPT_PARSE)
	ocr.ApplyParsePolicy(&pr)
	return pr, nil
}

func (e *Engine) Hint(ctx context.Context, in ocr.HintInput) (ocr.HintResult, error) {
	if e.APIKey == "" {
		return ocr.HintResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.Model

	system := `Ты — помощник для 1–4 классов. Сформируй РОВНО ОДИН блок подсказки уровня ` + string(in.Level) + `.
Не решай задачу и не подставляй числа/слова из условия. Вывод — строго JSON по hint.schema.json.

hint.schema.json:
` + prompt.HintSchema

	userObj := map[string]any{
		"task":  "Сгенерируй подсказку согласно PROMPT_HINT v1.4 и верни JSON по hint.schema.json.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": string(userJSON)},
		},
		"temperature":     0,
		"response_format": map[string]any{"type": "json_object"},
	}
	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.HintResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.HintResult{}, fmt.Errorf("openai hint %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.HintResult{}, err
	}
	if len(raw.Choices) == 0 {
		return ocr.HintResult{}, fmt.Errorf("openai hint: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var hr ocr.HintResult
	if err := json.Unmarshal([]byte(out), &hr); err != nil {
		return ocr.HintResult{}, fmt.Errorf("openai hint: bad JSON: %w", err)
	}
	hr.NoFinalAnswer = true
	return hr, nil
}
