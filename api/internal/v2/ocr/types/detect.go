package types

// DetectInput — DETECT_Input.v1 (see pipeline schema)
// Required: image_ref. Optional: locale (default "ru-RU"), max_tasks (1..3, default 1).
type DetectInput struct {
	ImageRef string `json:"image_ref"`           // ID/URL of the image binary
	Locale   string `json:"locale,omitempty"`    // e.g., "ru-RU"
	MaxTasks int    `json:"max_tasks,omitempty"` // 1..3
}

// DetectResult — DETECT_RouteSchema.v1.1c
// Required: branch, confidence, version.
// Optional: signals (<=4 items, each <=20 chars), debug.reason (<=120 chars), threshold_note ("T_route=0.7").
type DetectResult struct {
	Branch        string       `json:"branch"`            // "math" | "ru" | "generic"
	Confidence    float64      `json:"confidence"`        // 0..1
	Signals       []string     `json:"signals,omitempty"` // up to 4 hints
	Debug         *DetectDebug `json:"debug,omitempty"`
	ThresholdNote string       `json:"threshold_note,omitempty"` // "T_route=0.7"
	Version       string       `json:"version"`                  // must be "1.1c"
}

// DetectDebug holds optional debug info.
type DetectDebug struct {
	Reason string `json:"reason,omitempty"`
}
