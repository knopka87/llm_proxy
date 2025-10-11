package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"llm-proxy/api/internal/ocr"
	"llm-proxy/api/internal/prompt"
	"llm-proxy/api/internal/util"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type Engine struct {
	APIKey string
	Model  string
}

func New(apiKey, model string) *Engine {
	return &Engine{
		APIKey: strings.TrimSpace(apiKey),
		Model:  strings.TrimSpace(model),
	}
}

func (e *Engine) Name() string     { return "gemini" }
func (e *Engine) GetModel() string { return e.Model }
func (e *Engine) SetModel(m string) {
	if s := strings.TrimSpace(m); s != "" {
		e.Model = s
	}
}

// --------------------------- DETECT ---------------------------

// Detect Возвращает JSON по detect.schema.json.
func (e *Engine) Detect(ctx context.Context, img []byte, mime string, gradeHint int) (ocr.DetectResult, error) {
	if e.APIKey == "" {
		return ocr.DetectResult{}, errors.New("GEMINI_API_KEY is empty")
	}
	cl, err := genai.NewClient(ctx, option.WithAPIKey(e.APIKey))
	if err != nil {
		return ocr.DetectResult{}, err
	}
	defer cl.Close()

	m := cl.GenerativeModel(strings.TrimSpace(e.Model))
	if m == nil {
		return ocr.DetectResult{}, fmt.Errorf("gemini: model is nil")
	}
	// Возвращаем строго JSON
	m.GenerationConfig = genai.GenerationConfig{
		Temperature:      ptrFloat32(0),
		ResponseMIMEType: "application/json",
	}

	// system-инструкция + схема
	m.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text("Верни только JSON по detect.schema.json. Любой текст вне JSON — ошибка."),
			genai.Text("detect.schema.json:\n" + prompt.DetectSchema),
		},
	}

	// Пользовательский промпт: подсказки и картинка
	userText := "Определи пригодность фото для распознавания школьного задания. " +
		"Если несколько заданий — перечисли краткие описания. Верни JSON."
	if gradeHint >= 1 && gradeHint <= 4 {
		userText += fmt.Sprintf(" grade_hint=%d.", gradeHint)
	}

	parts := []genai.Part{
		genai.Text(userText),
		&genai.Blob{MIMEType: mime, Data: img},
	}

	// Ретраи на случай 5xx/транзиентных сбоёв
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := m.GenerateContent(ctx, parts...)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
			continue
		}
		txt := firstText(resp)
		if txt == "" {
			return ocr.DetectResult{}, fmt.Errorf("gemini detect: empty response")
		}
		txt = util.StripCodeFences(strings.TrimSpace(txt))

		var out ocr.DetectResult
		if err := json.Unmarshal([]byte(txt), &out); err != nil {
			return ocr.DetectResult{}, fmt.Errorf("gemini detect: bad JSON: %w", err)
		}
		return out, nil
	}
	return ocr.DetectResult{}, lastErr
}

// --------------------------- PARSE ---------------------------

// Parse Переписывает текст задания + вопрос. Возвращает JSON по parse.schema.json.
func (e *Engine) Parse(ctx context.Context, image []byte, opt ocr.ParseOptions) (ocr.ParseResult, error) {
	if e.APIKey == "" {
		return ocr.ParseResult{}, errors.New("GEMINI_API_KEY is empty")
	}
	cl, err := genai.NewClient(ctx, option.WithAPIKey(e.APIKey))
	if err != nil {
		return ocr.ParseResult{}, err
	}
	defer cl.Close()

	model := strings.TrimSpace(e.Model)
	m := cl.GenerativeModel(model)
	if m == nil {
		return ocr.ParseResult{}, fmt.Errorf("gemini: model is nil")
	}
	m.GenerationConfig = genai.GenerationConfig{
		Temperature:      ptrFloat32(0),
		ResponseMIMEType: "application/json",
	}

	// Системная часть: политика подтверждения и схема
	sys := `Ты — школьный ассистент 1–4 классов. Перепиши выбранное задание целиком (читаемо), не додумывай.
Отмечай нечитаемое в [квадратных скобках]. Выдели вопрос задачи.
Соблюдай политику подтверждения:
- Автоподтверждение, если: confidence ≥ 0.80, meaning_change_risk ≤ 0.20, bracketed_spans_count = 0, needs_rescan=false.
- Иначе запрашивай подтверждение.
Верни только JSON по parse.schema.json. Любой текст вне JSON — ошибка.`
	m.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(sys),
			genai.Text("parse.schema.json:\n" + prompt.ParseSchema),
		},
	}

	// Подсказки из опций (grade/subject/selected)
	var hints strings.Builder
	if opt.GradeHint >= 1 && opt.GradeHint <= 4 {
		fmt.Fprintf(&hints, " grade_hint=%d.", opt.GradeHint)
	}
	if s := strings.TrimSpace(opt.SubjectHint); s != "" {
		fmt.Fprintf(&hints, " subject_hint=%q.", s)
	}
	if opt.SelectedTaskIndex >= 0 || strings.TrimSpace(opt.SelectedTaskBrief) != "" {
		fmt.Fprintf(&hints, " selected_task=[index:%d, brief:%q].", opt.SelectedTaskIndex, opt.SelectedTaskBrief)
	}

	user := "Ответ строго JSON по parse.schema.json. Без комментариев." + hints.String()

	parts := []genai.Part{
		genai.Text(user),
		&genai.Blob{MIMEType: util.SniffMimeHTTP(image), Data: image},
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := m.GenerateContent(ctx, parts...)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
			continue
		}
		txt := firstText(resp)
		if txt == "" {
			return ocr.ParseResult{}, fmt.Errorf("gemini parse: empty response")
		}
		txt = util.StripCodeFences(strings.TrimSpace(txt))

		var pr ocr.ParseResult
		if err := json.Unmarshal([]byte(txt), &pr); err != nil {
			return ocr.ParseResult{}, fmt.Errorf("gemini parse: bad JSON: %w", err)
		}
		// Применяем серверную политику подтверждения (PROMPT_PARSE)
		ocr.ApplyParsePolicy(&pr)
		return pr, nil
	}
	return ocr.ParseResult{}, lastErr
}

// --------------------------- HINT ---------------------------

// Hint Генерирует L1/L2/L3 подсказку. Возвращает JSON по hint.schema.json.
func (e *Engine) Hint(ctx context.Context, in ocr.HintInput) (ocr.HintResult, error) {
	if e.APIKey == "" {
		return ocr.HintResult{}, errors.New("GEMINI_API_KEY is empty")
	}
	cl, err := genai.NewClient(ctx, option.WithAPIKey(e.APIKey))
	if err != nil {
		return ocr.HintResult{}, err
	}
	defer cl.Close()

	m := cl.GenerativeModel(strings.TrimSpace(e.Model))
	if m == nil {
		return ocr.HintResult{}, fmt.Errorf("gemini: model is nil")
	}
	m.GenerationConfig = genai.GenerationConfig{
		Temperature:      ptrFloat32(0),
		ResponseMIMEType: "application/json",
	}

	// System + схема
	sys := "Ты — помощник для 1–4 классов. Сформируй РОВНО ОДИН блок подсказки уровня " + string(in.Level) + ".\n" +
		"Не решай задачу и не подставляй числа/слова из условия. Вывод — строго JSON по hint.schema.json."
	m.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(sys),
			genai.Text("hint.schema.json:\n" + prompt.HintSchema),
		},
	}

	userObj := map[string]any{
		"task":  "Сгенерируй подсказку согласно PROMPT_HINT и верни JSON по hint.schema.json.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	parts := []genai.Part{
		genai.Text("INPUT_JSON:\n" + string(userJSON)),
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := m.GenerateContent(ctx, parts...)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
			continue
		}
		txt := firstText(resp)
		if txt == "" {
			return ocr.HintResult{}, fmt.Errorf("gemini hint: empty response")
		}
		txt = util.StripCodeFences(strings.TrimSpace(txt))

		var hr ocr.HintResult
		if err := json.Unmarshal([]byte(txt), &hr); err != nil {
			return ocr.HintResult{}, fmt.Errorf("gemini hint: bad JSON: %w", err)
		}
		hr.NoFinalAnswer = true
		return hr, nil
	}
	return ocr.HintResult{}, lastErr
}

// Normalize приводит ответ ученика к однозначной форме без догадок и без решения задачи.
// Строго возвращает JSON по normalize.schema.json (см. NORMALIZE_ANSWER v1.2).
func (e *Engine) Normalize(ctx context.Context, in ocr.NormalizeInput) (ocr.NormalizeResult, error) {
	if e.APIKey == "" {
		return ocr.NormalizeResult{}, errors.New("GEMINI_API_KEY is empty")
	}

	cl, err := genai.NewClient(ctx, option.WithAPIKey(e.APIKey))
	if err != nil {
		return ocr.NormalizeResult{}, err
	}
	defer cl.Close()

	m := cl.GenerativeModel(strings.TrimSpace(e.Model))
	if m == nil {
		return ocr.NormalizeResult{}, fmt.Errorf("gemini: model is nil")
	}
	m.GenerationConfig = genai.GenerationConfig{
		Temperature:      ptrFloat32(0),
		ResponseMIMEType: "application/json",
	}

	// Системные правила нормализации (кратко) + схема
	sys := `Ты — модуль нормализации ответа для 1–4 классов.
Извлеки РОВНО то, что прислал ребёнок, и представь это в форме solution_shape.
Строгие правила:
1) Не додумывать и не исправлять «как должно быть».
2) Не решать задачу и не выводить правильный ответ.
3) Минимальная чистка: убрать «Ответ:», мусор, унифицировать регистр/разделители.
4) Для shape=number число — в value, единицы — в units.detected/canonical.
5) Для string: нижний регистр, «ё» сохранять, дефис допустим, орфографию не чинить.
6) steps/list: 2–6 пунктов, не добавлять новых шагов.
7) Фото: OCR только для извлечения ответа; при плохом качестве — success=false и needs_clarification=true.
8) Несколько кандидатов — не выбирать; success=false, error="too_many_candidates" и короткое needs_user_action_message.
9) Неоднозначные форматы (½, 1 1/2, 1:20, 5–7, ≈10, >5) не сводить к арифметике; заполнить number_kind.
Верни СТРОГО JSON по normalize.schema.json.`
	m.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(sys),
			genai.Text("normalize.schema.json:\n" + prompt.NormalizeSchema),
		},
	}

	// Пользовательская часть: передаём вход как JSON, при фото добавляем Blob
	userObj := map[string]any{
		"task":  "Нормализуй ответ ученика и верни только JSON по normalize.schema.json.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	parts := []genai.Part{genai.Text("INPUT_JSON:\n" + string(userJSON))}
	if strings.EqualFold(in.Answer.Source, "photo") && len(in.Answer.PhotoB64) > 0 {
		mime := strings.TrimSpace(in.Answer.Mime)
		if mime == "" {
			mime = util.SniffMimeHTTP([]byte(in.Answer.PhotoB64))
		}
		parts = append(parts, &genai.Blob{MIMEType: mime, Data: []byte(in.Answer.PhotoB64)})
	}

	// Ретраи на случай временных ошибок
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := m.GenerateContent(ctx, parts...)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
			continue
		}
		raw := firstText(resp)
		if raw == "" {
			return ocr.NormalizeResult{}, fmt.Errorf("gemini normalize: empty response")
		}
		raw = util.StripCodeFences(strings.TrimSpace(raw))

		var nr ocr.NormalizeResult
		if err := json.Unmarshal([]byte(raw), &nr); err != nil {
			return ocr.NormalizeResult{}, fmt.Errorf("gemini normalize: bad JSON: %w", err)
		}
		return nr, nil
	}
	return ocr.NormalizeResult{}, lastErr
}

// --------------------------- helpers ---------------------------

func firstText(resp *genai.GenerateContentResponse) string {
	if resp == nil || len(resp.Candidates) == 0 {
		return ""
	}
	for _, c := range resp.Candidates {
		if c.Content == nil {
			continue
		}
		for _, p := range c.Content.Parts {
			if t, ok := p.(genai.Text); ok {
				return string(t)
			}
		}
	}
	return ""
}

func ptrFloat32(v float32) *float32 { return &v }
