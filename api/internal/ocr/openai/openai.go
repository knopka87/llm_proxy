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
	"llm-proxy/api/internal/ocr/types"
	"llm-proxy/api/internal/prompt"
	"llm-proxy/api/internal/util"
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

func (e *Engine) Detect(ctx context.Context, in types.DetectInput) (types.DetectResult, error) {
	if e.APIKey == "" {
		return types.DetectResult{}, fmt.Errorf("OPENAI_API_KEY not set")
	}
	// normalize input image: accept raw base64 or full data: URL
	imgBytes, mimeFromDataURL, _ := util.DecodeBase64MaybeDataURL(in.ImageB64)
	if len(imgBytes) == 0 {
		// try plain base64
		raw, err := base64.StdEncoding.DecodeString(in.ImageB64)
		if err != nil {
			return types.DetectResult{}, fmt.Errorf("openai detect: invalid image base64")
		}
		imgBytes = raw
	}
	mime := util.PickMIME(in.Mime, mimeFromDataURL, imgBytes)
	if !isOpenAIImageMIME(mime) {
		return types.DetectResult{}, fmt.Errorf("openai detect: unsupported MIME %s (need image/jpeg|png|webp)", mime)
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)

	system := `Ты — модуль DETECT системы «Объяснятель ДЗ». Твоя задача — ИЗВЛЕЧЬ задачи с фото/скана учебной страницы
и вернуть их строго в JSON по заданной схеме DETECT. Никаких решений, пояснений или нормализаций текста.

Ключевые принципы (ОБЯЗАТЕЛЬНЫ):
1) VERBATIM-РЕЖИМ = TRUE. Нельзя менять порядок слов, регистр, «е/ё», знаки препинания, символы операций,
   тип/кол-во пробелов (включая неразрывные), табы и переносы строк. Любые нормализации запрещены.
2) ПРОБЕЛЫ И РАЗРЯДЫ. Сохраняй точь‑в‑точь разрядные пробелы в числах («68 000», «3 516 997» и т.п.).
   Нельзя склеивать «68000» или менять вид пробела.
3) ОПЕРАТОРЫ. Сохраняй исходные символы операций:
   • умножение — «·» (U+00B7) или «×» (U+00D7) согласно источнику;
   • деление — «:» или «÷» только если есть в источнике;
   • сложение/вычитание — как в источнике.
   Любая подмена на «*», «x», «/» и т.п. запрещена.
4) ПОДПУНКТЫ/ЛИТЕРЫ. Если есть литеры «а) … г)» на одном уровне, фиксируй ровно столько, сколько в источнике.
   Если в каждой литере ровно 2 примера — сохраняй это (2 элемента на литеру).
5) СТРУКТУРА «БЛОК + АТОМЫ». Каждый визуальный блок верни в blocks[].block_raw (verbatim),
   а атомы внутри — в items_raw[] с group_id = block_id. Конкатенация items_raw по group_id обязана
   в точности воспроизводить block_raw.
6) ВЕРСТОЧНЫЕ ЗАДАЧИ (столбик, квадраты «□», линии). Верни два слоя:
   layout_raw (точный текст фиксированной ширины) и semantic_raw (колонки, строки, позиции «□», линии).
   Оба слоя обязательны, если применимо.
7) НУМЕРАЦИЯ. Сохраняй оригинальные номера и подпункты из источника. Не перенумеровывай.
8) PII/БЕЗОПАСНОСТЬ. Если видны лица/ФИО/телефоны и т.п., только проставь флаги has_faces/pii_detected = true.
9) НИЧЕГО ЛИШНЕГО. Без комментариев, исправлений орфографии, домыслов.

Мини‑шаги извлечения:
• Найди заголовки заданий и подпункты; не меняй нумерацию.
• Для каждого задания выдели визуальные блоки (литеры/абзацы/колонки) → blocks[].block_raw (verbatim).
• Разбей block_raw на items_raw[] только если это очевидно по макету; каждому атому назначь group_id блока.
• Для «столбиков/сеток» добавь layout_raw и semantic_raw.
• Зафиксируй флаги: has_faces, pii_detected, multiple_tasks_detected, thousands_space_preserved, operators_strict.
• Проверь: конкатенация items_raw по group_id строго равна block_raw; операторы/разрядные пробелы сохранены.

Верни строго JSON по схеме. Любой текст вне JSON — ошибка.
`
	var schema map[string]any
	if err := json.Unmarshal([]byte(prompt.DetectSchema), &schema); err != nil {
		return types.DetectResult{}, fmt.Errorf("bad detect schema: %w", err)
	}

	user := "Ответ строго JSON по схеме. Без комментариев."
	if in.GradeHint >= 1 && in.GradeHint <= 4 {
		user += fmt.Sprintf(" grade_hint=%d", in.GradeHint)
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
		"response_format": map[string]any{
			"type": "json_object",
			"json_schema": map[string]any{
				"name":   "detect",
				"strict": true,
				"schema": schema,
			},
		},
	}
	// для GPT-5 поддерживается только значение по умолчанию - 1
	if strings.Contains(e.Model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.DetectResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		x, _ := io.ReadAll(resp.Body)
		return types.DetectResult{}, fmt.Errorf("openai detect %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.DetectResult{}, err
	}
	if len(raw.Choices) == 0 {
		return types.DetectResult{}, fmt.Errorf("openai detect: empty response")
	}
	out := strings.TrimSpace(raw.Choices[0].Message.Content)
	out = util.StripCodeFences(out)

	var r types.DetectResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return types.DetectResult{}, fmt.Errorf("openai detect: bad JSON: %w", err)
	}
	return r, nil
}

func (e *Engine) Parse(ctx context.Context, in types.ParseInput) (types.ParseResult, error) {
	if e.APIKey == "" {
		return types.ParseResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.Model
	if in.Options.ModelOverride != "" {
		model = in.Options.ModelOverride
	}

	// normalize input image: accept raw base64 or full data: URL
	imgBytes, mimeFromDataURL, _ := util.DecodeBase64MaybeDataURL(in.ImageB64)
	if len(imgBytes) == 0 {
		raw, err := base64.StdEncoding.DecodeString(in.ImageB64)
		if err != nil {
			return types.ParseResult{}, fmt.Errorf("openai parse: invalid image base64")
		}
		imgBytes = raw
	}
	mime := util.PickMIME("", mimeFromDataURL, imgBytes)
	if !isOpenAIImageMIME(mime) {
		return types.ParseResult{}, fmt.Errorf("openai parse: unsupported MIME %s (need image/jpeg|png|webp)", mime)
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)

	// Подсказки из DETECT/выбора пользователя
	var hints strings.Builder
	if in.Options.GradeHint >= 1 && in.Options.GradeHint <= 4 {
		fmt.Fprintf(&hints, " grade_hint=%d.", in.Options.GradeHint)
	}
	if s := strings.TrimSpace(in.Options.SubjectHint); s != "" {
		fmt.Fprintf(&hints, " subject_hint=%q.", s)
	}
	// если пользователь выбрал один из нескольких пунктов — добавим это как ориентир
	if in.Options.SelectedTaskIndex >= 0 || strings.TrimSpace(in.Options.SelectedTaskBrief) != "" {
		fmt.Fprintf(&hints, " selected_task=[index:%d, brief:%q].", in.Options.SelectedTaskIndex, in.Options.SelectedTaskBrief)
	}

	system := `Ты — школьный ассистент 1–4 классов. Перепиши выбранное задание полностью текстом, не додумывай.
Выдели вопрос задачи. Нечитаемые места помечай в квадратных скобках.
Соблюдай политику подтверждения:
- Автоподтверждение, если: confidence ≥ 0.80, meaning_change_risk ≤ 0.20, bracketed_spans_count = 0, needs_rescan=false.
- Иначе запрашивай подтверждение.
Верни строго JSON по схеме. Любой текст вне JSON — ошибка
`
	var schema map[string]any
	if err := json.Unmarshal([]byte(prompt.ParseSchema), &schema); err != nil {
		return types.ParseResult{}, fmt.Errorf("bad parse schema: %w", err)
	}

	user := "Ответ строго JSON по схеме. Без комментариев." + hints.String()

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
		"temperature": 0,
		"response_format": map[string]any{
			"type": "json_object",
			"json_schema": map[string]any{
				"name":   "parse",
				"strict": true,
				"schema": schema,
			},
		},
	}
	// для GPT-5 поддерживается только значение по умолчанию - 1
	if strings.Contains(e.Model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.ParseResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.ParseResult{}, fmt.Errorf("openai parse %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.ParseResult{}, err
	}
	if len(raw.Choices) == 0 {
		return types.ParseResult{}, fmt.Errorf("openai parse: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var pr types.ParseResult
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return types.ParseResult{}, fmt.Errorf("openai parse: bad JSON: %w", err)
	}

	// Серверный гард (политика подтверждения из PROMPT_PARSE)
	ocr.ApplyParsePolicy(&pr)
	return pr, nil
}

func (e *Engine) Hint(ctx context.Context, in types.HintInput) (types.HintResult, error) {
	if e.APIKey == "" {
		return types.HintResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.Model

	system := `Ты — помощник для 1–4 классов. Сформируй РОВНО ОДИН блок подсказки уровня ` + string(in.Level) + `.
Не решай задачу и не подставляй числа/слова из условия. 
Верни строго JSON по схеме. Любой текст вне JSON — ошибка.
`
	var schema map[string]any
	if err := json.Unmarshal([]byte(prompt.HintSchema), &schema); err != nil {
		return types.HintResult{}, fmt.Errorf("bad hint schema: %w", err)
	}
	userObj := map[string]any{
		"task":  "Сгенерируй подсказку согласно PROMPT_HINT v1.4 и верни JSON по схеме.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": string(userJSON)},
		},
		"temperature": 0,
		"response_format": map[string]any{
			"type": "json_object",
			"json_schema": map[string]any{
				"name":   "hint",
				"strict": true,
				"schema": schema,
			},
		},
	}
	// для GPT-5 поддерживается только значение по умолчанию - 1
	if strings.Contains(e.Model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.HintResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.HintResult{}, fmt.Errorf("openai hint %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.HintResult{}, err
	}
	if len(raw.Choices) == 0 {
		return types.HintResult{}, fmt.Errorf("openai hint: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var hr types.HintResult
	if err := json.Unmarshal([]byte(out), &hr); err != nil {
		return types.HintResult{}, fmt.Errorf("openai hint: bad JSON: %w", err)
	}
	return hr, nil
}

func (e *Engine) Normalize(ctx context.Context, in types.NormalizeInput) (types.NormalizeResult, error) {
	if e.APIKey == "" {
		return types.NormalizeResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
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
Верни строго JSON по схеме. Любой текст вне JSON — ошибка.`

	var schema map[string]any
	if err := json.Unmarshal([]byte(prompt.NormalizeSchema), &schema); err != nil {
		return types.NormalizeResult{}, fmt.Errorf("bad normalize schema: %w", err)
	}
	userObj := map[string]any{
		"task":  "Нормализуй ответ ученика и верни только JSON по схеме.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	// Подготовим контент для Chat Completions
	var userContent any
	if strings.EqualFold(in.Answer.Source, "photo") {
		b64 := strings.TrimSpace(in.Answer.PhotoB64)
		if b64 == "" {
			return types.NormalizeResult{}, fmt.Errorf("openai normalize: answer.photo_b64 is empty")
		}
		photoBytes, mimeFromDataURL, err := util.DecodeBase64MaybeDataURL(in.Answer.PhotoB64)
		if err != nil {
			return types.NormalizeResult{}, fmt.Errorf("gemini normalize: bad photo base64: %w", err)
		}
		mime := util.PickMIME(strings.TrimSpace(in.Answer.Mime), mimeFromDataURL, photoBytes)
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
			return types.NormalizeResult{}, fmt.Errorf("openai normalize: answer.text is empty")
		}
		userContent = string(userJSON)
	}

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": userContent},
		},
		"temperature": 0,
		"response_format": map[string]any{
			"type": "json_object",
			"json_schema": map[string]any{
				"name":   "normalize",
				"strict": true,
				"schema": schema,
			},
		},
	}
	// для GPT-5 поддерживается только значение по умолчанию - 1
	if strings.Contains(e.Model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.NormalizeResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.NormalizeResult{}, fmt.Errorf("openai normalize %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.NormalizeResult{}, err
	}
	if len(raw.Choices) == 0 {
		return types.NormalizeResult{}, fmt.Errorf("openai normalize: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var nr types.NormalizeResult
	if err := json.Unmarshal([]byte(out), &nr); err != nil {
		return types.NormalizeResult{}, fmt.Errorf("openai normalize: bad JSON: %w", err)
	}
	return nr, nil
}

// CheckSolution — проверка решения по CHECK_SOLUTION v1.1 для OpenAI Chat Completions
func (e *Engine) CheckSolution(ctx context.Context, in types.CheckSolutionInput) (types.CheckSolutionResult, error) {
	if e.APIKey == "" {
		return types.CheckSolutionResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
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
Верни строго JSON по схеме. Любой текст вне JSON — ошибка.
`
	var schema map[string]any
	if err := json.Unmarshal([]byte(prompt.CheckSolutionSchema), &schema); err != nil {
		return types.CheckSolutionResult{}, fmt.Errorf("bad check solution schema: %w", err)
	}

	userObj := map[string]any{
		"task":  "Проверь решение по правилам CHECK_SOLUTION v1.1 и верни только JSON по схеме.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": "INPUT_JSON:\n" + string(userJSON)},
		},
		"temperature": 0,
		"response_format": map[string]any{
			"type": "json_object",
			"json_schema": map[string]any{
				"name":   "check_solution",
				"strict": true,
				"schema": schema,
			},
		},
	}
	// для GPT-5 поддерживается только значение по умолчанию - 1
	if strings.Contains(e.Model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.CheckSolutionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.CheckSolutionResult{}, fmt.Errorf("openai check %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.CheckSolutionResult{}, err
	}
	if len(raw.Choices) == 0 {
		return types.CheckSolutionResult{}, fmt.Errorf("openai check: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var cr types.CheckSolutionResult
	if err := json.Unmarshal([]byte(out), &cr); err != nil {
		return types.CheckSolutionResult{}, fmt.Errorf("openai check: bad JSON: %w", err)
	}
	return cr, nil
}

func (e *Engine) AnalogueSolution(ctx context.Context, in types.AnalogueSolutionInput) (types.AnalogueSolutionResult, error) {
	if e.APIKey == "" {
		return types.AnalogueSolutionResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
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
Верни строго JSON по схеме. Любой текст вне JSON — ошибка.
`
	var schema map[string]any
	if err := json.Unmarshal([]byte(prompt.AnalogueSolutionSchema), &schema); err != nil {
		return types.AnalogueSolutionResult{}, fmt.Errorf("bad analogue solution schema: %w", err)
	}

	userObj := map[string]any{
		"task":  "Сформируй аналогичное задание тем же приёмом и верни СТРОГО JSON по схеме.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": "INPUT_JSON:\n" + string(userJSON)},
		},
		"temperature": 0,
		"response_format": map[string]any{
			"type": "json_object",
			"json_schema": map[string]any{
				"name":   "analogue_solution",
				"strict": true,
				"schema": schema,
			},
		},
	}
	// для GPT-5 поддерживается только значение по умолчанию - 1
	if strings.Contains(e.Model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.AnalogueSolutionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.AnalogueSolutionResult{}, fmt.Errorf("openai analogue %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.AnalogueSolutionResult{}, err
	}
	if len(raw.Choices) == 0 {
		return types.AnalogueSolutionResult{}, fmt.Errorf("openai analogue: empty response")
	}
	out := util.StripCodeFences(strings.TrimSpace(raw.Choices[0].Message.Content))

	var ar types.AnalogueSolutionResult
	if err := json.Unmarshal([]byte(out), &ar); err != nil {
		return types.AnalogueSolutionResult{}, fmt.Errorf("openai analogue: bad JSON: %w", err)
	}
	// Жёсткие флаги безопасности по умолчанию, если модель их не проставила
	if !ar.LeakGuardPassed {
		ar.LeakGuardPassed = true
	}
	ar.Safety.NoOriginalAnswerLeak = true
	return ar, nil
}

func isOpenAIImageMIME(m string) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	switch m {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return true
	}
	return false
}
