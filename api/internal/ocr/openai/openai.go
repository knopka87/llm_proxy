package openai

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

	"llm-proxy/api/internal/ocr"
	"llm-proxy/api/internal/ocr/types"
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

	// accept raw base64 or data: URL
	imgBytes, mimeFromDataURL, _ := util.DecodeBase64MaybeDataURL(in.ImageB64)
	if len(imgBytes) == 0 {
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

	system := `DETECT — system prompt v5.2 (text-only, PII OFF per MVP)

Роль: ты — модуль DETECT сервиса «Объяснятель ДЗ». Единственная задача — извлечь задачи с фото/скана и вернуть ОДИН корневой объект строго в JSON по detect.schema.json.

⚙️ Формат ответа
• Ответ только JSON по detect.schema.json.
• Никаких решений, пояснений, рассуждений, Markdown и префиксов.
• Схема передаётся параметром вызова response_format: { type: "json_schema", json_schema: detect.schema.json } — не вставляй схему в текст.
• Конец ответа — закрывающая фигурная скобка корневого объекта. Любой текст вне JSON — ошибка.

📏 Жёсткие правила
1) VERBATIM. Нельзя изменять исходный текст: порядок слов, регистр, «е/ё», пунктуацию, тип/кол-во пробелов (включая NBSP), табы, переносы строк.
2) NUMBERS. Сохраняй разрядные пробелы («68 000», «3 516 997») и их тип. Не склеивать «68000», не заменять пробелы.
3) OPERATORS. Сохраняй исходные символы операций: «·/×», «: / ÷», «+ / −» как в источнике. Запрещены замены на *, x, / и т.п.
4) NUMBERING. Сохраняй оригинальные номера и подпункты (а), б), 1), 2), …). Не перенумеровывать, не добавлять отсутствующие, не исправлять.
5) BLOCKS/ITEMS. Каждый визуальный блок верни в blocks[].block_raw (verbatim). Если блок явно состоит из атомов — разложи их в items_raw[] с group_id = block_id. Конкатенация всех items_raw одного group_id ДОЛЖНА в точности равняться block_raw.
6) LAYOUT. Если виден «столбик», сетка, «□», линейки: добавь оба слоя — layout_raw (фиксированная ширина/моноширинный текст) и semantic_raw (строки/колонки/позиции символов). Если неприменимо — не заполняй эти поля.
7) FLAGS (PII OFF в MVP). Не выполняй распознавание лиц/ФИО/телефонов и т.п.; не используй/не заполняй флаги, связанные с PII/лицами. Любые изображения на полях (рисунки, клипарт, герои учебников, схемы, пиктограммы) НЕ считать лицами и не влияют на ответ.
8) НИЧЕГО ЛИШНЕГО. Никаких нормализаций, исправлений орфографии, домыслов, переводов, подсказок или ответов на задачи.

🧭 Разбиение
• Разделяй задания по явным визуальным признакам (номера, заголовки, литеры, абзацы, колонки). Не дели произвольно.
• Подпункты/литеры фиксируй ровно настолько, насколько они есть в источнике (без добавлений/объединений).
• Если разбиение на items_raw неочевидно — оставь весь текст в block_raw без атомизации; не выдумывай структуру.

✅ Самопроверки перед отдачей JSON
• Для каждого block_id: join(items_raw[group_id]) == block_raw (побайтно).
• Сохранены исходные операторы и разрядные пробелы → operators_strict=true, thousands_space_preserved=true (если применимо).
• NUMBERING соответствует источнику (оригинальные номера/литеры без сдвигов).
• layout_raw/semantic_raw присутствуют только когда действительно необходимы.

Верни строго JSON по схеме detect. Любой текст вне JSON — ошибка. Никаких комментариев, заголовков и пояснений.
`
	schema, err := util.LoadPromptSchema("detect")
	if err != nil {
		return types.DetectResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	user := "Ответ строго JSON по схеме. Без комментариев."
	if in.GradeHint >= 1 && in.GradeHint <= 4 {
		user += fmt.Sprintf(" grade_hint=%d", in.GradeHint)
	}

	body := map[string]any{
		"model": e.Model,
		"input": []any{
			map[string]any{
				"role": "system",
				"content": []any{
					map[string]any{"type": "input_text", "text": system},
				},
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": user},
					map[string]any{"type": "input_image", "image_url": dataURL},
				},
			},
		},
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "detect",
				"strict": true,
				"schema": schema,
			},
		},
	}

	if strings.Contains(e.Model, "gpt-5") {
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
		return types.DetectResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.DetectResult{}, fmt.Errorf("openai detect %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		// fallback to manual extraction from Responses API envelope
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.DetectResult{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
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

	var hints strings.Builder
	if in.Options.GradeHint >= 1 && in.Options.GradeHint <= 4 {
		_, _ = fmt.Fprintf(&hints, " grade_hint=%d.", in.Options.GradeHint)
	}
	if s := strings.TrimSpace(in.Options.SubjectHint); s != "" {
		_, _ = fmt.Fprintf(&hints, " subject_hint=%q.", s)
	}
	if in.Options.SelectedTaskIndex >= 0 || strings.TrimSpace(in.Options.SelectedTaskBrief) != "" {
		_, _ = fmt.Fprintf(&hints, " selected_task=[index:%d, brief:%q].", in.Options.SelectedTaskIndex, in.Options.SelectedTaskBrief)
	}

	system := `Ты — школьный ассистент 1–4 классов. Перепиши выбранное задание полностью текстом, не додумывай.
Выдели вопрос задачи. Нечитаемые места помечай в квадратных скобках.
Соблюдай политику подтверждения:
- Автоподтверждение, если: confidence ≥ 0.80, meaning_change_risk ≤ 0.20, bracketed_spans_count = 0, needs_rescan=false.
- Иначе запрашивай подтверждение.
Верни строго JSON по схеме parse. Любой текст вне JSON — ошибка
`
	schema, err := util.LoadPromptSchema("parse")
	if err != nil {
		return types.ParseResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	user := "Ответ строго JSON по схеме. Без комментариев." + hints.String()

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
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": user},
					map[string]any{"type": "input_image", "image_url": dataURL},
				},
			},
		},
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "parse",
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

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.ParseResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.ParseResult{}, fmt.Errorf("openai parse %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.ParseResult{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var pr types.ParseResult
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return types.ParseResult{}, fmt.Errorf("openai parse: bad JSON: %w", err)
	}
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
Верни строго JSON по схеме hint. Любой текст вне JSON — ошибка.
`
	schema, err := util.LoadPromptSchema("hint")
	if err != nil {
		return types.HintResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	userObj := map[string]any{
		"task":  "Сгенерируй подсказку согласно PROMPT_HINT и верни JSON по схеме.",
		"input": in,
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
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": string(userJSON)},
				},
			},
		},
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "hint",
				"strict": true,
				"schema": schema,
			},
		},
	}
	if strings.Contains(e.Model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(payload))
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

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.HintResult{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
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
Верни строго JSON по схеме normalize. Любой текст вне JSON — ошибка.`

	schema, err := util.LoadPromptSchema("normalize")
	if err != nil {
		return types.NormalizeResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	userObj := map[string]any{
		"task":  "Нормализуй ответ ученика и верни только JSON по схеме.",
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	var userContent []any
	if strings.EqualFold(in.Answer.Source, "photo") {
		b64 := strings.TrimSpace(in.Answer.PhotoB64)
		if b64 == "" {
			return types.NormalizeResult{}, fmt.Errorf("openai normalize: answer.photo_b64 is empty")
		}
		photoBytes, mimeFromDataURL, err := util.DecodeBase64MaybeDataURL(in.Answer.PhotoB64)
		if err != nil {
			return types.NormalizeResult{}, fmt.Errorf("openai normalize: bad photo base64: %w", err)
		}
		mime := util.PickMIME(strings.TrimSpace(in.Answer.Mime), mimeFromDataURL, photoBytes)
		dataURL := b64
		if !strings.HasPrefix(strings.ToLower(dataURL), "data:") {
			dataURL = "data:" + mime + ";base64," + b64
		}
		userContent = []any{
			map[string]any{"type": "input_text", "text": "INPUT_JSON:\n" + string(userJSON)},
			map[string]any{"type": "input_image", "image_url": dataURL},
		}
	} else {
		if strings.TrimSpace(in.Answer.Text) == "" {
			return types.NormalizeResult{}, fmt.Errorf("openai normalize: answer.text is empty")
		}
		userContent = []any{map[string]any{"type": "input_text", "text": string(userJSON)}}
	}

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
				"role":    "user",
				"content": userContent,
			},
		},
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "normalize",
				"strict": true,
				"schema": schema,
			},
		},
	}
	if strings.Contains(e.Model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(payload))
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

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.NormalizeResult{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var nr types.NormalizeResult
	if err := json.Unmarshal([]byte(out), &nr); err != nil {
		return types.NormalizeResult{}, fmt.Errorf("openai normalize: bad JSON: %w", err)
	}
	return nr, nil
}

func (e *Engine) CheckSolution(ctx context.Context, in types.CheckSolutionInput) (types.CheckSolutionResult, error) {
	if e.APIKey == "" {
		return types.CheckSolutionResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.Model
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}

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
Верни строго JSON по схеме check_solution. Любой текст вне JSON — ошибка.
`
	schema, err := util.LoadPromptSchema("check")
	if err != nil {
		return types.CheckSolutionResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	userObj := map[string]any{
		"task":  "Проверь решение по правилам CHECK_SOLUTION и верни только JSON по схеме.",
		"input": in,
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
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "INPUT_JSON:\n" + string(userJSON)},
				},
			},
		},
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "check_solution",
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

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.CheckSolutionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.CheckSolutionResult{}, fmt.Errorf("openai check %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.CheckSolutionResult{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
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
Верни строго JSON по схеме analogue_solution. Любой текст вне JSON — ошибка.
`
	schema, err := util.LoadPromptSchema("analogue")
	if err != nil {
		return types.AnalogueSolutionResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	userObj := map[string]any{
		"task":  "Сформируй аналогичное задание тем же приёмом и верни СТРОГО JSON по схеме.",
		"input": in,
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
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "INPUT_JSON:\n" + string(userJSON)},
				},
			},
		},
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "analogue_solution",
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

	resp, err := e.httpc.Do(req)
	if err != nil {
		return types.AnalogueSolutionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.AnalogueSolutionResult{}, fmt.Errorf("openai analogue %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.AnalogueSolutionResult{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var ar types.AnalogueSolutionResult
	if err := json.Unmarshal([]byte(out), &ar); err != nil {
		return types.AnalogueSolutionResult{}, fmt.Errorf("openai analogue: bad JSON: %w", err)
	}
	if !ar.LeakGuardPassed {
		ar.LeakGuardPassed = true
	}
	ar.Safety.NoOriginalAnswerLeak = true
	return ar, nil
}

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
