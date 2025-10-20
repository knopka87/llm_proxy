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
}

type NormalizeAnswer struct {
	Source   string `json:"source"` // text | photo
	Text     string `json:"text,omitempty"`
	PhotoB64 string `json:"photo_b64,omitempty"` // предпочтительно base64
	Mime     string `json:"mime,omitempty"`      // image/jpeg, image/png
}

// NormalizeResult — строгий JSON-ответ нормализации (schema v5)
// См. normalize.schema.json
type NormalizeResult struct {
	Success       bool        `json:"success"`
	Shape         string      `json:"shape"`                    // number|string|steps|list
	ShapeDetected *string     `json:"shape_detected,omitempty"` // number|string|steps|list|unknown|null
	Value         interface{} `json:"value"`                    // число | строка | []string | []any | null

	Units *UnitsInfo `json:"units,omitempty"`

	UncertainReasons       []string `json:"uncertain_reasons,omitempty"`
	NeedsClarification     *bool    `json:"needs_clarification,omitempty"`
	NeedsUserActionMessage *string  `json:"needs_user_action_message,omitempty"` // ≤120 символов
}

// UnitsInfo — обнаруженные/нормализованные единицы измерения (schema v5).
// Detected/Canonical/System/IsCompound/Kept/Mismatch допускают null.
type UnitsInfo struct {
	Detected  *string `json:"detected"`
	Canonical *string `json:"canonical"`
	Kept      *bool   `json:"kept"`
}
