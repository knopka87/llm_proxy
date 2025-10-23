package gpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-proxy/api/internal/ocr/types"
	"llm-proxy/api/internal/util"
)

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
