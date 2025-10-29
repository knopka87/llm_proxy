package types

import "encoding/json"

// --- CHECK SOLUTION (v1.2a) --------------------------------------------------
// Соответствует CHECK_Input.v1.2a и CheckSchema.v1.2a из pipeline_io_schemas.
// Эти структуры используются без утечки правильных ответов.

// CheckSolutionInput — входные данные проверки решения
// Обязательные поля по схеме: branch, subject, grade, student_normalized,
// expected_solution. additionalProperties: false (не enforce'ится на уровне Go).
// Внимание: expected_solution и hidden_correct_value — внутренние поля, не
// должны утекать пользователю в подсказках.
type CheckSolutionInput struct {
	Branch             string          `json:"branch"`                         // "math_branch"|"ru_branch"|"generic_branch"
	Subject            string          `json:"subject"`                        // "math"|"russian"|"generic"
	Grade              int             `json:"grade"`                          // 1..4
	StudentNormalized  NormalizeResult `json:"student_normalized"`             // Выход NORMALIZE_Output.v1
	ExpectedSolution   json.RawMessage `json:"expected_solution"`              // Эталон той же формы (внутренний)
	HiddenCorrectValue *string         `json:"hidden_correct_value,omitempty"` // Внутренний, если требуется
}

// CheckSolutionResult — строгий JSON по CheckSchema.v1.2a
// Обязательные поля: version, branch, verdict, confidence, safety.
type CheckSolutionResult struct {
	Version    string       `json:"version"`          // Всегда "1.2"
	Branch     string       `json:"branch"`           // "math_branch"|"ru_branch"|"generic_branch"
	Verdict    string       `json:"verdict"`          // "pass"|"fail"|"needs_more_info"
	Confidence float64      `json:"confidence"`       // [0,1]
	Issues     []CheckIssue `json:"issues,omitempty"` // ≤3
	MathChecks *MathChecks  `json:"math_checks,omitempty"`
	RuChecks   *RuChecks    `json:"ru_checks,omitempty"`
	Safety     CheckSafety  `json:"safety"`
}

// CheckIssue — запись о проблеме и способах исправления
type CheckIssue struct {
	Reason         string   `json:"reason"`                    // ≤180 символов
	FixSuggestions []string `json:"fix_suggestions,omitempty"` // ≤2, каждая ≤240 символов
}

// MathChecks — дополнительные проверки для математики
type MathChecks struct {
	UnitsOK        bool `json:"units_ok"`
	RoundingOK     bool `json:"rounding_ok"`
	SubstitutionOK bool `json:"substitution_ok"`
}

// RuChecks — дополнительные проверки для русского языка
type RuChecks struct {
	RuleRef        string `json:"rule_ref,omitempty"`
	Counterexample string `json:"counterexample,omitempty"`
}

// CheckSafety — обязательный блок безопасности результата
type CheckSafety struct {
	PIIRemoved    bool     `json:"pii_removed"`
	BannedContent bool     `json:"banned_content"`
	PIICategories []string `json:"pii_categories,omitempty"`
	Notes         string   `json:"notes,omitempty"`
}

// CheckSchemaVersion — вспомогательная константа версии схемы результата
const CheckSchemaVersion = "1.2"
