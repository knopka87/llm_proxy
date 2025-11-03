package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"llm-proxy/api/internal/v1/prompt"
)

func LoadSystemPrompt(name, provider, version string) (string, error) {
	system, err := loadPrompt(name, "system", provider, version)
	if err != nil {
		system, err = loadPrompt("universal", "system", provider, version)
	}
	return system, err
}

func LoadUserPrompt(name, provider, version string) (string, error) {
	return loadPrompt(name, "user", provider, version)
}

func loadPrompt(name, tp, provider, version string) (string, error) {
	if provider == "" {
		return "", fmt.Errorf("provider is empty")
	}
	// First try provider-aware layout used by UpdateSystemPromptHandler:
	//   <PROMPT_DIR or api/internal/<version>/ocr/<provider>/prompt/<name>.<type(tp)>.txt
	baseRoot := os.Getenv("PROMPT_DIR")
	if baseRoot == "" {
		baseRoot = filepath.Join("api", "internal")
	}

	p := filepath.Join(baseRoot, version, "ocr", strings.ToLower(provider), "prompt", fmt.Sprintf("%s.%s.txt", name, tp))
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		return strings.TrimSpace(string(b)), nil
	}

	return "", fmt.Errorf("prompt %q not found in %s (provider=%q) or legacy prompt dir", name, baseRoot, provider)
}

// Загружаем <name>.schema.json из PROMPT_SCHEMA_DIR, иначе берём из встроенных prompt.*.
func LoadPromptSchema(name, version string) (map[string]any, error) {
	p := filepath.Join("../", version, "/prompt", name+".schema.json")
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
