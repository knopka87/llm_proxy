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
