package types

type HintLevel string // Уровень подсказки

const (
	HintL1 HintLevel = "L1"
	HintL2 HintLevel = "L2"
	HintL3 HintLevel = "L3"
)

// HintInput Вход для генерации подсказки (User input из PROMPT_HINT v1.4)
type HintInput struct {
	Level                   HintLevel      `json:"level"` // "L1" | "L2" | "L3"
	RawText                 string         `json:"raw_text"`
	Subject                 string         `json:"subject"` // "math" | "russian" | ...
	TaskType                string         `json:"task_type"`
	Grade                   int            `json:"grade"`          // 1..4
	SolutionShape           string         `json:"solution_shape"` // "number" | "string" | "steps" | "list"
	SubjectConfidence       float64        `json:"subject_confidence"`
	TaskTypeConfidence      float64        `json:"task_type_confidence"`
	TerminologyLevel        string         `json:"terminology_level"` // "none" | "light" | "teacher"
	ComplexityBand          string         `json:"complexity_band,omitempty"`
	MethodTag               string         `json:"method_tag"` // keep existing tag
	MathFlags               []string       `json:"math_flags,omitempty"`
	RuleRefs                []string       `json:"rule_refs,omitempty"`
	AnalogyAlignment        string         `json:"analogy_alignment,omitempty"` // matched|loose|none
	PreviousHints           []string       `json:"previous_hints,omitempty"`
	LengthLimits            map[string]int `json:"length_limits,omitempty"` // soft caps for title/steps/etc.
	RequiresContextFromText bool           `json:"requires_context_from_text"`
}

type HintResult struct {
	HintTitle       string   `json:"hint_title"`
	HintSteps       []string `json:"hint_steps"`
	ControlQuestion string   `json:"control_question"`
	NoFinalAnswer   bool     `json:"no_final_answer"` // Должно быть true
	AnalogyContext  string   `json:"analogy_context,omitempty"`
	TransferPrompt  string   `json:"transfer_prompt,omitempty"`
	Checklist       []string `json:"checklist,omitempty"`
	RuleHint        string   `json:"rule_hint,omitempty"`
	Meta            struct {
		Level            string   `json:"level"` // "L1"|"L2"|"L3"
		Subject          string   `json:"subject"`
		TaskType         string   `json:"task_type"`
		Grade            int      `json:"grade"`
		TerminologyLevel string   `json:"terminology_level"` // "none"|"light"|"teacher"
		ControlType      string   `json:"control_type"`      // "plan"|"checklist"|"self_explain"
		ComplexityBand   string   `json:"complexity_band"`   // "low"|"mid"|"high"
		MethodTag        string   `json:"method_tag,omitempty"`
		AnalogyAlignment string   `json:"analogy_alignment,omitempty"` // "matched"|"loose"|"none"
		MathFlags        []string `json:"math_flags,omitempty"`
		RuleRefs         []string `json:"rule_refs,omitempty"`
		LengthPolicy     struct {
			SoftCapsUsed   bool           `json:"soft_caps_used"`
			AnyOverflow    bool           `json:"any_overflow"`
			OverflowFields []string       `json:"overflow_fields,omitempty"`
			OverflowReason string         `json:"overflow_reason,omitempty"` // "none"|"clarity"|"domain_specific"|"grade_support"
			LengthUsed     map[string]int `json:"length_used,omitempty"`
		} `json:"length_policy"`
	} `json:"meta"`
}
