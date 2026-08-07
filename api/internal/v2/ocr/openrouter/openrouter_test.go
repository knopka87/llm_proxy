package openrouter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"llm-proxy/api/internal/v2/ocr/types"
)

func TestFixEmptyArrayFields(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "visual_facts empty object becomes array",
			input:    `{"visual_facts":{}}`,
			expected: `{"visual_facts":[]}`,
		},
		{
			name:     "visual_facts non-empty object becomes empty array",
			input:    `{"visual_facts":{"key":"val"}}`,
			expected: `{"visual_facts":[]}`,
		},
		{
			name:     "items empty object becomes array",
			input:    `{"items":{}}`,
			expected: `{"items":[]}`,
		},
		{
			name:     "items non-empty object becomes empty array",
			input:    `{"items":{"foo":"bar"}}`,
			expected: `{"items":[]}`,
		},
		{
			name:     "plan empty object becomes array",
			input:    `{"plan":{}}`,
			expected: `{"plan":[]}`,
		},
		{
			name:     "solution_steps empty object becomes array",
			input:    `{"solution_steps":{}}`,
			expected: `{"solution_steps":[]}`,
		},
		{
			name:     "multiple array fields fixed",
			input:    `{"visual_facts":{},"items":{},"flags":{}}`,
			expected: `{"flags":[],"items":[],"visual_facts":[]}`,
		},
		{
			name:     "normal JSON with existing arrays passes through",
			input:    `{"visual_facts":["a","b"],"items":[1,2]}`,
			expected: `{"items":[1,2],"visual_facts":["a","b"]}`,
		},
		{
			name:     "normal JSON without array fields passes through",
			input:    `{"name":"test","value":42}`,
			expected: `{"name":"test","value":42}`,
		},
		{
			name:     "nested object with array field is fixed",
			input:    `{"data":{"visual_facts":{},"nested":{"items":{}}}}`,
			expected: `{"data":{"nested":{"items":[]},"visual_facts":[]}}`,
		},
		{
			name:     "deeply nested array fields are fixed",
			input:    `{"level1":{"level2":{"visual_facts":{},"items":{}}}}`,
			expected: `{"level1":{"level2":{"items":[],"visual_facts":[]}}}`,
		},
		{
			name:     "array field inside array element is fixed",
			input:    `{"rows":[{"visual_facts":{}},{"items":{}}]}`,
			expected: `{"rows":[{"visual_facts":[]},{"items":[]}]}`,
		},
		{
			name:     "spaces around colon are handled",
			input:    `{"visual_facts": {}}`,
			expected: `{"visual_facts":[]}`,
		},
		{
			name:     "non-array field with empty object stays unchanged",
			input:    `{"metadata":{},"visual_facts":{}}`,
			expected: `{"metadata":{},"visual_facts":[]}`,
		},
		{
			name:     "plan with string array elements is unchanged",
			input:    `{"plan":["step1","step2"]}`,
			expected: `{"plan":["step1","step2"]}`,
		},
		{
			name:     "plan with object elements converted to strings",
			input:    `{"plan":[{"action":"solve"},{"action":"check"}]}`,
			expected: `{"plan":["{\"action\":\"solve\"}","{\"action\":\"check\"}"]}`,
		},
		{
			name:     "solution_steps with object elements converted to strings",
			input:    `{"solution_steps":[{"step":1},{"step":2}]}`,
			expected: `{"solution_steps":["{\"step\":1}","{\"step\":2}"]}`,
		},
		{
			name:     "empty string returns unchanged",
			input:    ``,
			expected: ``,
		},
		{
			name:     "invalid JSON returns unchanged",
			input:    `{invalid json`,
			expected: `{invalid json`,
		},
		{
			name:     "complex real-world response",
			input:    `{"task_id":"t1","visual_facts":{},"items":{},"plan":["a"],"solution_steps":[],"metadata":{"x":1}}`,
			expected: `{"items":[],"metadata":{"x":1},"plan":["a"],"solution_steps":[],"task_id":"t1","visual_facts":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixEmptyArrayFields(tt.input)
			if got != tt.expected {
				t.Errorf("fixEmptyArrayFields(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestComposeCheckBlocks_UsesCanonicalTaxonomyAndVisualGuard(t *testing.T) {
	root := t.TempDir()
	promptDir := filepath.Join(root, "v2", "prompt", "check")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"check.advanced_arithmetic.system.txt": "ADVANCED_ARITHMETIC",
		"check.verify_arithmetic.system.txt":   "VERIFY_ARITHMETIC",
		"check.visual.system.txt":              "VISUAL_EVIDENCE",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(promptDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PROMPT_DIR", root)

	task := types.TaskStructCheck{
		VisualFacts: []types.VisualFact{{Kind: "fraction_total_parts", Value: 6, Critical: true}},
		Items:       []types.ParseItem{{PedKeys: types.PedKeys{TaskType: "fractions"}}},
	}
	composed, blocks := composeCheckBlocks("BASE", task)
	for _, block := range []string{"check.advanced_arithmetic", "check.verify_arithmetic", "check.visual"} {
		if !slices.Contains(blocks, block) {
			t.Errorf("block %q not loaded; got %v", block, blocks)
		}
	}
	for _, marker := range []string{"BASE", "ADVANCED_ARITHMETIC", "VERIFY_ARITHMETIC", "VISUAL_EVIDENCE"} {
		if !strings.Contains(composed, marker) {
			t.Errorf("composed prompt does not contain %q", marker)
		}
	}
}

func TestFixEmptyArrayFields_SemanticEqual(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		arrayField string
	}{
		{
			name:       "visual_facts becomes array",
			input:      `{"visual_facts":{},"other":"keep"}`,
			arrayField: "visual_facts",
		},
		{
			name:       "items becomes array",
			input:      `{"items":{"key":"val"},"other":"keep"}`,
			arrayField: "items",
		},
		{
			name:       "flags becomes array",
			input:      `{"flags":{},"other":"keep"}`,
			arrayField: "flags",
		},
		{
			name:       "nested visual_facts becomes array",
			input:      `{"data":{"visual_facts":{}}}`,
			arrayField: "visual_facts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixEmptyArrayFields(tt.input)
			var obj map[string]any
			if err := json.Unmarshal([]byte(got), &obj); err != nil {
				t.Fatalf("fixEmptyArrayFields returned invalid JSON: %v\ngot: %s", err, got)
			}

			// check top-level or nested
			v := obj[tt.arrayField]
			if v == nil {
				// check nested under "data"
				if data, ok := obj["data"].(map[string]any); ok {
					v = data[tt.arrayField]
				}
			}
			if v == nil {
				t.Fatalf("field %q not found in output", tt.arrayField)
			}
			if _, isArr := v.([]any); !isArr {
				t.Errorf("field %q should be array, got %T (%v)", tt.arrayField, v, v)
			}
		})
	}
}
