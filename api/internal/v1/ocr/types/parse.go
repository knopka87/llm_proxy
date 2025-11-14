package types

// Subject represents a subject hint for PARSE.
// Allowed values: "math", "russian", "generic".

const (
	SubjectMath    Subject = "math"
	SubjectRussian Subject = "russian"
	SubjectGeneric Subject = "generic"
)

type Subject string

func (s Subject) String() string {
	switch s {
	case SubjectMath:
		return string(SubjectMath)
	case SubjectRussian:
		return string(SubjectRussian)
	default:
		return string(SubjectGeneric)
	}
}

// ParseRequest — вход запроса (PARSE.request.v1)
// required: image; optional: locale, subject_hint, grade_hint
type ParseRequest struct {
	Image       string   `json:"image"`
	Locale      string   `json:"locale,omitempty"`       // "ru-RU" | "en-US"
	SubjectHint *Subject `json:"subject_hint,omitempty"` // "math" | "russian" | "generic"
	GradeHint   *int64   `json:"grade_hint,omitempty"`   // 1..4
}

// ParseResponse — выход (PARSE.response.v1)
// required: raw_task_text, task_struct; optional: needs_user_confirmation (default true)
type ParseResponse struct {
	RawTaskText           string     `json:"raw_task_text"`
	TaskStruct            TaskStruct `json:"task_struct"`
	NeedsUserConfirmation bool       `json:"needs_user_confirmation,omitempty"` // default=true
}
