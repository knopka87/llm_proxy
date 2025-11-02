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

const OCR = "ocr"

func (e *Engine) OCR(ctx context.Context, in types.OCRRequest) (types.OCRResponse, error) {
	if e.APIKey == "" {
		return types.OCRResponse{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.GetModel()

	// TODO переделать на отдельный env
	model = "gpt-4.1-mini"

	system, err := util.LoadSystemPrompt(OCR, e.Name(), e.Version())
	if err != nil {
		return types.OCRResponse{}, err
	}

	schema, err := util.LoadPromptSchema(OCR, e.Version())
	if err != nil {
		return types.OCRResponse{}, err
	}
	util.FixJSONSchemaStrict(schema)

	user, err := util.LoadUserPrompt(OCR, e.Name(), e.Version())
	if err != nil {
		return types.OCRResponse{}, err
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
				"name":   OCR,
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
		return types.OCRResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.OCRResponse{}, fmt.Errorf("openai OCR %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.OCRResponse{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var pr types.OCRResponse
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return types.OCRResponse{}, fmt.Errorf("openai OCR: bad JSON: %w", err)
	}
	return pr, nil
}
