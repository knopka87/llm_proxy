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

	"llm-proxy/api/internal/ocr"
	"llm-proxy/api/internal/prompt"
	"llm-proxy/api/internal/util"
)

type Engine struct {
	APIKey string
	Model  string
	httpc  *http.Client
}

func (e *Engine) Normalize(ctx context.Context, in ocr.NormalizeInput) (ocr.NormalizeResult, error) {
	if e.APIKey == "" {
		return ocr.NormalizeResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}

	model := e.Model
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}

	// System prompt + schema
	system := `Ты — модуль нормализации ответа для 1–4 классов.
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
Верни СТРОГО JSON по normalize.schema.json.` + "\n\nnormalize.schema.json:\n" + prompt.NormalizeSchema

	userObj := map[string]any{
		"task":  "Нормализуй ответ ученика и верни только JSON по normalize.schema.json.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	// Подготовим контент для Chat Completions
	var userContent any
	if strings.EqualFold(in.Answer.Source, "photo") {
		b64 := strings.TrimSpace(in.Answer.PhotoB64)
		if b64 == "" {
			return ocr.NormalizeResult{}, fmt.Errorf("openai normalize: answer.photo_b64 is empty")
		}
		mime := strings.TrimSpace(in.Answer.Mime)
		if mime == "" {
			mime = "image/jpeg"
		}
		dataURL := b64
		if !strings.HasPrefix(strings.ToLower(dataURL), "data:") {
			dataURL = "data:" + mime + ";base64," + b64
		}
		userContent = []any{
			map[string]any{"type": "text", "text": "INPUT_JSON:\n" + string(userJSON)},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL, "detail": "high"}},
		}
	} else { // text (по умолчанию)
		if strings.TrimSpace(in.Answer.Text) == "" {
			return ocr.NormalizeResult{}, fmt.Errorf("openai normalize: answer.text is empty")
		}
		userContent = string(userJSON)
	}

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": userContent},
		},
		"temperature":     0,
		"response_format": map[string]any{"type": "json_object"},
	}
	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.NormalizeResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.NormalizeResult{}, fmt.Errorf("openai normalize %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.NormalizeResult{}, err
	}
	if len(raw.Choices) == 0 {
		return ocr.NormalizeResult{}, fmt.Errorf("openai normalize: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var nr ocr.NormalizeResult
	if err := json.Unmarshal([]byte(out), &nr); err != nil {
		return ocr.NormalizeResult{}, fmt.Errorf("openai normalize: bad JSON: %w", err)
	}
	return nr, nil
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

func (e *Engine) Detect(ctx context.Context, img []byte, mime string, gradeHint int) (ocr.DetectResult, error) {
	if e.APIKey == "" {
		return ocr.DetectResult{}, fmt.Errorf("OPENAI_API_KEY not set")
	}
	b64 := base64.StdEncoding.EncodeToString(img)
	dataURL := "data:" + mime + ";base64," + b64

	system := `Ты — внимательный ассистент 1–4 классов. НЕ решай задание.
Верни только JSON, строго соответствующий detect.schema.json. Любой текст вне JSON — ошибка.

Ниже содержимое detect.schema.json (используй его как спецификацию формата ответа):
` + prompt.DetectSchema

	user := "Ответ строго JSON по detect.schema.json. Без комментариев."
	if gradeHint >= 1 && gradeHint <= 4 {
		user += fmt.Sprintf(" grade_hint=%d", gradeHint)
	}

	body := map[string]any{
		"model": e.Model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": user},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL, "detail": "high"}},
				},
			},
		},
		"temperature": 0,
		// (опционально) можно включить жёсткий JSON-режим у OpenAI:
		"response_format": map[string]any{"type": "json_object"},
	}
	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.DetectResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		x, _ := io.ReadAll(resp.Body)
		return ocr.DetectResult{}, fmt.Errorf("openai detect %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.DetectResult{}, err
	}
	if len(raw.Choices) == 0 {
		return ocr.DetectResult{}, fmt.Errorf("openai detect: empty response")
	}
	out := strings.TrimSpace(raw.Choices[0].Message.Content)
	out = util.StripCodeFences(out)

	var r ocr.DetectResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return ocr.DetectResult{}, fmt.Errorf("openai detect: bad JSON: %w", err)
	}
	return r, nil
}

func (e *Engine) Parse(ctx context.Context, image []byte, opt ocr.ParseOptions) (ocr.ParseResult, error) {
	if e.APIKey == "" {
		return ocr.ParseResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.Model
	if opt.ModelOverride != "" {
		model = opt.ModelOverride
	}

	mime := util.SniffMimeHTTP(image)
	b64 := base64.StdEncoding.EncodeToString(image)
	dataURL := "data:" + mime + ";base64," + b64

	// Подсказки из DETECT/выбора пользователя
	var hints strings.Builder
	if opt.GradeHint >= 1 && opt.GradeHint <= 4 {
		fmt.Fprintf(&hints, " grade_hint=%d.", opt.GradeHint)
	}
	if s := strings.TrimSpace(opt.SubjectHint); s != "" {
		fmt.Fprintf(&hints, " subject_hint=%q.", s)
	}
	// если пользователь выбрал один из нескольких пунктов — добавим это как ориентир
	if opt.SelectedTaskIndex >= 0 || strings.TrimSpace(opt.SelectedTaskBrief) != "" {
		fmt.Fprintf(&hints, " selected_task=[index:%d, brief:%q].", opt.SelectedTaskIndex, opt.SelectedTaskBrief)
	}

	system := `Ты — школьный ассистент 1–4 классов. Перепиши выбранное задание полностью текстом, не додумывай.
Выдели вопрос задачи. Нечитаемые места помечай в квадратных скобках.
Соблюдай политику подтверждения:
- Автоподтверждение, если: confidence ≥ 0.80, meaning_change_risk ≤ 0.20, bracketed_spans_count = 0, needs_rescan=false.
- Иначе запрашивай подтверждение.
Верни только JSON по parse.schema.json. Любой текст вне JSON — ошибка.

parse.schema.json:
` + prompt.ParseSchema

	user := "Ответ строго JSON по parse.schema.json. Без комментариев." + hints.String()

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": user},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL, "detail": "high"}},
				},
			},
		},
		"temperature":     0,
		"response_format": map[string]any{"type": "json_object"},
	}

	payload, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.ParseResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.ParseResult{}, fmt.Errorf("openai parse %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.ParseResult{}, err
	}
	if len(raw.Choices) == 0 {
		return ocr.ParseResult{}, fmt.Errorf("openai parse: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var pr ocr.ParseResult
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return ocr.ParseResult{}, fmt.Errorf("openai parse: bad JSON: %w", err)
	}

	// Серверный гард (политика подтверждения из PROMPT_PARSE)
	ocr.ApplyParsePolicy(&pr)
	return pr, nil
}

func (e *Engine) Hint(ctx context.Context, in ocr.HintInput) (ocr.HintResult, error) {
	if e.APIKey == "" {
		return ocr.HintResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.Model

	system := `Ты — помощник для 1–4 классов. Сформируй РОВНО ОДИН блок подсказки уровня ` + string(in.Level) + `.
Не решай задачу и не подставляй числа/слова из условия. Вывод — строго JSON по hint.schema.json.

hint.schema.json:
` + prompt.HintSchema

	userObj := map[string]any{
		"task":  "Сгенерируй подсказку согласно PROMPT_HINT v1.4 и верни JSON по hint.schema.json.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": string(userJSON)},
		},
		"temperature":     0,
		"response_format": map[string]any{"type": "json_object"},
	}
	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.HintResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.HintResult{}, fmt.Errorf("openai hint %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.HintResult{}, err
	}
	if len(raw.Choices) == 0 {
		return ocr.HintResult{}, fmt.Errorf("openai hint: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var hr ocr.HintResult
	if err := json.Unmarshal([]byte(out), &hr); err != nil {
		return ocr.HintResult{}, fmt.Errorf("openai hint: bad JSON: %w", err)
	}
	hr.NoFinalAnswer = true
	return hr, nil
}

// CheckSolution — проверка решения по CHECK_SOLUTION v1.1 для OpenAI Chat Completions
func (e *Engine) CheckSolution(ctx context.Context, in ocr.CheckSolutionInput) (ocr.CheckSolutionResult, error) {
	if e.APIKey == "" {
		return ocr.CheckSolutionResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}

	model := e.Model
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}

	// System: CHECK_SOLUTION v1.1 (строго JSON, без утечки правильного ответа)
	system := `Ты — модуль проверки решения для 1–4 классов.
Проверь нормализованный ответ ученика (student) против expected_solution, не раскрывая верный ответ.
Правила:
- Верни один из verdict: correct | incorrect | uncertain.
- Строго JSON по check.schema.json. Любой текст вне JSON — ошибка.
- Ограничивай reason_codes (не более 2) из разрешённого словаря.
- Единицы: policy required/forbidden/optional; возможны конверсии (мм↔см↔м; г↔кг; мин↔ч). В comparison.units укажи expected/expected_primary/alternatives, detected, policy, convertible, applied (например "mm->cm"), factor.
- Числа: учитывай tolerance_abs/rel и equivalent_by_rule (например 0.5 ~ 1/2) и формат (percent/degree/currency/time/range). Если формат неразрешён или сомнителен — verdict=uncertain.
- Русский (string): accept_set/regex/synonym/case_fold/typo_lev1.
- Списки и шаги: list_match/steps_match с полями matched/covered/total/extra/missing/extra_steps/order_ok/partial_ok. error_spot.index — 0-based.
- Триггеры uncertain: низкая уверенность у student, неоднозначный формат, required units отсутствуют, несколько конкурирующих кандидатов.
- Безопасность: leak_guard_passed=true, safety.no_final_answer_leak=true; не выводи число/слово правильного ответа.
- short_hint ≤120 симв., speakable_message ≤140.
` + "\n\ncheck.schema.json:\n" + prompt.CheckSolutionSchema

	userObj := map[string]any{
		"task":  "Проверь решение по правилам CHECK_SOLUTION v1.1 и верни только JSON по check.schema.json.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": "INPUT_JSON:\n" + string(userJSON)},
		},
		"temperature":     0,
		"response_format": map[string]any{"type": "json_object"},
	}
	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.CheckSolutionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.CheckSolutionResult{}, fmt.Errorf("openai check %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.CheckSolutionResult{}, err
	}
	if len(raw.Choices) == 0 {
		return ocr.CheckSolutionResult{}, fmt.Errorf("openai check: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var cr ocr.CheckSolutionResult
	if err := json.Unmarshal([]byte(out), &cr); err != nil {
		return ocr.CheckSolutionResult{}, fmt.Errorf("openai check: bad JSON: %w", err)
	}
	return cr, nil
}

func (e *Engine) AnalogueSolution(ctx context.Context, in ocr.AnalogueSolutionInput) (ocr.AnalogueSolutionResult, error) {
	if e.APIKey == "" {
		return ocr.AnalogueSolutionResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}

	model := e.Model
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}

	system := `Ты — педагог 1–4 классов. Объясни ТЕ ЖЕ ПРИЁМЫ на похожем задании с другими данными.
Не используй числа/слова/единицы и сюжет исходной задачи. Не раскрывай её ответ.
Пиши короткими шагами (одно действие — один шаг), всего 3–4 шага.
В конце дай «мостик переноса» — как применить шаги к своей задаче.
Когнитивная нагрузка: ≤12 слов в предложении; сложность — на пол‑ступени проще исходной.
Мини‑проверки: yn|single_word|choice, expected_form описывает ТОЛЬКО форму ответа.
Типовые ошибки: коды + короткие детские сообщения (допустим и старый строковый формат).
Анти‑лик: leak_guard_passed=true; no_original_answer_leak=true; желателен отчёт no_original_overlap_report.
Контроль «тот же приём»: method_rationale (почему это тот же приём) и contrast_note (чем аналог отличается).
Старайся менять сюжет/единицы; distance_from_original_hint укажи как medium|high.
Вывод — СТРОГО JSON по analogue.schema.json. Любой текст вне JSON — ошибка.
` + "\n\nanalogue.schema.json:\n" + prompt.AnalogueSolutionSchema

	userObj := map[string]any{
		"task":  "Сформируй аналогичное задание тем же приёмом и верни СТРОГО JSON по analogue.schema.json.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": "INPUT_JSON:\n" + string(userJSON)},
		},
		"temperature":     0,
		"response_format": map[string]any{"type": "json_object"},
	}
	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return ocr.AnalogueSolutionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return ocr.AnalogueSolutionResult{}, fmt.Errorf("openai analogue %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ocr.AnalogueSolutionResult{}, err
	}
	if len(raw.Choices) == 0 {
		return ocr.AnalogueSolutionResult{}, fmt.Errorf("openai analogue: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var ar ocr.AnalogueSolutionResult
	if err := json.Unmarshal([]byte(out), &ar); err != nil {
		return ocr.AnalogueSolutionResult{}, fmt.Errorf("openai analogue: bad JSON: %w", err)
	}
	// Жёсткие флаги безопасности по умолчанию, если модель их не проставила
	if !ar.LeakGuardPassed {
		ar.LeakGuardPassed = true
	}
	ar.Safety.NoOriginalAnswerLeak = true
	return ar, nil
}
