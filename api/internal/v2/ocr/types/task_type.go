package types

import "strings"

// Canonical task types shared by PARSE, template routing, HINT and CHECK.
const (
	TaskTypeArithmeticFluency   = "arithmetic_fluency"
	TaskTypeCreativeComposition = "creative_composition"
	TaskTypeDataRepresentation  = "data_representation"
	TaskTypeFractionsPercent    = "fractions_percent"
	TaskTypeGeometry            = "geometry"
	TaskTypeLogic               = "logic"
	TaskTypeMeasurementUnits    = "measurement_units"
	TaskTypeNumberSense         = "number_sense"
	TaskTypeNumeralSystems      = "numeral_systems"
	TaskTypePatternsLogic       = "patterns_logic"
	TaskTypeSetsLogic           = "sets_logic"
	TaskTypeWordProblems        = "word_problems"
)

// NormalizeTaskType converts legacy prompt values to the routing taxonomy used
// by the template registry. Unknown values are preserved for forward compatibility.
func NormalizeTaskType(taskType string) string {
	switch strings.TrimSpace(strings.ToLower(taskType)) {
	case "arithmetic", "expressions", "equations":
		return TaskTypeArithmeticFluency
	case "fractions":
		return TaskTypeFractionsPercent
	case "tables":
		return TaskTypeDataRepresentation
	case "comparison":
		return TaskTypeNumberSense
	case "patterns":
		return TaskTypePatternsLogic
	case "other_math":
		return TaskTypeLogic
	default:
		return strings.TrimSpace(strings.ToLower(taskType))
	}
}

// HintAdvancedPromptBlock returns the grade-specific pedagogical block shared
// by a family of canonical task types.
func HintAdvancedPromptBlock(taskType string) string {
	switch NormalizeTaskType(taskType) {
	case TaskTypeArithmeticFluency, TaskTypeFractionsPercent:
		return "arithmetic_fluency"
	case TaskTypeMeasurementUnits:
		return "measurement_units"
	case TaskTypeNumberSense, TaskTypeNumeralSystems, TaskTypeDataRepresentation:
		return "number_sense"
	case TaskTypeWordProblems:
		return "word_problems"
	case TaskTypeGeometry:
		return "geometry"
	case TaskTypePatternsLogic, TaskTypeLogic, TaskTypeSetsLogic:
		return "patterns_logic"
	default:
		return ""
	}
}

// CheckAdvancedPromptBlock returns the checker block shared by a family of
// canonical task types.
func CheckAdvancedPromptBlock(taskType string) string {
	switch NormalizeTaskType(taskType) {
	case TaskTypeArithmeticFluency, TaskTypeFractionsPercent, TaskTypeMeasurementUnits,
		TaskTypeNumberSense, TaskTypeNumeralSystems:
		return "arithmetic"
	case TaskTypeWordProblems:
		return "word_problems"
	case TaskTypeGeometry, TaskTypeDataRepresentation:
		return "geometry"
	case TaskTypePatternsLogic, TaskTypeLogic, TaskTypeSetsLogic:
		return "patterns"
	default:
		return ""
	}
}

// VerificationPromptBlock returns the independent-verification algorithm that
// must be included for the task family.
func VerificationPromptBlock(taskType string) string {
	switch NormalizeTaskType(taskType) {
	case TaskTypeArithmeticFluency, TaskTypeFractionsPercent, TaskTypeMeasurementUnits,
		TaskTypeNumberSense, TaskTypeNumeralSystems, TaskTypeWordProblems:
		return "arithmetic"
	case TaskTypeDataRepresentation:
		return "tables"
	case TaskTypePatternsLogic, TaskTypeLogic, TaskTypeSetsLogic:
		return "transforms"
	default:
		return ""
	}
}
