package gemini

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

func (e *Engine) Name() string { return "gemini" }

func (e *Engine) Recognize(ctx context.Context, image []byte, opt ocr.Options) (string, error) {
	if e.APIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY is empty")
	}
	if opt.Model != "" {
		e.Model = opt.Model
	}
	mime := util.SniffMimeHTTP(image)
	b64 := base64.StdEncoding.EncodeToString(image)

	body := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": "Transcribe all legible text from the image. Return plain UTF-8 text only."},
					map[string]any{"inline_data": map[string]any{
						"mime_type": mime,
						"data":      b64,
					}},
				},
			},
		},
	}
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models/%s:generateContent?key=%s", e.Model, e.APIKey)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini %d: %s", resp.StatusCode, string(x))
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Candidates) > 0 && len(out.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(out.Candidates[0].Content.Parts[0].Text), nil
	}
	return "", nil
}
