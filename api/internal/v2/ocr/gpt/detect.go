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
	"time"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v2/ocr/types"
)

const DETECT = "detect"

func (e *Engine) Detect(ctx context.Context, in types.DetectRequest) (types.DetectResponse, error) {
	if e.APIKey == "" {
		return types.DetectResponse{}, fmt.Errorf("OPENAI_API_KEY not set")
	}

	model := e.GetModel()
	// TODO переделать на отдельный env
	model = "gpt-4.1-mini"

	// accept raw base64 or data: URL
	imgBytes, mimeFromDataURL, _ := util.DecodeBase64MaybeDataURL(in.Image)
	if len(imgBytes) == 0 {
		raw, err := base64.StdEncoding.DecodeString(in.Image)
		if err != nil {
			return types.DetectResponse{}, fmt.Errorf("openai detect: invalid image base64")
		}
		imgBytes = raw
	}
	mime := util.PickMIME("", mimeFromDataURL, imgBytes)
	if !isOpenAIImageMIME(mime) {
		return types.DetectResponse{}, fmt.Errorf("openai detect: unsupported MIME %s (need image/jpeg|png|webp)", mime)
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)
	in.Image = ""

	system, err := util.LoadSystemPrompt(DETECT, e.Name(), e.Version())
	if err != nil {
		return types.DetectResponse{}, err
	}

	schema, err := util.LoadPromptSchema(DETECT, e.Version())
	if err != nil {
		return types.DetectResponse{}, err
	}
	util.FixJSONSchemaStrict(schema)

	user, err := util.LoadUserPrompt(DETECT, e.Name(), e.Version())
	if err != nil {
		return types.DetectResponse{}, err
	}

	userObj := map[string]any{
		"task": user,
		"in":   in,
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
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   DETECT,
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

	start := time.Now()
	resp, err := e.httpc.Do(req)
	t := time.Since(start).Milliseconds()
	log.Printf("detect time: %d", t)
	if err != nil {
		return types.DetectResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.DetectResponse{}, fmt.Errorf("openai detect %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		// fallback to manual extraction from Responses API envelope
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.DetectResponse{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var r types.DetectResponse
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return types.DetectResponse{}, fmt.Errorf("openai detect: bad JSON: %w", err)
	}
	return r, nil
}
