package gpt

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
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

func (e *Engine) Name() string { return "gpt" }

func (e *Engine) GetModel() string { return e.Model }

// fallbackExtractResponsesText extracts model text from the Responses API envelope
// per https://platform.openai.com/docs/api-reference/responses/object.
// It prefers `output_text`, and otherwise concatenates any text segments
// found in `output[i].content[j].text` where `type` is `output_text` or `text`.
func fallbackExtractResponsesText(raw []byte) string {
	type content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type output struct {
		Content []content `json:"content"`
		Role    string    `json:"role,omitempty"`
	}
	var env struct {
		Object     string   `json:"object"`
		Status     string   `json:"status"`
		Output     []output `json:"output"`
		OutputText string   `json:"output_text"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return ""
	}

	// Prefer the convenience field when present
	if s := strings.TrimSpace(env.OutputText); s != "" {
		return s
	}

	var b strings.Builder
	for _, o := range env.Output {
		for _, c := range o.Content {
			if strings.TrimSpace(c.Text) == "" {
				continue
			}
			// Both `output_text` and `text` are seen in practice
			if c.Type == "output_text" || c.Type == "text" || c.Type == "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(c.Text)
			}
		}
	}
	return b.String()
}

func truncateBytes(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}

func isOpenAIImageMIME(m string) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	switch m {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return true
	}
	return false
}
