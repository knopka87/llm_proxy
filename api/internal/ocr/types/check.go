package types

import "encoding/json"

// --- CHECK SOLUTION (v1.1) --------------------------------------------------
// Структуры соответствуют инструкции CHECK_SOLUTION v1.1 и check.schema.json.
// Они используются в llm_proxy/checksolution.go и в провайдерах LLM для
// формирования и парсинга результата без утечки финальных ответов.

// CheckSolutionInput — входные данные проверки решения
// ВНИМАНИЕ: Expected содержит сведения о верном решении и политиках сравнения,
// но сервис не должен раскрывать правильный ответ пользователю.
type CheckSolutionInput struct {
	TaskID       string          `json:"task_id,omitempty"`
	UserIDAnon   string          `json:"user_id_anon,omitempty"`
	Subject      string          `json:"subject,omitempty"` // math|russian|...
	Grade        int             `json:"grade,omitempty"`
	ParseContext json.RawMessage `json:"parse_context,omitempty"`

	Student  NormalizeResult  `json:"student"`           // нормализованный ответ ученика
	Expected ExpectedSolution `json:"expected_solution"` // ожидаемая форма/политики
}

// ExpectedSolution — ожидаемая форма и политики для сравнения
// Shape: number|string|steps|list
// Units.policy: required|forbidden|optional
// Числовое значение хранится как строка, чтобы поддерживать дроби/смешанные.
type ExpectedSolution struct {
	Shape string             `json:"shape"`
	Units *UnitsExpectedSpec `json:"units,omitempty"`
}

type UnitsExpectedSpec struct {
	Policy          string   `json:"policy,omitempty"` // required|forbidden|optional
	ExpectedPrimary string   `json:"expected_primary,omitempty"`
	Alternatives    []string `json:"alternatives,omitempty"`
}

// CheckSolutionResult — строгий JSON по check.schema.json v1.1
// Не должен раскрывать правильный ответ.
type CheckSolutionResult struct {
	Verdict          string          `json:"verdict"` // correct|incorrect|uncertain
	ShortHint        string          `json:"short_hint,omitempty"`
	ReasonCodes      []string        `json:"reason_codes,omitempty"` // ≤2
	Comparison       CheckComparison `json:"comparison"`
	NextActionCode   string          `json:"next_action_code,omitempty"`
	SpeakableMessage string          `json:"speakable_message,omitempty"`
}

type CheckComparison struct {
	Units       *UnitsComparison `json:"units,omitempty"`
	NumberDiff  *NumberDiff      `json:"number_diff,omitempty"`
	StringMatch *StringMatch     `json:"string_match,omitempty"`
	ListMatch   *ListMatch       `json:"list_match,omitempty"`
	StepsMatch  *StepsMatch      `json:"steps_match,omitempty"`
}

type UnitsComparison struct {
	Detected *string `json:"detected"`
	Applied  *string `json:"applied,omitempty"`
}

type NumberDiff struct {
	WithinTolerance bool `json:"within_tolerance"`
}

type StringMatch struct {
	Mode string `json:"mode"` // "exact"|"regex"|"synonym"|"case_fold"
}

type ListMatch struct {
	Total   int      `json:"total"`
	Extra   int      `json:"extra"`
	Missing []string `json:"missing,omitempty"`
}

type StepsMatch struct {
	Missing    []string `json:"missing,omitempty"`
	OrderOK    bool     `json:"order_ok"`
	ExtraSteps []string `json:"extra_steps,omitempty"`
}
