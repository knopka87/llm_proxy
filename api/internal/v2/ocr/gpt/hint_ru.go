package gpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v2/ocr/types"
)

const HINT_RU = "hint_ru"

func (e *Engine) HintRU(ctx context.Context, in types.HintRUCompactInput) (types.HintRUResponse, *types.LLMStats, error) {
	if e.apiKey == "" {
		return types.HintRUResponse{}, nil, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.GetModel()
	if model == "" {
		model = "gpt-5-mini"
	}

	temp := 1

	system, err := util.LoadSystemPrompt(HINT_RU, e.Name(), e.Version())
	if err != nil {
		return types.HintRUResponse{}, nil, err
	}

	schema, err := util.LoadPromptSchema(HINT_RU, e.Version())
	if err != nil {
		return types.HintRUResponse{}, nil, err
	}
	util.FixJSONSchemaStrict(schema)

	userJSON, _ := json.Marshal(in)

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
				"name":   HINT_RU,
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
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	start := time.Now()
	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.HintRUResponse{}, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.HintRUResponse{}, nil, fmt.Errorf("openai hint_ru %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	t := time.Since(start).Milliseconds()
	inTok, outTok := parseUsage(raw)
	stats := &types.LLMStats{InputTokens: inTok, OutputTokens: outTok, LatencyMs: t}
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.HintRUResponse{}, stats, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var hr types.HintRUResponse
	if err := json.Unmarshal([]byte(out), &hr); err != nil {
		return types.HintRUResponse{}, stats, fmt.Errorf("openai hint_ru: bad JSON: %w", err)
	}
	return hr, stats, nil
}
