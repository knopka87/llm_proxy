package types

type ParseInput struct {
	ImageB64 string       `json:"image_b64"`
	Options  ParseOptions `json:"options"`
}

type ParseResult struct {
	FinalState string   `json:"final_state"`
	RawText    string   `json:"raw_text"`
	Question   string   `json:"question"`
	Entities   Entities `json:"entities"`

	Confidence        float64 `json:"confidence"`
	NeedsRescan       bool    `json:"needs_rescan"`
	RescanReason      string  `json:"rescan_reason"`
	Subject           string  `json:"subject"`
	TaskType          string  `json:"task_type"`
	SolutionShape     string  `json:"solution_shape"`
	MeaningChangeRisk float64 `json:"meaning_change_risk"`

	BracketedSpansCount int    `json:"bracketed_spans_count"`
	ConfirmationNeeded  bool   `json:"confirmation_needed"`
	ConfirmationReason  string `json:"confirmation_reason"`
	Grade               int    `json:"grade"`
	GradeAlignment      string `json:"grade_alignment"`

	OriginalNumber            string   `json:"original_number"`
	HasSubparts               bool     `json:"has_subparts"`
	SubpartsLabels            []string `json:"subparts_labels"`
	SolutionFragmentsDetected bool     `json:"solution_fragments_detected"`
	StrippedSolutionSpans     [][2]int `json:"stripped_solution_spans"`
	HasDiagramsOrFormulas     bool     `json:"has_diagrams_or_formulas"`
	RoutingHint               string   `json:"routing_hint"`

	AttachmentsUsed []string `json:"attachments_used,omitempty"`
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

type Entities struct {
	Numbers []float64 `json:"numbers"`
	Units   []string  `json:"units"`
	Names   []string  `json:"names"`
}
