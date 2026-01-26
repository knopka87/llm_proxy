package types

// --- CHECK (v1) ----------------------------------------------------
// Соответствует схемам CHECK.request.v1 и CHECK.response.v1.

// CheckRequest — вход запроса (CHECK.request.v1)
type CheckRequest struct {
	TaskStruct  ParseTask `json:"task_struct"`
	RawTaskText string    `json:"raw_task_text"`
	Student     struct {
		Grade   int64   `json:"grade"`
		Subject Subject `json:"subject"`
		Locale  string  `json:"locale"`
	} `json:"student"`
	PhotoQualityHint string `json:"photo_quality_hint"`
}

// CheckStatus — статус обработки
type CheckStatus string

const (
	CheckStatusEvaluated       CheckStatus = "evaluated"
	CheckStatusNeedBetterPhoto CheckStatus = "need_better_photo"
	CheckStatusNoAnswer        CheckStatus = "no_answer"
	CheckStatusInternalError   CheckStatus = "internal_error"
)

// PhotoQualityLabel — качественная оценка фото
type PhotoQualityLabel string

const (
	PhotoQualityLow    PhotoQualityLabel = "low"
	PhotoQualityMedium PhotoQualityLabel = "medium"
	PhotoQualityHigh   PhotoQualityLabel = "high"
)

// ErrorSpan — диапазон ошибки в исходном ответе
type ErrorSpan struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Label string `json:"label"`
}

// PhotoQuality — оценка качества фотографии ответа
type PhotoQuality struct {
	Score float64           `json:"score"` // 0-1
	Label PhotoQualityLabel `json:"label"` // "low" | "medium" | "high"
}

// CheckDebug — диагностическая информация (не показывать пользователю)
type CheckDebug struct {
	RawAnswerText    *string `json:"raw_answer_text"`
	NormalizedAnswer *string `json:"normalized_answer"`
}

// CheckResponse — CHECK.response.v1
// Required: status, can_evaluate, is_correct, feedback, error_spans, confidence, photo_quality, failure_reason, debug.
type CheckResponse struct {
	Status        CheckStatus   `json:"status"` // "evaluated" | "need_better_photo" | "no_answer" | "internal_error"
	CanEvaluate   bool          `json:"can_evaluate"`
	IsCorrect     *bool         `json:"is_correct"` // nullable
	Feedback      string        `json:"feedback"`
	ErrorSpans    []ErrorSpan   `json:"error_spans"`    // nullable array
	Confidence    *float64      `json:"confidence"`     // nullable, 0-1
	PhotoQuality  *PhotoQuality `json:"photo_quality"`  // nullable
	FailureReason *string       `json:"failure_reason"` // nullable
	Debug         *CheckDebug   `json:"debug"`          // nullable
}
