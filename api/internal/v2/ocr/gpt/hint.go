package gpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v2/ocr/types"
)

const HINT = "hint"

func (e *Engine) Hint(ctx context.Context, in types.HintRequest) (types.HintResponse, error) {
	if e.APIKey == "" {
		return types.HintResponse{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.GetModel()

	// TODO переделать на отдельный env
	// Базовая модель по уровню: L1/L2 — gpt-4.1-mini, L3 — gpt-5-mini.
	// model = "gpt-4.1-mini"
	model = "gpt-5-mini"

	// Параметры сэмплинга по уровням
	temp := 1

	// switch in.Level {
	// case types.HintL3:
	// 	// model = "gpt-5-mini"
	// 	temp = 1
	// case types.HintL2:
	// 	// остаёмся на gpt-4.1-mini
	// 	temp = 1
	// default:
	// 	// L1: значения по умолчанию заданы выше
	// }

	// Try to load system prompt from /prompt/hint<L1|L2|L3>.txt; fallback to the default text if not found.
	system, err := util.LoadSystemPrompt(HINT, e.Name(), e.Version())
	if err != nil {
		return types.HintResponse{}, err
	}

	schema, err := util.LoadPromptSchema(HINT, e.Version())
	if err != nil {
		return types.HintResponse{}, err
	}
	util.FixJSONSchemaStrict(schema)

	userTask, err := util.LoadUserPrompt(HINT, e.Name(), e.Version())
	if err != nil {
		return types.HintResponse{}, err
	}

	userObj := map[string]any{
		"task":  userTask,
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

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
					map[string]any{"type": "input_text", "text": string(userJSON)},
				},
			},
		},
		"temperature": temp,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   HINT,
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
		return types.HintResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.HintResponse{}, fmt.Errorf("openai hint %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.HintResponse{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var hr types.HintResponse
	if err := json.Unmarshal([]byte(out), &hr); err != nil {
		return types.HintResponse{}, fmt.Errorf("openai hint: bad JSON: %w", err)
	}
	return hr, nil
}
