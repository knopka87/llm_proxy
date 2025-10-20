package types

type HintLevel string // Уровень подсказки

const (
	HintL1 HintLevel = "L1"
	HintL2 HintLevel = "L2"
	HintL3 HintLevel = "L3"
)

// HintInput Вход для генерации подсказки (User input из PROMPT_HINT v1.4)
type HintInput struct {
	Level            HintLevel `json:"level"` // "L1" | "L2" | "L3"
	RawText          string    `json:"raw_text"`
	Subject          string    `json:"subject"` // "math" | "russian" | ...
	TaskType         string    `json:"task_type"`
	Grade            int       `json:"grade"`             // 1..4
	SolutionShape    string    `json:"solution_shape"`    // "number" | "string" | "steps" | "list"
	TerminologyLevel string    `json:"terminology_level"` // "none" | "light" | "teacher"
}

type HintResult struct {
	HintTitle       string   `json:"hint_title"`
	HintSteps       []string `json:"hint_steps"`
	ControlQuestion string   `json:"control_question"`
	RuleHint        string   `json:"rule_hint,omitempty"`
}
