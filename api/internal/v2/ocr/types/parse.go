package types

// Subject represents a subject hint for PARSE.
// Allowed values: "math", "russian", "generic".

const (
	SubjectMath    Subject = "math"
	SubjectRussian Subject = "russian"
	SubjectGeneric Subject = "generic"
)

type Subject string

func (s Subject) String() string {
	switch s {
	case SubjectMath:
		return string(SubjectMath)
	case SubjectRussian:
		return string(SubjectRussian)
	default:
		return string(SubjectGeneric)
	}
}

// SolutionShape represents the expected/guessed solution shape.
// Allowed values: "number", "string", "list", "steps".
type SolutionShape string

const (
	SolutionShapeNumber SolutionShape = "number"
	SolutionShapeString SolutionShape = "string"
	SolutionShapeList   SolutionShape = "list"
	SolutionShapeSteps  SolutionShape = "steps"
)

// ParseInput — соответствует схеме PARSE_Input.v2.3.
// raw_task_text — обязателен;
// subject_hint и grade_hint — допускают null и могут отсутствовать.
type ParseInput struct {
	RawTaskText string   `json:"raw_task_text"`          // required
	SubjectHint *Subject `json:"subject_hint,omitempty"` // nullable: "math" | "russian" | "generic"
	GradeHint   *int     `json:"grade_hint,omitempty"`   // nullable: 1..4
}

// ParseWarning — перечисление предупреждений из PARSE_ParseSchema.v2.3.
type ParseWarning string

const (
	ParseWarningHasHandwrittenAnswers ParseWarning = "has_handwritten_answers"
	ParseWarningLowQualityOCR         ParseWarning = "low_quality_ocr"
	ParseWarningIncompleteText        ParseWarning = "incomplete_text"
)

// ParseResult — соответствует схеме PARSE_ParseSchema.v2.3.
// Единственное обязательное поле — solution_shape_guess.
// Остальные поля опциональны и присутствуют при наличии оценки/гипотезы.
type ParseResult struct {
	GradeGuess         *int           `json:"grade_guess,omitempty"`      // 1..4
	GradeConfidence    *float64       `json:"grade_confidence,omitempty"` // 0..1
	SolutionShapeGuess SolutionShape  `json:"solution_shape_guess"`       // required
	HasSubparts        *bool          `json:"has_subparts,omitempty"`
	Subparts           []string       `json:"subparts,omitempty"` // <= 6 элементов, каждый до 8 символов
	Warnings           []ParseWarning `json:"warnings,omitempty"` // <= 3 элемента
}

/*
Deprecated: сохранено для обратной совместимости кода, который мог
использовать внутренние опции. Новая схема PARSE_Input.v2.3 больше не
содержит вложенных опций — используйте поля ParseInput напрямую.
*/
type ParseOptions struct {
	// Подсказки для модели
	GradeHint   int    // Предполагаемый класс (1–4), 0 если неизвестно
	SubjectHint string // "math" | "russian" | "" — если известно из DETECT

	// Метаданные для кэша/аудита
	ChatID       int64  // Идентификатор чата
	MediaGroupID string // Telegram MediaGroupID, пусто если одиночное фото
	ImageHash    string // SHA-256 объединённого изображения (util.SHA256Hex)

	// Дизамбигуация по выбору пользователя (если на фото несколько заданий)
	SelectedTaskIndex int    // Индекс выбранного задания (0-based), -1 если не выбран
	SelectedTaskBrief string // Краткое описание выбранного пункта (из DETECT), может быть пустым

	// Необязательная модель (перекрывает e.model при вызове движка)
	ModelOverride string
}
