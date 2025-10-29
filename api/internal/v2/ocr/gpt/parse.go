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

const PARSE = "parse"

func (e *Engine) Parse(ctx context.Context, in types.ParseInput) (types.ParseResult, error) {
	if e.APIKey == "" {
		return types.ParseResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.GetModel()

	// TODO переделать на отдельный env
	model = "gpt-4.1-mini"

	system, err := util.LoadSystemPrompt(PARSE, e.Name(), e.Version())
	if err != nil {
		return types.ParseResult{}, err
	}

	schema, err := util.LoadPromptSchema(PARSE, e.Version())
	if err != nil {
		return types.ParseResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	user, err := util.LoadUserPrompt(PARSE, e.Name(), e.Version())
	if err != nil {
		return types.ParseResult{}, err
	}

	userObj := map[string]any{
		"task":  user,
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
	return pr, nil
}
