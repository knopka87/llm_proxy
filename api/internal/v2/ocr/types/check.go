package types

import (
	"fmt"
	"strings"
)

// --- CHECK (v1) ----------------------------------------------------
// Соответствует схемам CHECK.request.v1 и CHECK.response.v1.

type StudentCheck struct {
	Grade   int64  `json:"grade"`
	Subject string `json:"subject"`
	Locale  string `json:"locale"`
}

type TaskStructCheck struct {
	TaskTextClean   string           `json:"task_text_clean"`
	VisualReasoning *string          `json:"visual_reasoning,omitempty"` // описание визуальных элементов из PARSE
	VisualFacts     []VisualFact     `json:"visual_facts"`
	QualityFlags    ParseTaskQuality `json:"quality_flags"`
	Items           []ParseItem      `json:"items"`
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
	RawAnswerText        *string               `json:"raw_answer_text"`
	NormalizedAnswer     *string               `json:"normalized_answer"`
	ExpectedAnswer       *string               `json:"expected_answer,omitempty"`
	DecisionReason       *string               `json:"decision_reason,omitempty"`
	VerificationMethod   *string               `json:"verification_method,omitempty"`
	ParseConsistent      *bool                 `json:"parse_consistent,omitempty"`
	VerificationComplete *bool                 `json:"verification_complete,omitempty"`
	VisualEvidence       []CheckVisualEvidence `json:"visual_evidence,omitempty"`
}

// CheckVisualEvidence is one independently observed visual assertion.
type CheckVisualEvidence struct {
	ObjectID string `json:"object_id"`
	Observed string `json:"observed"`
	Expected string `json:"expected"`
	Matches  bool   `json:"matches"`
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

// CheckErrorDetails — технические детали ошибки для родительского отчёта.
// Заполняется только при decision=incorrect и can_evaluate=true.
type CheckErrorDetails struct {
	Topic                  string `json:"topic"`                   // математическая тема
	ErrorType              string `json:"error_type"`              // тип ошибки
	BriefForReport         string `json:"brief_for_report"`        // для родителя
	PracticeRecommendation string `json:"practice_recommendation"` // что потренировать
}

// CheckResponse — CHECK.response.v1
// Required: status, can_evaluate, decision, feedback, error_spans, confidence, photo_quality, failure_reason, debug, error_details.
type CheckResponse struct {
	Status        CheckStatus        `json:"status"` // "evaluated" | "need_better_photo" | "no_answer" | "internal_error"
	CanEvaluate   bool               `json:"can_evaluate"`
	Decision      CheckDecision      `json:"decision"`   // P0.3: enum вместо is_correct
	IsCorrect     *bool              `json:"is_correct"` // deprecated: для обратной совместимости
	Feedback      string             `json:"feedback"`
	ErrorSpans    []ErrorSpan        `json:"error_spans"`    // nullable array
	Confidence    *float64           `json:"confidence"`     // nullable, 0-1
	PhotoQuality  *PhotoQuality      `json:"photo_quality"`  // nullable
	FailureReason *string            `json:"failure_reason"` // nullable
	Debug         *CheckDebug        `json:"debug"`          // nullable
	ErrorDetails  *CheckErrorDetails `json:"error_details"`  // технические детали для отчёта родителю
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

// ValidateSemantics enforces cross-field and evidence invariants that cannot be
// represented reliably by JSON Schema alone.
func (r CheckResponse) ValidateSemantics(in CheckRequest) error {
	if r.CanEvaluate {
		if r.Status != CheckStatusEvaluated {
			return fmt.Errorf("can_evaluate=true requires status=evaluated")
		}
		if r.Decision != CheckDecisionCorrect && r.Decision != CheckDecisionIncorrect {
			return fmt.Errorf("can_evaluate=true requires correct or incorrect decision")
		}
		if r.Confidence == nil || r.FailureReason != nil {
			return fmt.Errorf("evaluated response requires confidence and no failure_reason")
		}
		if r.Debug == nil || r.Debug.ExpectedAnswer == nil || strings.TrimSpace(*r.Debug.ExpectedAnswer) == "" {
			return fmt.Errorf("evaluated response requires independently verified expected_answer")
		}
		if r.Debug.NormalizedAnswer == nil || strings.TrimSpace(*r.Debug.NormalizedAnswer) == "" {
			return fmt.Errorf("evaluated response requires normalized_answer")
		}
		if !checkRequestIsVisual(in) {
			normalizedStudent := normalizeCheckAnswer(*r.Debug.NormalizedAnswer)
			normalizedExpected := normalizeCheckAnswer(*r.Debug.ExpectedAnswer)
			if r.Decision == CheckDecisionCorrect && normalizedStudent != normalizedExpected {
				return fmt.Errorf("correct decision contradicts normalized and expected answers")
			}
			if r.Decision == CheckDecisionIncorrect && normalizedStudent == normalizedExpected {
				return fmt.Errorf("incorrect decision contradicts equal normalized and expected answers")
			}
		}
		if r.Debug.VerificationComplete == nil || !*r.Debug.VerificationComplete {
			return fmt.Errorf("evaluated response requires verification_complete=true")
		}
		if r.Debug.VerificationMethod == nil || strings.TrimSpace(*r.Debug.VerificationMethod) == "" {
			return fmt.Errorf("evaluated response requires verification_method")
		}
		if r.Debug.ParseConsistent == nil || !*r.Debug.ParseConsistent {
			return fmt.Errorf("evaluated response requires parse_consistent=true")
		}
		if r.Debug.DecisionReason == nil || !strings.Contains(*r.Debug.DecisionReason, "independent_verification") {
			return fmt.Errorf("evaluated response requires independent_verification decision_reason")
		}
		if checkRequestIsVisual(in) {
			if len(r.Debug.VisualEvidence) == 0 {
				return fmt.Errorf("visual task requires visual_evidence")
			}
			for i, evidence := range r.Debug.VisualEvidence {
				if strings.TrimSpace(evidence.ObjectID) == "" || strings.TrimSpace(evidence.Observed) == "" || strings.TrimSpace(evidence.Expected) == "" {
					return fmt.Errorf("visual_evidence[%d] is incomplete", i)
				}
			}
		}
		return nil
	}

	if r.Decision == CheckDecisionCorrect || r.Decision == CheckDecisionIncorrect {
		return fmt.Errorf("can_evaluate=false cannot have evaluated decision")
	}
	if r.Confidence != nil {
		return fmt.Errorf("can_evaluate=false requires confidence=null")
	}
	if r.Decision == CheckDecisionInvalidExpected {
		if r.FailureReason == nil || *r.FailureReason != "parse_inconsistency" {
			return fmt.Errorf("invalid_expected requires parse_inconsistency")
		}
	}
	return nil
}

func normalizeCheckAnswer(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ",", ".")
	return strings.Join(strings.Fields(value), "")
}

func checkRequestIsVisual(in CheckRequest) bool {
	if in.TaskStruct.VisualReasoning != nil && strings.TrimSpace(*in.TaskStruct.VisualReasoning) != "" {
		return true
	}
	if len(in.TaskStruct.VisualFacts) > 0 {
		return true
	}
	if len(in.TaskStruct.Items) == 0 {
		return false
	}
	item := in.TaskStruct.Items[0]
	if item.PedKeys.Format == "drawing" || item.PedKeys.Format == "diagram" || item.PedKeys.Format == "visual" {
		return true
	}
	return containsString(item.PedKeys.Constraints, "needs_visual")
}

// ConservativeCheckResponse avoids publishing an unsupported correct/incorrect
// verdict when semantic validation still fails after one retry.
func ConservativeCheckResponse() CheckResponse {
	reason := "ground_truth_unverified"
	return CheckResponse{
		Status:        CheckStatusInternalError,
		CanEvaluate:   false,
		Decision:      CheckDecisionCannotEvaluate,
		Feedback:      "Не удалось надёжно подтвердить эталон ответа. Попробуй проверить решение ещё раз позже.",
		ErrorSpans:    nil,
		Confidence:    nil,
		PhotoQuality:  nil,
		FailureReason: &reason,
		Debug:         nil,
		ErrorDetails:  nil,
	}
}
