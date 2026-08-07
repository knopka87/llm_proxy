package types

import "testing"

func TestNormalizeTaskType(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"expressions": TaskTypeArithmeticFluency,
		"fractions":   TaskTypeFractionsPercent,
		"tables":      TaskTypeDataRepresentation,
		"comparison":  TaskTypeNumberSense,
		"sets_logic":  TaskTypeSetsLogic,
	}
	for input, want := range tests {
		if got := NormalizeTaskType(input); got != want {
			t.Errorf("NormalizeTaskType(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestVerificationPromptBlock(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		TaskTypeArithmeticFluency:  "arithmetic",
		TaskTypeWordProblems:       "arithmetic",
		TaskTypeFractionsPercent:   "arithmetic",
		TaskTypeDataRepresentation: "tables",
		TaskTypeSetsLogic:          "transforms",
	}
	for taskType, want := range tests {
		if got := VerificationPromptBlock(taskType); got != want {
			t.Errorf("VerificationPromptBlock(%q)=%q, want %q", taskType, got, want)
		}
	}
}

func TestHintAdvancedPromptBlock(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		TaskTypeFractionsPercent:   "arithmetic_fluency",
		TaskTypeMeasurementUnits:   "measurement_units",
		TaskTypeDataRepresentation: "number_sense",
		TaskTypeSetsLogic:          "patterns_logic",
	}
	for taskType, want := range tests {
		if got := HintAdvancedPromptBlock(taskType); got != want {
			t.Errorf("HintAdvancedPromptBlock(%q)=%q, want %q", taskType, got, want)
		}
	}
}
