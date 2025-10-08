package ocr

import "encoding/json"

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

type HintLevel string // Уровень подсказки

const (
	HintL1 HintLevel = "L1"
	HintL2 HintLevel = "L2"
	HintL3 HintLevel = "L3"
)

// HintInput Вход для генерации подсказки (User input из PROMPT_HINT v1.4)
type HintInput struct {
	Level                   HintLevel `json:"level"` // "L1" | "L2" | "L3"
	RawText                 string    `json:"raw_text"`
	Subject                 string    `json:"subject"` // "math" | "russian" | ...
	TaskType                string    `json:"task_type"`
	Grade                   int       `json:"grade"`          // 1..4
	SolutionShape           string    `json:"solution_shape"` // "number" | "string" | "steps" | "list"
	SubjectConfidence       float64   `json:"subject_confidence"`
	TaskTypeConfidence      float64   `json:"task_type_confidence"`
	TerminologyLevel        string    `json:"terminology_level"`    // "none" | "light" | "teacher"
	MethodTag               string    `json:"method_tag"`           // напр. "порядок_действий" и т.п.
	MathFlags               []string  `json:"math_flags,omitempty"` // ["check_units","order_of_operations","place_value","algorithmic_column"]
	RequiresContextFromText bool      `json:"requires_context_from_text"`
}

type HintResult struct {
	HintTitle       string   `json:"hint_title"`
	HintSteps       []string `json:"hint_steps"`
	ControlQuestion string   `json:"control_question"`
	NoFinalAnswer   bool     `json:"no_final_answer"` // Должно быть true
	AnalogyContext  string   `json:"analogy_context,omitempty"`
	TransferPrompt  string   `json:"transfer_prompt,omitempty"`
	Checklist       []string `json:"checklist,omitempty"`
	RuleHint        string   `json:"rule_hint,omitempty"`
	Meta            struct {
		Level            string   `json:"level"` // "L1"|"L2"|"L3"
		Subject          string   `json:"subject"`
		TaskType         string   `json:"task_type"`
		Grade            int      `json:"grade"`
		TerminologyLevel string   `json:"terminology_level"` // "none"|"light"|"teacher"
		ControlType      string   `json:"control_type"`      // "plan"|"checklist"|"self_explain"
		ComplexityBand   string   `json:"complexity_band"`   // "low"|"mid"|"high"
		MethodTag        string   `json:"method_tag,omitempty"`
		AnalogyAlignment string   `json:"analogy_alignment,omitempty"` // "matched"|"generic"
		MathFlags        []string `json:"math_flags,omitempty"`
		RuleRefs         []string `json:"rule_refs,omitempty"`
		LengthPolicy     struct {
			SoftCapsUsed   bool     `json:"soft_caps_used"`
			AnyOverflow    bool     `json:"any_overflow"`
			OverflowFields []string `json:"overflow_fields,omitempty"`
			OverflowReason string   `json:"overflow_reason,omitempty"` // "none"|"clarity"|"domain_specific"|"grade_support"
			LengthUsed     FlexMap  `json:"length_used,omitempty"`
		} `json:"length_policy"`
	} `json:"meta"`
}

type Entities struct {
	Numbers []float64 `json:"numbers"`
	Units   []string  `json:"units"`
	Names   []string  `json:"names"`
}

// FlexMap — гибкий парсер для length_used: допускает
// 1) объект { "field": 123 }, 2) числа как float, 3) массив пар [{field,len}|{k,v}|{name,value}]
type FlexMap map[string]int

func (m *FlexMap) UnmarshalJSON(b []byte) error {
	// Попробуем как map[string]int
	var mi map[string]int
	if err := json.Unmarshal(b, &mi); err == nil {
		*m = mi
		return nil
	}
	// Попробуем как map[string]float64
	var mf map[string]float64
	if err := json.Unmarshal(b, &mf); err == nil {
		res := make(map[string]int, len(mf))
		for k, v := range mf {
			res[k] = int(v + 0.5)
		}
		*m = res
		return nil
	}
	// Попробуем как массив пар
	var arr []map[string]any
	if err := json.Unmarshal(b, &arr); err == nil {
		res := map[string]int{}
		for _, p := range arr {
			var key string
			if s, ok := p["field"].(string); ok {
				key = s
			} else if s, ok := p["k"].(string); ok {
				key = s
			} else if s, ok := p["name"].(string); ok {
				key = s
			}
			var val int
			switch {
			case p["len"] != nil:
				if f, ok := p["len"].(float64); ok {
					val = int(f + 0.5)
				}
			case p["v"] != nil:
				if f, ok := p["v"].(float64); ok {
					val = int(f + 0.5)
				}
			case p["value"] != nil:
				if f, ok := p["value"].(float64); ok {
					val = int(f + 0.5)
				}
			}
			if key != "" {
				res[key] = val
			}
		}
		*m = res
		return nil
	}
	// Если вообще что-то странное — молча игнорируем
	*m = nil
	return nil
}
