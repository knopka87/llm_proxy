package types

type ParseInput struct {
	ImageB64 string       `json:"image_b64"`
	Options  ParseOptions `json:"options"`
}

type ParseResult struct {
	RawText  string `json:"raw_text"`
	Question string `json:"question"`

	Confidence    float64 `json:"confidence"`
	NeedsRescan   bool    `json:"needs_rescan"`
	RescanReason  string  `json:"rescan_reason"`
	Subject       string  `json:"subject"`
	TaskType      string  `json:"task_type"`
	SolutionShape string  `json:"solution_shape"`

	ConfirmationNeeded bool   `json:"confirmation_needed"`
	ConfirmationReason string `json:"confirmation_reason"`
	Grade              int    `json:"grade"`
}

// ParseOptions — опции для этапа Parse (подсказки модели и служебные метаданные).
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

	// Необязательная модель (перекрывает e.Model при вызове движка)
	ModelOverride string
}
