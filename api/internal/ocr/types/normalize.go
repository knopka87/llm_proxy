package types

import "encoding/json"

type NormalizeInput struct {
	TaskID        string          `json:"task_id"`
	UserIDAnon    string          `json:"user_id_anon"`
	Grade         int             `json:"grade"`
	Subject       string          `json:"subject"`
	TaskType      string          `json:"task_type"`
	SolutionShape string          `json:"solution_shape"`
	Answer        NormalizeAnswer `json:"answer"`
	ParseContext  json.RawMessage `json:"parse_context"`
	Provider      string          `json:"provider,omitempty"`
	Model         string          `json:"model,omitempty"`
}

type NormalizeAnswer struct {
	Source   string `json:"source"` // text | photo
	Text     string `json:"text,omitempty"`
	PhotoB64 string `json:"photo_b64,omitempty"` // предпочтительно base64
	Mime     string `json:"mime,omitempty"`      // image/jpeg, image/png
	Lang     string `json:"lang,omitempty"`
}

// NormalizeResult — строгий JSON-ответ нормализации (schema v5)
// См. normalize.schema.json
type NormalizeResult struct {
	Success       bool        `json:"success"`
	Shape         string      `json:"shape"`                    // number|string|steps|list
	ShapeDetected *string     `json:"shape_detected,omitempty"` // number|string|steps|list|unknown|null
	Value         interface{} `json:"value"`                    // число | строка | []string | []any | null
	NumberKind    *string     `json:"number_kind,omitempty"`    // integer|decimal|fraction|mixed_fraction|time|range|unknown|null
	Confidence    *float64    `json:"confidence,omitempty"`     // 0..1|null

	AnswerSource        string   `json:"answer_source"`                   // text|photo
	SourceOCRConfidence *float64 `json:"source_ocr_confidence,omitempty"` // 0..1|null
	OCREngine           *string  `json:"ocr_engine,omitempty"`            // null|string

	Normalized *NormalizedInfo `json:"normalized,omitempty"`
	Units      *UnitsInfo      `json:"units,omitempty"`

	Warnings   []string    `json:"warnings,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`

	UncertainReasons       []string `json:"uncertain_reasons,omitempty"`
	NeedsClarification     *bool    `json:"needs_clarification,omitempty"`
	NeedsUserActionMessage *string  `json:"needs_user_action_message,omitempty"` // ≤120 символов
	NextActionHint         *string  `json:"next_action_hint,omitempty"`          // ask_rephoto|ask_retry|none|null
	AutoSelectPolicy       *string  `json:"auto_select_policy,omitempty"`        // last|first|none|null
	TranscribeOutputID     *string  `json:"transcribe_output_id,omitempty"`
	RawTranscription       *string  `json:"raw_transcription,omitempty"`
	Error                  *string  `json:"error"` // empty|null|enum
	PIIFlag                *bool    `json:"pii_flag,omitempty"`
}

// NormalizedInfo — сведения о «чистке» исходного ответа.
type NormalizedInfo struct {
	Raw   *string  `json:"raw,omitempty"`
	Clean *string  `json:"clean,omitempty"`
	Notes []string `json:"notes,omitempty"`
}

// UnitsInfo — обнаруженные/нормализованные единицы измерения (schema v5).
// Detected/Canonical/System/IsCompound/Kept/Mismatch допускают null.
type UnitsInfo struct {
	Detected   *string  `json:"detected"`
	Canonical  *string  `json:"canonical"`
	System     *string  `json:"system,omitempty"`
	IsCompound *bool    `json:"is_compound"`
	Parts      []string `json:"parts,omitempty"`
	Kept       *bool    `json:"kept"`
	Mismatch   *bool    `json:"mismatch"`
}

// Span — позиция сущности в исходном тексте (необязательно).
type Span struct {
	SpanFrom int    `json:"span_from"`
	SpanTo   int    `json:"span_to"`
	Label    string `json:"label,omitempty"`
}

// Candidate — альтернативное прочтение значения.
type Candidate struct {
	Value    interface{} `json:"value"`
	SpanFrom int         `json:"span_from"`
	SpanTo   int         `json:"span_to"`
	Kind     *string     `json:"kind,omitempty"`     // digit_number|word_number|time|range|operator|unit|strikethrough|overwritten|superscript|caret_insert|null
	Priority *int        `json:"priority,omitempty"` // 0..N|null
}
