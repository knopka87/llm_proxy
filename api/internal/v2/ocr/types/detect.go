package types

// DetectRequest — DETECT.request.v1
// Required: image. Optional: locale ("ru-RU" | "en-US"), max_tasks (const=1; default 1).
type DetectRequest struct {
	Image    string `json:"image"`               // Image handle (URL or base64 id)
	Locale   string `json:"locale,omitempty"`    // "ru-RU" | "en-US"
	MaxTasks int    `json:"max_tasks,omitempty"` // schema: minimum=1, maximum=1 (treated as const=1)
}

// DetectResponse — DETECT.response.v1
// Required: subject_hint, confidence.
// Optional: grade_hint (1..4), debug_reason (≤120 chars).
type DetectResponse struct {
	SubjectHint Subject `json:"subject_hint"`           // "math" | "russian" | "generic"
	GradeHint   *int64  `json:"grade_hint,omitempty"`   // 1..4
	Confidence  float64 `json:"confidence"`             // 0..1
	DebugReason string  `json:"debug_reason,omitempty"` // ≤120 chars
}
