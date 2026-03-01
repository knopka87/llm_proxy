package types

import (
	"fmt"
	"regexp"
	"strings"
)

// ParseRequest — вход запроса (PARSE.request.v1)
type ParseRequest struct {
	Image             string `json:"image"`
	TaskId            string `json:"task_id"`
	Grade             int64  `json:"grade"`
	SubjectCandidate  string `json:"subject_candidate"`
	SubjectConfidence string `json:"subject_confidence"`
	Locale            string `json:"locale"`
}

// H3Reason — enum for hint policy h3_reason
type H3Reason string

const (
	H3ReasonNone              H3Reason = "none"
	H3ReasonSplitLongH2       H3Reason = "split_long_h2"
	H3ReasonLogicalChain      H3Reason = "logical_chain"
	H3ReasonApplyToManyPlaces H3Reason = "apply_to_many_places"
)

// VisualFact — факт из изображения
type VisualFact struct {
	Kind     string      `json:"kind"`
	Value    interface{} `json:"value"` // string | number | null | array
	Critical bool        `json:"critical"`
}

// ParseTaskQuality — quality flags for task
type ParseTaskQuality struct {
	Flags []string `json:"flags"`
}

// ParseTask — parsed task info
type ParseTask struct {
	TaskId        string           `json:"task_id"`
	Subject       Subject          `json:"subject"` // uses shared Subject type from detect.go
	Grade         int              `json:"grade"`
	TaskTextClean string           `json:"task_text_clean"`
	VisualFacts   []VisualFact     `json:"visual_facts"`
	Quality       ParseTaskQuality `json:"quality"`
}

// PedKeys — pedagogical routing keys
type PedKeys struct {
	TemplateId     string                 `json:"template_id"`
	TaskType       string                 `json:"task_type"`
	Format         string                 `json:"format"`
	UnitKind       *string                `json:"unit_kind"` // nullable
	Constraints    []string               `json:"constraints"`
	TemplateParams map[string]interface{} `json:"template_params"`
}

// HintPolicy — hint generation policy
type HintPolicy struct {
	MaxHints       int      `json:"max_hints"`
	DefaultVisible int      `json:"default_visible"`
	H3Reason       H3Reason `json:"h3_reason"`
}

// ItemQuality — item quality flags
type ItemQuality struct {
	UnsafeToFinalizeAnswer bool `json:"unsafe_to_finalize_answer"`
}

// SolutionInternal — internal solution representation
type SolutionInternal struct {
	Plan          []string    `json:"plan"`
	SolutionSteps []string    `json:"solution_steps"`
	FinalAnswer   interface{} `json:"final_answer"` // string | number | null
}

// ValidateFinalAnswer checks consistency between solution_steps and final_answer.
// Returns (isConsistent, derivedAnswer).
// P0.1: Prevents false "correct" verdicts when PARSE answer contradicts solution steps.
func (si *SolutionInternal) ValidateFinalAnswer() (consistent bool, derivedAnswer string) {
	if si.FinalAnswer == nil {
		// No final answer to validate
		return true, ""
	}

	// If no solution steps, we can't validate
	if len(si.SolutionSteps) == 0 {
		return true, ""
	}

	// Extract numeric/text answer from last solution step (common pattern)
	lastStep := si.SolutionSteps[len(si.SolutionSteps)-1]
	derivedAnswer = extractAnswerFromStep(lastStep)

	// Compare with final_answer
	finalAnswerStr := formatAnswerForComparison(si.FinalAnswer)

	// If we couldn't extract answer from steps, assume consistent
	if derivedAnswer == "" {
		return true, ""
	}

	// Normalize and compare
	consistent = normalizeAnswer(derivedAnswer) == normalizeAnswer(finalAnswerStr)
	return consistent, derivedAnswer
}

// ParseItem — parsed item (sub-task)
type ParseItem struct {
	ItemId           string           `json:"item_id"`
	ItemTextClean    string           `json:"item_text_clean"`
	PedKeys          PedKeys          `json:"ped_keys"`
	HintPolicy       HintPolicy       `json:"hint_policy"`
	ItemQuality      ItemQuality      `json:"item_quality"`
	SolutionInternal SolutionInternal `json:"solution_internal"`
}

// ParseResponse — PARSE_OUTPUT
// Required: schema_version, task, items.
type ParseResponse struct {
	SchemaVersion string      `json:"schema_version"`
	Task          ParseTask   `json:"task"`
	Items         []ParseItem `json:"items"`
}

// ValidateItems checks all items for final_answer ↔ solution_steps consistency.
// Sets UnsafeToFinalizeAnswer=true if inconsistency detected.
// P0.1: Called after JSON unmarshal to catch PARSE errors before CHECK.
func (pr *ParseResponse) ValidateItems() {
	for i := range pr.Items {
		item := &pr.Items[i]
		consistent, _ := item.SolutionInternal.ValidateFinalAnswer()
		if !consistent {
			item.ItemQuality.UnsafeToFinalizeAnswer = true
		}
	}
}

// --- Helper functions for answer validation ---

// extractAnswerFromStep extracts final numeric/text answer from a solution step.
// Looks for patterns like "= 42", "Ответ: 42", "итого 42" etc.
func extractAnswerFromStep(step string) string {
	// Pattern: "= <number>" at the end
	reEquals := regexp.MustCompile(`=\s*([\d.,]+)\s*$`)
	if m := reEquals.FindStringSubmatch(step); len(m) > 1 {
		return m[1]
	}

	// Pattern: "Ответ: <value>" or "ответ: <value>"
	reAnswer := regexp.MustCompile(`(?i)ответ[:\s]+(.+?)(?:\.|$)`)
	if m := reAnswer.FindStringSubmatch(step); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}

	// Pattern: "итого <number>"
	reTotal := regexp.MustCompile(`(?i)итого\s+([\d.,]+)`)
	if m := reTotal.FindStringSubmatch(step); len(m) > 1 {
		return m[1]
	}

	return ""
}

// formatAnswerForComparison converts interface{} answer to string for comparison.
func formatAnswerForComparison(answer interface{}) string {
	if answer == nil {
		return ""
	}
	switch v := answer.(type) {
	case string:
		return v
	case float64:
		// Format without trailing zeros
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case int, int64:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// normalizeAnswer normalizes an answer string for comparison.
// Handles: spaces, comma/dot decimal separators, trailing zeros.
func normalizeAnswer(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	// Replace comma with dot for decimal comparison
	s = strings.ReplaceAll(s, ",", ".")
	// Remove trailing zeros after decimal point
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	// Remove spaces
	s = strings.ReplaceAll(s, " ", "")
	return s
}
