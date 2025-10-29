package gpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v1/ocr/types"
)

const CHECK = "check"

func (e *Engine) CheckSolution(ctx context.Context, in types.CheckSolutionInput) (types.CheckSolutionResult, error) {
	if e.APIKey == "" {
		return types.CheckSolutionResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.GetModel()
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}
	// TODO переделать на отдельный env
	model = "gpt-4o"

	system := `CHECK_SOLUTION (compatible with NORMALIZE_ANSWER)

Ты — модуль проверки решения для 1–4 классов. Работай только с нормализованным ответом ученика.
  
Контекст о входе от нормализации:
- student — объект из normalize: {success, shape∈{number|string|steps|list}, value, number_kind?, units{detected,canonical,kept?}, warnings[], normalized{notes[]?}, explanation_short?, next_action_hint?, needs_user_action_message?}.
- Не домысливай и не решай заново; опирайся на смысл value от normalize.
  
Маршрутизация ввода:
- Если student.success=false или student.next_action_hint ∈ {ask_rephoto, ask_retry} → verdict=uncertain; reason_codes добавь bad_input_low_quality / bad_input_format / multiple_candidates (по ситуации); перенеси needs_user_action_message в speakable_message (усечь до 140).
  
Правила сравнения:
- Верни один из verdict: correct | incorrect | uncertain. Строго JSON по check.schema.json. Любой текст вне JSON — ошибка.
- Единицы: policy required/forbidden/optional; используй student.units.detected/canonical и факт units.kept. Если normalize удалил единицы (warnings содержит unit_removed) и policy=optional — это НЕ ошибка. Разреши конверсии (mm↔cm↔m; g↔kg; min↔h); в comparison.units укажи expected/expected_primary/alternatives, detected, policy, convertible, applied (например "mm->cm"), factor.
- Числа: учитывай tolerance_abs/rel и equivalent_by_rule (например 0.5 ~ 1/2). Если формат (percent/degree/currency/time/range) требуется, но отсутствует, или сомнителен — verdict=uncertain.
- Unicode/пробелы: normalized.notes может содержать unicode_nbsp_normalized — не штрафуй.
- Исправления: если warnings содержит correction_detected — допускай часть исправлений, но отслеживай противоречия.
  
Списки и шаги:
- student.shape=list/steps → student.value может быть усечён до 6 элементов нормализацией; учитывай это как partial_ok при допустимой политике.
- Заполняй list_match/steps_match: matched/covered/total/extra/missing/extra_steps/order_ok/partial_ok. error_spot.index — 0-based.
  
Язык/строки:
- Для string используй accept_set/regex/synonym/case_fold/typo_lev1.
  
Триггеры uncertain:
- Низкая уверенность у student, неоднозначный формат, required units отсутствуют, несколько конкурирующих кандидатов (например normalize.error=too_many_candidates).
  
Безопасность:
- leak_guard_passed=true, safety.no_final_answer_leak=true; не раскрывай правильное число/слово.
- short_hint ≤120 символов, speakable_message ≤140.
  
Верни строго JSON по схеме check. Любой текст вне JSON — ошибка.
`
	schema, err := util.LoadPromptSchema(CHECK, e.Version())
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
				"name":   CHECK,
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
