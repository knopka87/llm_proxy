package types

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
	Template      string      `json:"template"`
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
	SchemaVersion string     `json:"schema_version"`
	TaskRef       TaskRef    `json:"task_ref"`
	Task          HintTask   `json:"task"`
	Items         []HintItem `json:"items"`
	UI            HintUI     `json:"ui"`
}
