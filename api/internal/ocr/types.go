package ocr

// Result — единый результат для всех движков.
// Для Yandex используются только Text (и, опционально, FoundTask).
type Result struct {
	// Всегда полезно иметь сырой текст, если удаётся его достать
	Text string

	// Детекция сути задачи
	FoundTask     bool
	FoundSolution bool

	// Если FoundSolution == true
	SolutionVerdict string // "correct" | "incorrect" | "uncertain"
	SolutionNote    string // краткое пояснение "где/какого рода" ошибка (без решения)

	// Подсказки L1→L3: от лёгкой наводки до подробного плана решения,
	// но без самого ответа/итогового вычисления.
	Hints []string // len=0 или 3 (предпочтительно)
}

type DetectResult struct {
	FinalState             string   `json:"final_state"`
	Confidence             float64  `json:"confidence"`
	NeedsRescan            bool     `json:"needs_rescan"`
	RescanReason           string   `json:"rescan_reason"`
	RescanCode             string   `json:"rescan_code"`
	MultipleTasksDetected  bool     `json:"multiple_tasks_detected"`
	TasksBrief             []string `json:"tasks_brief"`
	TopCandidateIndex      *int     `json:"top_candidate_index,omitempty"`
	AutoChoiceSuggested    bool     `json:"auto_choice_suggested"`
	DisambiguationNeeded   bool     `json:"disambiguation_needed"`
	DisambiguationReason   string   `json:"disambiguation_reason"`
	DisambiguationQuestion string   `json:"disambiguation_question"`
	FoundSolution          bool     `json:"found_solution"`
	SolutionRoughQuality   string   `json:"solution_rough_quality"`
	HasDiagramsOrFormulas  bool     `json:"has_diagrams_or_formulas"`
	HasFaces               bool     `json:"has_faces"`
	PIIDetected            bool     `json:"pii_detected"`
	SubjectGuess           string   `json:"subject_guess"`
	SubjectConfidence      float64  `json:"subject_confidence"`
	AltSubjects            []string `json:"alt_subjects"`
}

type ParseResult struct {
	FinalState          string   `json:"final_state"` // "recognized_task" | "needs_clarification"
	RawText             string   `json:"raw_text"`
	Question            string   `json:"question"`
	Entities            Entities `json:"entities"`
	Confidence          float64  `json:"confidence"`
	NeedsRescan         bool     `json:"needs_rescan"`
	RescanReason        string   `json:"rescan_reason"`
	Subject             string   `json:"subject"` // "math" | "russian" | ...
	TaskType            string   `json:"task_type"`
	SolutionShape       string   `json:"solution_shape"` // "number" | "string" | "steps" | "list"
	MeaningChangeRisk   float64  `json:"meaning_change_risk"`
	BracketedSpansCount int      `json:"bracketed_spans_count"`
	ConfirmationNeeded  bool     `json:"confirmation_needed"`
	ConfirmationReason  string   `json:"confirmation_reason"` // "low_confidence" | ... | "none"
	Grade               int      `json:"grade"`
	GradeAlignment      string   `json:"grade_alignment"` // "aligned" | "maybe_lower" | ...
}

type Entities struct {
	Numbers []float64 `json:"numbers"`
	Units   []string  `json:"units"`
	Names   []string  `json:"names"`
}
