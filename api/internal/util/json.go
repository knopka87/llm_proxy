package util

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	prompt1 "llm-proxy/api/internal/v1/prompt"
	prompt2 "llm-proxy/api/internal/v2/prompt"
)

func LoadSystemPrompt(name, provider, version string, subdirs ...string) (string, error) {
	// Try to load from subdirectories first
	if len(subdirs) > 0 {
		if p, err := cachedLoadPromptSubdirs(name, "system", provider, version, subdirs...); err == nil {
			return p, nil
		}
		for _, subdir := range subdirs {
			if p, err := cachedLoadPromptSubdirs(name, "system", provider, version, subdir); err == nil {
				return p, nil
			}
		}
	}

	// Fallback to universal prompt
	system, err := cachedLoadPromptSubdirs("universal", "system", provider, version, "universal")
	if err != nil {
		system, err = cachedLoadPrompt("universal", "system", provider, version)
	}
	return system, err
}

func LoadUserPrompt(name, provider, version string, subdirs ...string) (string, error) {
	// Try to load from subdirectories first
	if len(subdirs) > 0 {
		if p, err := cachedLoadPromptSubdirs(name, "user", provider, version, subdirs...); err == nil {
			return p, nil
		}
		for _, subdir := range subdirs {
			if p, err := cachedLoadPromptSubdirs(name, "user", provider, version, subdir); err == nil {
				return p, nil
			}
		}
	}
	return cachedLoadPrompt(name, "user", provider, version)
}

func loadPrompt(name, tp, provider, version string, subdirs ...string) (string, error) {
	if provider == "" {
		return "", fmt.Errorf("provider is empty")
	}
	baseRoot := os.Getenv("PROMPT_DIR")
	if baseRoot == "" {
		baseRoot = filepath.Join("api", "internal")
	}

	// Промпты общие для всех провайдеров — лежат в v2/prompt/
	pathParts := []string{baseRoot, version, "prompt"}
	if len(subdirs) > 0 {
		pathParts = append(pathParts, subdirs...)
	}
	pathParts = append(pathParts, fmt.Sprintf("%s.%s.txt", name, tp))
	p := filepath.Join(pathParts...)

	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		return strings.TrimSpace(string(b)), nil
	}

	// Fallback: старая структура v2/ocr/{provider}/prompt/
	if len(subdirs) > 0 {
		p = filepath.Join(baseRoot, version, "ocr", strings.ToLower(provider), "prompt", subdirs[len(subdirs)-1], fmt.Sprintf("%s.%s.txt", name, tp))
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b)), nil
		}
	}

	// Fallback: базовый промпт без поддиректорий
	p = filepath.Join(baseRoot, version, "prompt", fmt.Sprintf("%s.%s.txt", name, tp))
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		return strings.TrimSpace(string(b)), nil
	}

	return "", fmt.Errorf("prompt %q not found (provider=%q, version=%q)", name, provider, version)
}

// Загружаем <name>.schema.json из PROMPT_SCHEMA_DIR, иначе берём из встроенных prompt.*.
func LoadPromptSchema(name, version string) (map[string]any, error) {
	baseRoot := os.Getenv("PROMPT_DIR")
	if baseRoot == "" {
		baseRoot = filepath.Join("api", "internal")
	}
	p := filepath.Join(baseRoot, version, "prompt", name+".schema.json")
	log.Printf("schema path: %s", p)
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("bad %s schema (file): %w", name, err)
		}
		// Unwrap OpenAI-style wrapper: {"name":..., "strict":..., "schema":{...}}
		if inner, ok := m["schema"].(map[string]any); ok {
			m = inner
		}
		ensureSchemaMeta(m)
		return m, nil
	}

	var raw []byte
	switch name {
	case "detect":
		if version == "v2" {
			raw = []byte(prompt2.DetectSchema)
		} else {
			raw = []byte(prompt1.DetectSchema)
		}
	case "parse":
		if version == "v2" {
			raw = []byte(prompt2.ParseSchema)
		} else {
			raw = []byte(prompt1.ParseSchema)
		}
	case "hint":
		if version == "v2" {
			raw = []byte(prompt2.HintSchema)
		} else {
			raw = []byte(prompt1.HintSchema)
		}
	case "check":
		if version == "v2" {
			raw = []byte(prompt2.CheckSolutionSchema)
		} else {
			raw = []byte(prompt1.CheckSolutionSchema)
		}
	case "analogue":
		if version == "v2" {
			raw = []byte(prompt2.AnalogueSolutionSchema)
		} else {
			raw = []byte(prompt1.AnalogueSolutionSchema)
		}
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

// Приводим схему к «строгому» виду для OpenAI/OpenRouter strict mode:
//   - nullable-поля (type: ["string", "null"]) конвертируются в anyOf: [{type:X}, {type:null}]
//   - required содержит ВСЕ ключи из properties (включая nullable)
//   - additionalProperties: false на каждом объекте с properties (OpenAI strict requirement)
func FixJSONSchemaStrict(node any) {
	switch n := node.(type) {
	case map[string]any:
		// Convert nullable type arrays to anyOf BEFORE processing required
		convertNullableToAnyOf(n)

		if props, ok := n["properties"].(map[string]any); ok {
			if _, hasType := n["type"]; !hasType {
				n["type"] = "object"
			}
			req := make([]any, 0, len(props))
			for k := range props {
				req = append(req, k)
			}
			n["required"] = req
			// OpenAI strict mode requires additionalProperties: false on every object
			if _, has := n["additionalProperties"]; !has {
				n["additionalProperties"] = false
			}
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

// convertNullableToAnyOf replaces "type": ["string", "null"] with
// "anyOf": [{"type":"string"}, {"type":"null"}] — required by OpenAI strict mode.
// For object/array types, copies sibling schema keywords (properties, required,
// additionalProperties, items, etc.) into the anyOf branch so OpenAI strict
// mode accepts the schema.
func convertNullableToAnyOf(n map[string]any) {
	typeVal, ok := n["type"]
	if !ok {
		return
	}
	typeArr, ok := typeVal.([]any)
	if len(typeArr) < 2 {
		return
	}

	var baseTypes []any
	hasNull := false
	for _, t := range typeArr {
		if s, ok := t.(string); ok && s == "null" {
			hasNull = true
		} else {
			baseTypes = append(baseTypes, t)
		}
	}
	if !hasNull {
		return
	}

	// Remove nullable from parent — build anyOf branches
	delete(n, "type")

	// Keywords to move into the non-null anyOf branch (OpenAI strict requires them)
	objectKeywords := []string{"properties", "required", "additionalProperties"}
	arrayKeywords := []string{"items", "minItems", "maxItems"}

	anyOf := make([]any, 0, len(baseTypes)+1)
	for _, bt := range baseTypes {
		branch := map[string]any{"type": bt}
		if s, ok := bt.(string); ok {
			switch s {
			case "object":
				for _, kw := range objectKeywords {
					if v, ok := n[kw]; ok {
						branch[kw] = v
						delete(n, kw)
					}
				}
				// OpenAI strict mode requires additionalProperties: false on every object
				if _, has := branch["additionalProperties"]; !has {
					branch["additionalProperties"] = false
				}
			case "array":
				for _, kw := range arrayKeywords {
					if v, ok := n[kw]; ok {
						branch[kw] = v
						delete(n, kw)
					}
				}
			}
		}
		anyOf = append(anyOf, branch)
	}
	anyOf = append(anyOf, map[string]any{"type": "null"})
	n["anyOf"] = anyOf
}
