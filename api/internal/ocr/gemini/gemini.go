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

func (e *Engine) Hint(ctx context.Context, in ocr.HintInput) (ocr.HintResult, error) {
	if e.APIKey == "" {
		return ocr.HintResult{}, fmt.Errorf("GEMINI_API_KEY is empty")
	}
	model := e.Model
	mime := "application/json"

	// system-инструкция кратко фиксирует правила «No Final Answer» и JSON-вывод
	system := `Ты — помощник для 1–4 классов. Сформируй РОВНО ОДИН блок подсказки уровня ` + string(in.Level) + `.
Не решай задачу и не подставляй числа/слова из условия. Вывод — строго JSON по hint.schema.json.`

	// Тело «user»: прикладываем схему и входные данные как JSON
	userObj := map[string]any{
		"task":  "Сгенерируй подсказку согласно PROMPT_HINT v1.4 и верни JSON по hint.schema.json.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	body := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": system},
					map[string]any{"text": "hint.schema.json:\n" + prompt.HintSchema},
					map[string]any{"inline_data": map[string]any{
						"mime_type": mime,
						"data":      base64.StdEncoding.EncodeToString(userJSON),
					}},
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
		return ocr.HintResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.HintResult{}, fmt.Errorf("gemini hint %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
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
		return ocr.HintResult{}, err
	}
	if len(raw.Candidates) == 0 || len(raw.Candidates[0].Content.Parts) == 0 {
		return ocr.HintResult{}, fmt.Errorf("gemini hint: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Candidates[0].Content.Parts[0].Text))

	var hr ocr.HintResult
	if err := json.Unmarshal([]byte(out), &hr); err != nil {
		return ocr.HintResult{}, fmt.Errorf("gemini hint: bad JSON: %w", err)
	}
	// Страховка: no_final_answer должен быть true
	hr.NoFinalAnswer = true
	return hr, nil
}
