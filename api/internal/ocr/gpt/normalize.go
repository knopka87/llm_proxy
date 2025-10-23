package gpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-proxy/api/internal/ocr/types"
	"llm-proxy/api/internal/util"
)

func (e *Engine) Normalize(ctx context.Context, in types.NormalizeInput) (types.NormalizeResult, error) {
	if e.APIKey == "" {
		return types.NormalizeResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.Model
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}

	// TODO переделать на отдельный env
	e.Model = "gpt-4o"

	system := `PROMPT — NORMALIZE_ANSWER (NO PII, компакт для MVP)
Задача: нормализуй ответ ученика в JSON по схеме normalize: success, shape ∈ {number|string|steps|list}, value, optional units, warnings.
Не придумывай ответ, не решай заново, не меняй смысл. Не выводи null-поля и пустые массивы.
Маршрутизация:
• Низкий OCR/качество → next_action_hint=ask_rephoto (+ краткая фраза в needs_user_action_message ≤120).
• Формат/форма не соблюдена → next_action_hint=ask_retry (+ короткая подсказка).
• Конфликт значений → success=false, error=too_many_candidates.
Правила:
• steps и list — до 6 элементов. Для длинных списков: показывай 6, поясни в explanation_short (≤400).
• Если ожидался number, а пришло «число+единица» — верни число (value), units.kept=false, warnings+=unit_removed.
• Unicode допустим; NBSP нормализовать (notes+=unicode_nbsp_normalized).
• Доп. пояснение (если нужно) — explanation_short (≤400).
Примеры (кратко):
A) number (зачёркнутое): raw="5̶, 7" → {"success":true,"shape":"number","value":7,"number_kind":"integer","warnings":["correction_detected"]}
B) steps: "1) 30+20=50; 2) 5+7=12; 3) 50+12=62" → {"success":true,"shape":"steps","value":["30+20=50","5+7=12","50+12=62"]}
C) list+units: "5 см, 7 см, 9 см" → {"success":true,"shape":"list","value":[5,7,9],"units":{"detected":"см","canonical":"cm","kept":false},"warnings":["unit_removed"]}
D) string+Unicode: "длина = 12\u202fсм — ок" → {"success":true,"shape":"string","value":"длина = 12 см — ок","units":{"detected":"см","canonical":"cm","kept":true},"normalized":{"notes":["unicode_nbsp_normalized"]}}
Верни строго JSON по схеме normalize. Любой текст вне JSON — ошибка.`

	schema, err := util.LoadPromptSchema("normalize")
	if err != nil {
		return types.NormalizeResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	userObj := map[string]any{
		"task":  "Нормализуй ответ ученика и верни только JSON по схеме normalize.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	var userContent []any
	if strings.EqualFold(in.Answer.Source, "photo") {
		b64 := strings.TrimSpace(in.Answer.PhotoB64)
		if b64 == "" {
			return types.NormalizeResult{}, fmt.Errorf("openai normalize: answer.photo_b64 is empty")
		}
		photoBytes, mimeFromDataURL, err := util.DecodeBase64MaybeDataURL(in.Answer.PhotoB64)
		if err != nil {
			return types.NormalizeResult{}, fmt.Errorf("openai normalize: bad photo base64: %w", err)
		}
		mime := util.PickMIME(strings.TrimSpace(in.Answer.Mime), mimeFromDataURL, photoBytes)
		dataURL := b64
		if !strings.HasPrefix(strings.ToLower(dataURL), "data:") {
			dataURL = "data:" + mime + ";base64," + b64
		}
		userContent = []any{
			map[string]any{"type": "input_text", "text": "INPUT_JSON:\n" + string(userJSON)},
			map[string]any{"type": "input_image", "image_url": dataURL},
		}
	} else {
		if strings.TrimSpace(in.Answer.Text) == "" {
			return types.NormalizeResult{}, fmt.Errorf("openai normalize: answer.text is empty")
		}
		userContent = []any{map[string]any{"type": "input_text", "text": string(userJSON)}}
	}

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
				"role":    "user",
				"content": userContent,
			},
		},
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "normalize",
				"strict": true,
				"schema": schema,
			},
		},
	}
	if strings.Contains(e.Model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.NormalizeResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.NormalizeResult{}, fmt.Errorf("openai normalize %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.NormalizeResult{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var nr types.NormalizeResult
	if err := json.Unmarshal([]byte(out), &nr); err != nil {
		return types.NormalizeResult{}, fmt.Errorf("openai normalize: bad JSON: %w", err)
	}
	return nr, nil
}
