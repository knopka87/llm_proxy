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
	"child-bot/api/internal/prompt"
	"child-bot/api/internal/util"
)

type Engine struct {
	APIKey string
	Model  string
	Base   string // "https://generativelanguage.googleapis.com/v1"
	httpc  *http.Client
}

func New(key, model, base string) *Engine {
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1"
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &Engine{
		APIKey: key,
		Model:  model,
		Base:   strings.TrimRight(base, "/"),
		httpc:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *Engine) Name() string { return "gemini" }

func (e *Engine) GetModel() string { return e.Model }

func (e *Engine) Detect(ctx context.Context, img []byte, mime string, gradeHint int) (ocr.DetectResult, error) {
	if e.APIKey == "" {
		return ocr.DetectResult{}, fmt.Errorf("GEMINI_API_KEY not set")
	}
	b64 := base64.StdEncoding.EncodeToString(img)

	system := `Ты — внимательный ассистент 1–4 классов. Проанализируй изображение и НЕ решай задание. 
Определи: есть ли учебное задание; одно или несколько; годность фото; неприемлемость; есть ли уже написанное решение/черновик. 
Верни только JSON по detect.schema.json. Любой текст вне JSON — ошибка.`

	user := "Ответ строго JSON по detect.schema.json. Без комментариев."
	if gradeHint >= 1 && gradeHint <= 4 {
		user += fmt.Sprintf(" grade_hint=%d", gradeHint)
	}

	body := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					// 1) инструкция
					map[string]any{"text": system},
					// 2) прикладываем схему
					map[string]any{"text": "detect.schema.json:\n" + prompt.DetectSchema},
					// 3) картинка
					map[string]any{"inline_data": map[string]any{
						"mime_type": mime,
						"data":      b64,
					}},
					// 4) короткая просьба-суммаризатор
					map[string]any{"text": user},
				},
			},
		},
		"generationConfig": map[string]any{"temperature": 0},
	}
	payload, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", e.Base, e.Model, e.APIKey)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.DetectResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.DetectResult{}, fmt.Errorf("gemini detect %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}
	var raw struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.DetectResult{}, err
	}
	if len(raw.Candidates) == 0 || len(raw.Candidates[0].Content.Parts) == 0 {
		return ocr.DetectResult{}, fmt.Errorf("gemini detect: empty response")
	}
	out := strings.TrimSpace(raw.Candidates[0].Content.Parts[0].Text)
	out = util.StripCodeFences(out)

	var r ocr.DetectResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return ocr.DetectResult{}, fmt.Errorf("gemini detect: bad JSON: %w", err)
	}
	return r, nil
}

func (e *Engine) Parse(ctx context.Context, image []byte, opt ocr.ParseOptions) (ocr.ParseResult, error) {
	if e.APIKey == "" {
		return ocr.ParseResult{}, fmt.Errorf("GEMINI_API_KEY is empty")
	}
	model := e.Model
	if opt.ModelOverride != "" {
		model = opt.ModelOverride
	}
	mime := util.SniffMimeHTTP(image)
	b64 := base64.StdEncoding.EncodeToString(image)

	// Подсказки из DETECT/регистрации
	hints := ""
	if opt.GradeHint >= 1 && opt.GradeHint <= 4 {
		hints += fmt.Sprintf(" grade_hint=%d.", opt.GradeHint)
	}
	if strings.TrimSpace(opt.SubjectHint) != "" {
		hints += fmt.Sprintf(" subject_hint=%q.", opt.SubjectHint)
	}

	system := `Ты — школьный ассистент 1–4 классов. Перепиши выбранное задание полностью текстом, не додумывай.
Выдели вопрос задачи. Нечитаемые места помечай в квадратных скобках.
Соблюдай политику подтверждения:
- Автоподтверждение, если: confidence ≥ 0.80, meaning_change_risk ≤ 0.20, bracketed_spans_count = 0, needs_rescan=false.
- Иначе запрашивай подтверждение.
Верни только JSON по parse.schema.json. Любой текст вне JSON — ошибка.`

	user := "Ответ строго JSON по parse.schema.json. Без комментариев." + hints

	body := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": system},
					map[string]any{"text": "parse.schema.json:\n" + prompt.ParseSchema},
					map[string]any{"inline_data": map[string]any{
						"mime_type": mime,
						"data":      b64,
					}},
					map[string]any{"text": user},
				},
			},
		},
		"generationConfig": map[string]any{"temperature": 0},
	}
	payload, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", e.Base, model, e.APIKey)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.ParseResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.ParseResult{}, fmt.Errorf("gemini parse %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.ParseResult{}, err
	}
	if len(raw.Candidates) == 0 || len(raw.Candidates[0].Content.Parts) == 0 {
		return ocr.ParseResult{}, fmt.Errorf("gemini parse: empty response")
	}
	out := strings.TrimSpace(raw.Candidates[0].Content.Parts[0].Text)
	out = util.StripCodeFences(out)

	var pr ocr.ParseResult
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return ocr.ParseResult{}, fmt.Errorf("gemini parse: bad JSON: %w", err)
	}

	// Серверный гард (политика подтверждения из PROMPT_PARSE)
	ocr.ApplyParsePolicy(&pr)
	return pr, nil
}

// Hint генерирует подсказку L1/L2/L3 (JSON) по PROMPT_HINT.
// Делает 3 ретрая на 5xx и НЕ использует inline_data.
func (e *Engine) Hint(ctx context.Context, in ocr.HintInput) (ocr.HintResult, error) {
	if strings.TrimSpace(e.APIKey) == "" {
		return ocr.HintResult{}, fmt.Errorf("GEMINI_API_KEY is empty")
	}
	model := e.Model
	// на случай если кто-то передал "models/<id>"
	model = strings.TrimPrefix(model, "models/")

	system := "Ты — помощник для 1–4 классов. Сформируй РОВНО ОДИН блок подсказки уровня " + string(in.Level) + ".\n" +
		"Не решай задачу и не подставляй числа/слова из условия. Вывод — строго JSON по hint.schema.json."

	userObj := map[string]any{
		"task":  "Сгенерируй подсказку по PROMPT_HINT и верни JSON по hint.schema.json.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	// Составим один текстовый промпт: system + схема + JSON входа.
	promptText := strings.Join([]string{
		system,
		"",
		"hint.schema.json:",
		prompt.HintSchema,
		"",
		"INPUT_JSON:",
		string(userJSON),
	}, "\n")

	body := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": promptText},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":        0,
			"response_mime_type": "application/json", // ← JSON mode
		},
	}

	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", e.Base, model, e.APIKey)

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")

		resp, err := e.httpc.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 500 { // retryable
			lastErr = fmt.Errorf("gemini hint %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return ocr.HintResult{}, fmt.Errorf("gemini hint %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}

		var raw struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			return ocr.HintResult{}, err
		}
		if len(raw.Candidates) == 0 || len(raw.Candidates[0].Content.Parts) == 0 {
			return ocr.HintResult{}, fmt.Errorf("gemini hint: empty response")
		}
		out := strings.TrimSpace(raw.Candidates[0].Content.Parts[0].Text)
		out = util.StripCodeFences(out)

		var hr ocr.HintResult
		if err := json.Unmarshal([]byte(out), &hr); err != nil {
			return ocr.HintResult{}, fmt.Errorf("gemini hint: bad JSON: %w", err)
		}
		hr.NoFinalAnswer = true
		return hr, nil
	}
	return ocr.HintResult{}, lastErr
}
