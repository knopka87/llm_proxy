package types

type HintLevel string // Уровень подсказки

const (
	HintL1 HintLevel = "L1"
	HintL2 HintLevel = "L2"
	HintL3 HintLevel = "L3"
)

// HintInput Вход для генерации подсказки (User input из PROMPT_HINT v1.4)
type HintInput struct {
	Level            HintLevel `json:"level"` // "L1" | "L2" | "L3"
	RawText          string    `json:"raw_text"`
	Subject          string    `json:"subject"` // "math" | "russian" | ...
	TaskType         string    `json:"task_type"`
	Grade            int       `json:"grade"`             // 1..4
	SolutionShape    string    `json:"solution_shape"`    // "number" | "string" | "steps" | "list"
	TerminologyLevel string    `json:"terminology_level"` // "none" | "light" | "teacher"
}

type HintResult struct {
	// Уровень подсказки: L1 — наводящий вопрос, L2 — практический совет, L3 — общий алгоритм.
	// В схеме это поле называется `hint_title` и имеет enum [L1, L2, L3].
	HintTitle HintLevel `json:"hint_title"`

	// Короткие шаги подсказки. По схеме: minItems=1, maxItems=3; каждый элемент 10..150 символов.
	HintSteps []string `json:"hint_steps"`

	// Опционально. Класс ученика 1..4 (для логирования). В схеме поле не обязательное.
	Grade *int `json:"grade,omitempty"`
}
