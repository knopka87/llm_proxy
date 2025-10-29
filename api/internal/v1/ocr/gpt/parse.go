package gpt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v1/ocr"
	"llm-proxy/api/internal/v1/ocr/types"
)

const PARSE = "parse"

func (e *Engine) Parse(ctx context.Context, in types.ParseInput) (types.ParseResult, error) {
	if e.APIKey == "" {
		return types.ParseResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.model
	if in.Options.ModelOverride != "" {
		model = in.Options.ModelOverride
	}
	// TODO переделать на отдельный env
	model = "gpt-4.1-mini"

	imgBytes, mimeFromDataURL, _ := util.DecodeBase64MaybeDataURL(in.ImageB64)
	if len(imgBytes) == 0 {
		raw, err := base64.StdEncoding.DecodeString(in.ImageB64)
		if err != nil {
			return types.ParseResult{}, fmt.Errorf("openai parse: invalid image base64")
		}
		imgBytes = raw
	}
	mime := util.PickMIME("", mimeFromDataURL, imgBytes)
	if !isOpenAIImageMIME(mime) {
		return types.ParseResult{}, fmt.Errorf("openai parse: unsupported MIME %s (need image/jpeg|png|webp)", mime)
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)

	var hints strings.Builder
	if in.Options.GradeHint >= 1 && in.Options.GradeHint <= 4 {
		_, _ = fmt.Fprintf(&hints, " grade_hint=%d.", in.Options.GradeHint)
	}
	if s := strings.TrimSpace(in.Options.SubjectHint); s != "" {
		_, _ = fmt.Fprintf(&hints, " subject_hint=%q.", s)
	}
	if in.Options.SelectedTaskIndex >= 0 || strings.TrimSpace(in.Options.SelectedTaskBrief) != "" {
		_, _ = fmt.Fprintf(&hints, " selected_task=[index:%d, brief:%q].", in.Options.SelectedTaskIndex, in.Options.SelectedTaskBrief)
	}

	system := `Ты — школьный ассистент 1–4 классов. Перепиши выбранное задание полностью текстом, не додумывай.
Выдели вопрос задачи. Нечитаемые места помечай в квадратных скобках.
Соблюдай политику подтверждения:
- Автоподтверждение, если: confidence ≥ 0.80, meaning_change_risk ≤ 0.20, bracketed_spans_count = 0, needs_rescan=false.
- Иначе запрашивай подтверждение.
Верни строго JSON по схеме parse. Любой текст вне JSON — ошибка
`
	schema, err := util.LoadPromptSchema(PARSE, e.Version())
	if err != nil {
		return types.ParseResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	user := "Ответ строго JSON по схеме. Без комментариев." + hints.String()

	body := map[string]any{
		"model": model,
		"input": []any{
			map[string]any{
				"role": "system",
				"content": []any{
					map[string]any{"type": "input_text", "text": system},
				},
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": user},
					map[string]any{"type": "input_image", "image_url": dataURL},
				},
			},
		},
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   PARSE,
				"strict": true,
				"schema": schema,
			},
		},
	}
	if strings.Contains(model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.ParseResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.ParseResult{}, fmt.Errorf("openai parse %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.ParseResult{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var pr types.ParseResult
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return types.ParseResult{}, fmt.Errorf("openai parse: bad JSON: %w", err)
	}
	ocr.ApplyParsePolicy(&pr)
	return pr, nil
}
