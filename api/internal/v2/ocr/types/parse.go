package types

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
