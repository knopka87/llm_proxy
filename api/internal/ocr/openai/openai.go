package openai

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

func (e *Engine) Name() string { return "gpt" }

func (e *Engine) Analyze(ctx context.Context, image []byte, opt ocr.Options) (ocr.Result, error) {
	if e.APIKey == "" {
		return ocr.Result{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.Model
	if opt.Model != "" {
		model = opt.Model
	}
	mime := util.SniffMimeHTTP(image)
	b64 := base64.StdEncoding.EncodeToString(image)
	dataURL := util.MakeDataURL(mime, b64)

	system := `You analyze a PHOTO of a school task. Do:
1) Extract readable task text.
2) Detect whether there is a written solution on the photo.
3) If no solution: produce exactly 3 hints (L1..L3) guiding to solving, without final answer.
4) If a solution exists: check it and set verdict "correct" or "incorrect" (or "uncertain"); if incorrect, tell WHERE/WHAT KIND OF error (no final result); and produce 3 hints as above.
Respond with STRICT JSON:
{"text":string,"foundTask":bool,"foundSolution":bool,"solutionVerdict":"correct"|"incorrect"|"uncertain"|"","solutionNote":string,"hints":[string,string,string]}`

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "Analyze this image and return only the JSON described above."},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL, "detail": "high"}},
				},
			},
		},
		"temperature": 0,
	}
	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.Result{}, fmt.Errorf("openai %d: %s", resp.StatusCode, string(x))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ocr.Result{}, err
	}
	if len(out.Choices) == 0 {
		return ocr.Result{}, nil
	}

	outJSON := strings.TrimSpace(out.Choices[0].Message.Content)

	var r ocr.Result
	if err := json.Unmarshal([]byte(outJSON), &r); err != nil {
		r = ocr.Result{Text: outJSON}
	}
	if len(r.Hints) > 3 {
		r.Hints = r.Hints[:3]
	}
	if r.Hints == nil {
		r.Hints = []string{}
	}
	return r, nil
}
