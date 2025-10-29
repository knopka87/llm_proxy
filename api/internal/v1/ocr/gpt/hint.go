package gpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v1/ocr/types"
)

const HINT = "hint"

func (e *Engine) Hint(ctx context.Context, in types.HintInput) (types.HintResult, error) {
	if e.APIKey == "" {
		return types.HintResult{}, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.GetModel()

	// TODO переделать на отдельный env
	model = "gpt-4.1-mini"

	// Try to load system prompt from /prompt/hint<L1|L2|L3>.txt; fallback to the default text if not found.
	defaultSystem := `Ты — помощник для 1–4 классов. Сформируй РОВНО ОДИН блок подсказки уровня ` + string(in.Level) + `. Не решай задачу и не подставляй числа/слова из условия. Верни строго JSON по схеме hint. Любой текст вне JSON — ошибка.`

	level := strings.ToUpper(string(in.Level))
	system, err := util.LoadSystemPrompt(HINT+level, e.Name(), e.Version())
	log.Printf("HINT" + level + ": " + system)
	if err != nil {
		system = defaultSystem
	}

	schema, err := util.LoadPromptSchema(HINT, e.Version())
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
				"name":   HINT,
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
