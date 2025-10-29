package types

type HintLevel string // Уровень подсказки

const (
	HintL1 HintLevel = "L1"
	HintL2 HintLevel = "L2"
	HintL3 HintLevel = "L3"
)

// HintInput соответствует HINT_Input.v1.1b
// required: level, grade, subject, raw_task_text; optional: shown_levels (<=2)
type HintInput struct {
	Level       HintLevel   `json:"level"`   // "L1" | "L2" | "L3"
	Grade       int         `json:"grade"`   // 1..4
	Subject     string      `json:"subject"` // "math" | "russian" | "generic"
	RawTaskText string      `json:"raw_task_text"`
	ShownLevels []HintLevel `json:"shown_levels,omitempty"` // max 2
}

// HintResult соответствует HintSchema.v1.1b
// required: level, hints; optional: debug
type HintResult struct {
	Level HintLevel `json:"level"` // "L1" | "L2" | "L3"
	Hints []string  `json:"hints"` // exactly 1 item by contract
	Debug *struct {
		Reason string `json:"reason"` // max 120 chars
	} `json:"debug,omitempty"`
}
