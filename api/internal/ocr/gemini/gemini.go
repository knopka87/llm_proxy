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

func (e *Engine) Analyze(ctx context.Context, image []byte, opt ocr.Options) (ocr.Result, error) {
	if e.APIKey == "" {
		return ocr.Result{}, fmt.Errorf("GEMINI_API_KEY is empty")
	}
	model := e.Model
	if opt.Model != "" {
		model = opt.Model
	}
	mime := util.SniffMimeHTTP(image)
	b64 := base64.StdEncoding.EncodeToString(image)

	system := `You are an assistant that analyzes a PHOTO of a school math/logic task.
1) Extract the task text (human-readable string).
2) Detect if a written SOLUTION is present on the photo.
3) If there is NO solution -> produce exactly 3 hints (L1..L3): from light nudge to more detailed plan. Do NOT give the final answer.
4) If there IS a solution -> check it. If correct -> verdict "correct".
   Otherwise -> verdict "incorrect" and explain WHERE or WHAT KIND OF error (without giving the actual fix or final result).
   In both cases produce 3 hints as above (no final answer).
Return STRICT JSON with the following fields (text and solutionNote on russian language):
{
  "text": string,                 // extracted readable text from the photo (may be empty)
  "foundTask": boolean,
  "foundSolution": boolean,
  "solutionVerdict": "correct" | "incorrect" | "uncertain" | "",
  "solutionNote": string,         // short note about where/what kind of the error (no final answer)
  "hints": [string, string, string] // exactly 3 items when possible
}`

	body := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": system},
					map[string]any{"inline_data": map[string]any{
						"mime_type": mime,
						"data":      b64,
					}},
				},
			},
		},
		"generationConfig": map[string]any{"temperature": 0},
	}
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models/%s:generateContent?key=%s", model, e.APIKey)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.Result{}, fmt.Errorf("gemini %d: %s", resp.StatusCode, string(x))
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
		return ocr.Result{}, err
	}
	if len(out.Candidates) > 0 && len(out.Candidates[0].Content.Parts) == 0 {
		return ocr.Result{}, nil
	}

	outJSON := strings.TrimSpace(out.Candidates[0].Content.Parts[0].Text)
	var r ocr.Result
	if err := json.Unmarshal([]byte(outJSON), &r); err != nil {
		// если модель прислала текст вместо JSON — сделаем мягкий фоллбэк
		r = ocr.Result{
			Text: outJSON,
		}
	}
	// нормализуем длину hints (до 3)
	if len(r.Hints) > 3 {
		r.Hints = r.Hints[:3]
	}
	if r.Hints == nil {
		r.Hints = []string{}
	}
	return r, nil
}
