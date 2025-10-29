package types

// NormalizeInput — schema NORMALIZE_Input.v1
// См. раздел NORMALIZE в сводном файле схем.
type NormalizeInput struct {
	StudentAnswerText string `json:"student_answer_text"`
	ExpectedShape     string `json:"expected_shape"`         // number|string|list|steps
	UnitsPolicy       string `json:"units_policy,omitempty"` // правила единиц/округления/формата (опционально)
}

// NormalizeResult — schema NORMALIZE_Output.v1
// См. раздел NORMALIZE в сводном файле схем.
type NormalizeResult struct {
	Shape              string      `json:"shape"` // number|string|list|steps
	Value              interface{} `json:"value"`
	Units              string      `json:"units,omitempty"`
	Warnings           []string    `json:"warnings,omitempty"` // элементы вида "spelling_issue:..." (до 10)
	NeedsClarification *bool       `json:"needs_clarification,omitempty"`
	UncertainReasons   []string    `json:"uncertain_reasons,omitempty"` // до 3 причин
}
