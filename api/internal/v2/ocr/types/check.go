package types

// --- CHECK (v1) ----------------------------------------------------
// Соответствует схемам CHECK.request.v1 и CHECK.response.v1.

type StudentCheck struct {
	Grade   int64  `json:"grade"`
	Subject string `json:"subject"`
	Locale  string `json:"locale"`
}

type TaskStructCheck struct {
	TaskTextClean string           `json:"task_text_clean"`
	VisualFacts   []VisualFact     `json:"visual_facts"`
	QualityFlags  ParseTaskQuality `json:"quality_flags"`
	Items         []ParseItem      `json:"items"`
}

// CheckRequest — вход запроса (CHECK.request.v1)
type CheckRequest struct {
	Image            string          `json:"image"`
	TaskStruct       TaskStructCheck `json:"task_struct"`
	RawTaskText      string          `json:"raw_task_text"`
	Student          StudentCheck    `json:"student"`
	PhotoQualityHint string          `json:"photo_quality_hint"`
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
	ExpectedAnswer   *string `json:"expected_answer,omitempty"` // P2.2: что сравниваем
	DecisionReason   *string `json:"decision_reason,omitempty"` // P2.2: причина решения
}

// CheckDecision — результат проверки ответа (P0.3: enum вместо nullable bool)
type CheckDecision string

const (
	CheckDecisionCorrect         CheckDecision = "correct"          // ответ верный
	CheckDecisionIncorrect       CheckDecision = "incorrect"        // ответ неверный
	CheckDecisionNeedAnnotation  CheckDecision = "need_annotation"  // нужна аннотация/подпись
	CheckDecisionInvalidExpected CheckDecision = "invalid_expected" // противоречие в эталоне
	CheckDecisionCannotEvaluate  CheckDecision = "cannot_evaluate"  // невозможно честно проверить
)

// CheckResponse — CHECK.response.v1
// Required: status, can_evaluate, decision, feedback, error_spans, confidence, photo_quality, failure_reason, debug.
type CheckResponse struct {
	Status        CheckStatus   `json:"status"` // "evaluated" | "need_better_photo" | "no_answer" | "internal_error"
	CanEvaluate   bool          `json:"can_evaluate"`
	Decision      CheckDecision `json:"decision"`   // P0.3: enum вместо is_correct
	IsCorrect     *bool         `json:"is_correct"` // deprecated: для обратной совместимости
	Feedback      string        `json:"feedback"`
	ErrorSpans    []ErrorSpan   `json:"error_spans"`    // nullable array
	Confidence    *float64      `json:"confidence"`     // nullable, 0-1
	PhotoQuality  *PhotoQuality `json:"photo_quality"`  // nullable
	FailureReason *string       `json:"failure_reason"` // nullable
	Debug         *CheckDebug   `json:"debug"`          // nullable
}

// NormalizeDecision заполняет Decision из IsCorrect для обратной совместимости
func (r *CheckResponse) NormalizeDecision() {
	if r.Decision != "" {
		return // Decision уже установлен
	}
	if !r.CanEvaluate {
		r.Decision = CheckDecisionCannotEvaluate
		return
	}
	if r.IsCorrect == nil {
		r.Decision = CheckDecisionCannotEvaluate
		return
	}
	if *r.IsCorrect {
		r.Decision = CheckDecisionCorrect
	} else {
		r.Decision = CheckDecisionIncorrect
	}
}

// SetIsCorrectFromDecision заполняет IsCorrect из Decision для обратной совместимости
func (r *CheckResponse) SetIsCorrectFromDecision() {
	switch r.Decision {
	case CheckDecisionCorrect:
		t := true
		r.IsCorrect = &t
	case CheckDecisionIncorrect:
		f := false
		r.IsCorrect = &f
	default:
		r.IsCorrect = nil
	}
}
