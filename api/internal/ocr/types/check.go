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
	Shape  string             `json:"shape"`
	Units  *UnitsExpectedSpec `json:"units,omitempty"`
	Number *NumberSpec        `json:"number_spec,omitempty"`
	String *StringSpec        `json:"string_spec,omitempty"`
	List   *ListSpec          `json:"list_spec,omitempty"`
	Steps  *StepsSpec         `json:"steps_spec,omitempty"`
}

type UnitsExpectedSpec struct {
	Policy          string   `json:"policy,omitempty"` // required|forbidden|optional
	ExpectedPrimary string   `json:"expected_primary,omitempty"`
	Alternatives    []string `json:"alternatives,omitempty"`
}

type NumberSpec struct {
	Value                 string   `json:"value"` // не раскрывать пользователю
	ToleranceAbs          *float64 `json:"tolerance_abs,omitempty"`
	ToleranceRel          *float64 `json:"tolerance_rel,omitempty"`
	AllowEquivalentByRule bool     `json:"allow_equivalent_by_rule,omitempty"`
	Format                string   `json:"format,omitempty"` // percent|degree|currency|time|range|plain
}

type StringSpec struct {
	AcceptSet     []string `json:"accept_set,omitempty"`
	Regex         string   `json:"regex,omitempty"`
	Synonyms      []string `json:"synonyms,omitempty"`
	CaseFold      bool     `json:"case_fold,omitempty"`
	AllowTypoLev1 bool     `json:"allow_typo_lev1,omitempty"`
	Expected      string   `json:"expected,omitempty"`
}

type ListSpec struct {
	Expected     []string `json:"expected,omitempty"`
	OrderMatters bool     `json:"order_matters,omitempty"`
	AllowExtra   bool     `json:"allow_extra,omitempty"`
	AllowMissing bool     `json:"allow_missing,omitempty"`
}

type StepsSpec struct {
	Expected     []string `json:"expected,omitempty"`
	OrderMatters bool     `json:"order_matters,omitempty"`
}

// CheckSolutionResult — строгий JSON по check.schema.json v1.1
// Не должен раскрывать правильный ответ.
type CheckSolutionResult struct {
	Verdict          string          `json:"verdict"` // correct|incorrect|uncertain
	ShortHint        string          `json:"short_hint,omitempty"`
	ReasonCodes      []string        `json:"reason_codes,omitempty"` // ≤2
	ErrorSpot        *ErrorSpot      `json:"error_spot,omitempty"`
	Comparison       CheckComparison `json:"comparison"`
	NextActionCode   string          `json:"next_action_code,omitempty"`
	SpeakableMessage string          `json:"speakable_message,omitempty"`
	Safety           CheckSafety     `json:"safety"`
	LeakGuardPassed  bool            `json:"leak_guard_passed"`
	CheckConfidence  float64         `json:"check_confidence,omitempty"`
	PolicyApplied    []string        `json:"policy_applied,omitempty"`
}

type ErrorSpot struct {
	Type        string  `json:"type"`  // "step"|"item"
	Index       int     `json:"index"` // 0-based
	ExpectedTag *string `json:"expected_tag,omitempty"`
}

type CheckComparison struct {
	ShapeOK              bool             `json:"shape_ok"`
	Units                *UnitsComparison `json:"units,omitempty"`
	NumberDiff           *NumberDiff      `json:"number_diff,omitempty"`
	StringMatch          *StringMatch     `json:"string_match,omitempty"`
	ListMatch            *ListMatch       `json:"list_match,omitempty"`
	StepsMatch           *StepsMatch      `json:"steps_match,omitempty"`
	InputCandidatesCount int              `json:"input_candidates_count,omitempty"`
}

type UnitsComparison struct {
	Expected        []string `json:"expected"`
	ExpectedPrimary *string  `json:"expected_primary,omitempty"`
	Alternatives    []string `json:"alternatives,omitempty"`
	Detected        *string  `json:"detected"`
	Policy          string   `json:"policy"` // "required"|"forbidden"|"optional"
	Convertible     bool     `json:"convertible"`
	Applied         *string  `json:"applied,omitempty"`
	Factor          *float64 `json:"factor,omitempty"`
}

type NumberDiff struct {
	Abs              *float64 `json:"abs"`
	Rel              *float64 `json:"rel"`
	WithinTolerance  bool     `json:"within_tolerance"`
	EquivalentByRule *bool    `json:"equivalent_by_rule,omitempty"`
}

type StringMatch struct {
	Mode     string   `json:"mode"`               // "exact"|"regex"|"synonym"|"case_fold"
	Distance *float64 `json:"distance,omitempty"` // может быть null
}

type ListMatch struct {
	Matched        int      `json:"matched"`
	Total          int      `json:"total"`
	Extra          int      `json:"extra"`
	Missing        []string `json:"missing,omitempty"`
	ExtraItemsList []string `json:"extra_items_list,omitempty"`
}

type StepsMatch struct {
	Covered    []string `json:"covered,omitempty"`
	Missing    []string `json:"missing,omitempty"`
	OrderOK    bool     `json:"order_ok"`
	PartialOK  bool     `json:"partial_ok,omitempty"`
	ExtraSteps []string `json:"extra_steps,omitempty"`
}

type CheckSafety struct {
	NoPII              bool  `json:"no_pii"`
	NoFinalAnswerLeak  bool  `json:"no_final_answer_leak"`
	NoMathResultInText *bool `json:"no_math_result_in_text,omitempty"`
}
