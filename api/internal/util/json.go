package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"llm-proxy/api/internal/prompt"
)

// Загружаем <name>.schema.json из PROMPT_SCHEMA_DIR, иначе берём из встроенных prompt.*.
func LoadPromptSchema(name string) (map[string]any, error) {
	p := filepath.Join("../prompt", name+".schema.json")
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("bad %s schema (file): %w", name, err)
		}
		ensureSchemaMeta(m)
		return m, nil
	}

	var raw []byte
	switch name {
	case "detect":
		raw = []byte(prompt.DetectSchema)
	case "parse":
		raw = []byte(prompt.ParseSchema)
	case "hint":
		raw = []byte(prompt.HintSchema)
	case "normalize":
		raw = []byte(prompt.NormalizeSchema)
	case "check":
		raw = []byte(prompt.CheckSolutionSchema)
	case "analogue":
		raw = []byte(prompt.AnalogueSolutionSchema)
	default:
		return nil, fmt.Errorf("unknown schema name: %s", name)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("bad %s schema (embedded): %w", name, err)
	}
	ensureSchemaMeta(m)
	return m, nil
}

// Мини-метаданные схемы (некоторые клиенты ожидают $schema).
func ensureSchemaMeta(m map[string]any) {
	if _, ok := m["$schema"]; !ok {
		m["$schema"] = "http://json-schema.org/draft-07/schema#"
	}
}

// Приводим схему к «строгому» виду для OpenAI: если есть properties — добавляем type=object и required со всеми полями.
func FixJSONSchemaStrict(node any) {
	switch n := node.(type) {
	case map[string]any:
		if props, ok := n["properties"].(map[string]any); ok {
			if _, hasType := n["type"]; !hasType {
				n["type"] = "object"
			}
			req := make([]any, 0, len(props))
			for k := range props {
				req = append(req, k)
			}
			n["required"] = req
			for _, v := range props {
				FixJSONSchemaStrict(v)
			}
		}
		if items, ok := n["items"]; ok {
			switch it := items.(type) {
			case map[string]any:
				FixJSONSchemaStrict(it)
			case []any:
				for _, el := range it {
					FixJSONSchemaStrict(el)
				}
			}
		}
		for _, k := range []string{"oneOf", "anyOf", "allOf"} {
			if v, ok := n[k]; ok {
				if arr, ok := v.([]any); ok {
					for _, el := range arr {
						FixJSONSchemaStrict(el)
					}
				}
			}
		}
	case []any:
		for _, v := range n {
			FixJSONSchemaStrict(v)
		}
	}
}
