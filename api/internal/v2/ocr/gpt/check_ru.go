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

const CHECK_RU = "check_ru"

func (e *Engine) CheckRU(ctx context.Context, in types.CheckRUCompactInput) (types.CheckRUResponse, *types.LLMStats, error) {
	if e.apiKey == "" {
		return types.CheckRUResponse{}, nil, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.GetModel()
	if model == "" {
		model = "gpt-5-mini"
	}

	temp := 1

	system, err := util.LoadSystemPrompt(CHECK_RU, e.Name(), e.Version())
	if err != nil {
		return types.CheckRUResponse{}, nil, err
	}

	schema, err := util.LoadPromptSchema(CHECK_RU, e.Version())
	if err != nil {
		return types.CheckRUResponse{}, nil, err
	}
	util.FixJSONSchemaStrict(schema)

	userTask, err := util.LoadUserPrompt(CHECK_RU, e.Name(), e.Version())
	if err != nil {
		userTask = ""
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
				"name":   CHECK_RU,
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
		return types.CheckRUResponse{}, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.CheckRUResponse{}, nil, fmt.Errorf("openai check_ru %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
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
		return types.CheckRUResponse{}, stats, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var cr types.CheckRUResponse
	if err := json.Unmarshal([]byte(out), &cr); err != nil {
		return types.CheckRUResponse{}, stats, fmt.Errorf("openai check_ru: bad JSON: %w", err)
	}
	return cr, stats, nil
}
