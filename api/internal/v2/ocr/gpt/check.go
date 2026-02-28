package gpt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v2/ocr/types"
)

const CHECK = "check"

func (e *Engine) CheckSolution(ctx context.Context, in types.CheckRequest) (types.CheckResponse, error) {
	log.Printf("[check] started, image_len=%d, task_text=%q, items_count=%d",
		len(in.Image), truncateStr(in.RawTaskText, 50), len(in.TaskStruct.Items))

	if e.APIKey == "" {
		log.Printf("[check] ERROR: OPENAI_API_KEY is empty")
		return types.CheckResponse{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.GetModel()
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}
	// TODO переделать на отдельный env
	model = "gpt-5-mini"

	system, err := util.LoadSystemPrompt(CHECK, e.Name(), e.Version())
	if err != nil {
		return types.CheckResponse{}, err
	}

	schema, err := util.LoadPromptSchema(CHECK, e.Version())
	if err != nil {
		return types.CheckResponse{}, err
	}
	util.FixJSONSchemaStrict(schema)

	user, err := util.LoadUserPrompt(CHECK, e.Name(), e.Version())
	if err != nil {
		return types.CheckResponse{}, err
	}

	// Decode image from base64 and create data URL for multimodal input
	imgBytes, mimeFromDataURL, _ := util.DecodeBase64MaybeDataURL(in.Image)
	if len(imgBytes) == 0 {
		raw, err := base64.StdEncoding.DecodeString(in.Image)
		if err != nil {
			return types.CheckResponse{}, fmt.Errorf("openai check: invalid image base64")
		}
		imgBytes = raw
	}
	mime := util.PickMIME("", mimeFromDataURL, imgBytes)
	if !isOpenAIImageMIME(mime) {
		return types.CheckResponse{}, fmt.Errorf("openai check: unsupported MIME %s (need image/jpeg|png|webp)", mime)
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)
	in.Image = "" // Clear from JSON since sending as separate image block

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
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "INPUT_JSON:\n" + string(userJSON)},
					map[string]any{"type": "input_image", "image_url": dataURL},
				},
			},
		},
		"temperature": 0.5,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   CHECK,
				"strict": true,
				"schema": schema,
			},
		},
	}
	if strings.Contains(model, "gpt-5") {
		// Lower temperature for better instruction following in check mode
		body["temperature"] = 0.3
	}

	payload, _ := json.Marshal(body)
	log.Printf("[check] calling OpenAI API, model=%s, payload_len=%d", model, len(payload))

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		log.Printf("[check] ERROR: HTTP request failed: %v", err)
		return types.CheckResponse{}, err
	}
	defer resp.Body.Close()

	log.Printf("[check] OpenAI response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		log.Printf("[check] ERROR: OpenAI returned %d: %s", resp.StatusCode, truncateStr(string(x), 500))
		return types.CheckResponse{}, fmt.Errorf("openai check %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	log.Printf("[check] OpenAI response body_len=%d", len(raw))

	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		log.Printf("[check] ERROR: empty output from OpenAI, body=%s", truncateBytes(raw, 500))
		return types.CheckResponse{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}

	var cr types.CheckResponse
	if err := json.Unmarshal([]byte(out), &cr); err != nil {
		log.Printf("[check] ERROR: bad JSON from OpenAI: %v, out=%s", err, truncateStr(out, 500))
		return types.CheckResponse{}, fmt.Errorf("openai check: bad JSON: %w", err)
	}

	log.Printf("[check] success: status=%s, can_evaluate=%v, is_correct=%v", cr.Status, cr.CanEvaluate, cr.IsCorrect)
	return cr, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
