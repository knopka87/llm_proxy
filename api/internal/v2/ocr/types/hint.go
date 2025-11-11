package types

type HintLevel string // Уровень подсказки

const (
	HintL1 HintLevel = "L1"
	HintL2 HintLevel = "L2"
	HintL3 HintLevel = "L3"
)

// HintRequest — вход запроса (HINT.request.v1)
// required: task_struct, level; optional: previous_hints (<=2), locale
type HintRequest struct {
	TaskStruct    TaskStruct `json:"task_struct"`
	RawTaskText   string     `json:"raw_task_text"`
	Level         HintLevel  `json:"level"`                    // "L1" | "L2" | "L3"
	Grade         *int64     `json:"grade"`                    // 1..4
	PreviousHints []string   `json:"previous_hints,omitempty"` // max 2 (валидируется схемой)
	Locale        string     `json:"locale,omitempty"`         // "ru-RU" | "en-US"
}

// HintResponse — выход (HINT.response.v1)
// required: level, hint_text
type HintResponse struct {
	Level    HintLevel `json:"level"`     // "L1" | "L2" | "L3"
	HintText string    `json:"hint_text"` // непустая строка
}
