package types

import (
	"fmt"
	"strings"
)

// HintLevel — уровень подсказки
type HintLevel string

const (
	HintL1 HintLevel = "L1"
	HintL2 HintLevel = "L2"
	HintL3 HintLevel = "L3"
)

// HintMode — режим подсказки
type HintMode string

const (
	HintModeLearn  HintMode = "learn"
	HintModeRescue HintMode = "rescue"
)

// HintRequest — вход запроса (HINT.request.v1)
type HintRequest struct {
	Task          ParseTask   `json:"task"`
	Mode          string      `json:"mode"`
	Items         []ParseItem `json:"items"`
	AppliedPolicy HintPolicy  `json:"applied_policy"`
	Template      string      `json:"template,omitempty"`      // selected pedagogical template profile, resolved by child_bot backend
	ExtraContext  string      `json:"extra_context,omitempty"` // verified retrieval grounding supplied by child_bot
}

// TaskRef — reference to parsed task
type TaskRef struct {
	TaskId             string `json:"task_id"`
	ParseSchemaVersion string `json:"parse_schema_version"`
}

// HintTaskQuality — quality flags for hint task
type HintTaskQuality struct {
	Flags []string `json:"flags"`
}

// HintTask — task info in hint response
type HintTask struct {
	Subject Subject         `json:"subject"` // uses shared Subject type
	Grade   int             `json:"grade"`
	Mode    *HintMode       `json:"mode"` // "learn" | "rescue" | null
	Quality HintTaskQuality `json:"quality"`
}

// AppliedPolicy — applied hint policy
type AppliedPolicy struct {
	MaxHints       int `json:"max_hints"`
	DefaultVisible int `json:"default_visible"`
}

// PlanCoverage — plan coverage info
type PlanCoverage struct {
	PlanStepsTotal   int `json:"plan_steps_total"`
	PlanStepsCovered int `json:"plan_steps_covered"`
}

// Hint — single hint entry
type Hint struct {
	Level    HintLevel `json:"level"` // "L1" | "L2" | "L3"
	HintText string    `json:"hint_text"`
}

// HintItem — item with hints
type HintItem struct {
	ItemId        string        `json:"item_id"`
	TemplateId    string        `json:"template_id"`
	AppliedPolicy AppliedPolicy `json:"applied_policy"`
	PlanCoverage  PlanCoverage  `json:"plan_coverage"`
	Hints         []Hint        `json:"hints"`
}

// HintButton — UI button for hint
type HintButton struct {
	Level HintLevel `json:"level"` // "L1" | "L2" | "L3"
	Label string    `json:"label"`
}

// HintUI — UI configuration
type HintUI struct {
	Buttons []HintButton `json:"buttons"`
}

// HintResponse — HINT_OUTPUT
// Required: schema_version, task_ref, task, items, ui.
type HintResponse struct {
	SchemaVersion  string     `json:"schema_version"`
	PromptVersion  string     `json:"prompt_version,omitempty"`  // версия промпта (например "3_class")
	AdvancedTopics []string   `json:"advanced_topics,omitempty"` // загруженные advanced-хинты (например ["arithmetic_fluency", "word_problems"])
	TaskRef        TaskRef    `json:"task_ref"`
	Task           HintTask   `json:"task"`
	Items          []HintItem `json:"items"`
	UI             HintUI     `json:"ui"`
}

// ValidateAgainstRequest enforces semantic invariants that JSON Schema cannot
// express. Incomplete hints must never be published as a successful response.
func (r HintResponse) ValidateAgainstRequest(in HintRequest) error {
	if in.Task.Subject != SubjectMath || len(in.Items) == 0 {
		if len(r.Items) != 0 || len(r.UI.Buttons) != 0 {
			return fmt.Errorf("subject gate: expected empty hints")
		}
		return nil
	}

	if len(r.Items) != len(in.Items) {
		return fmt.Errorf("items count: got %d, want %d", len(r.Items), len(in.Items))
	}

	requestItems := make(map[string]ParseItem, len(in.Items))
	for _, item := range in.Items {
		requestItems[item.ItemId] = item
	}
	for _, item := range r.Items {
		source, ok := requestItems[item.ItemId]
		if !ok {
			return fmt.Errorf("unknown item_id %q", item.ItemId)
		}
		total := len(source.SolutionInternal.Plan)
		if item.PlanCoverage.PlanStepsTotal != total {
			return fmt.Errorf("item %s: plan_steps_total=%d, want %d", item.ItemId, item.PlanCoverage.PlanStepsTotal, total)
		}
		if item.PlanCoverage.PlanStepsCovered != total {
			return fmt.Errorf("item %s: incomplete plan coverage %d/%d", item.ItemId, item.PlanCoverage.PlanStepsCovered, total)
		}

		expectedHints := source.HintPolicy.MaxHints
		if expectedHints < 2 {
			expectedHints = 2
		}
		if expectedHints > 3 {
			expectedHints = 3
		}
		if len(item.Hints) != expectedHints {
			return fmt.Errorf("item %s: hints count=%d, want %d", item.ItemId, len(item.Hints), expectedHints)
		}
		seen := make(map[HintLevel]bool, len(item.Hints))
		for _, hint := range item.Hints {
			if strings.TrimSpace(hint.HintText) == "" {
				return fmt.Errorf("item %s: empty %s hint", item.ItemId, hint.Level)
			}
			if seen[hint.Level] {
				return fmt.Errorf("item %s: duplicate %s hint", item.ItemId, hint.Level)
			}
			seen[hint.Level] = true
		}
		if !seen[HintL1] || !seen[HintL2] || expectedHints == 3 && !seen[HintL3] {
			return fmt.Errorf("item %s: required hint levels are missing", item.ItemId)
		}
	}
	return nil
}
