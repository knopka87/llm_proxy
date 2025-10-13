package ocr

import "encoding/json"

type DetectInput struct {
	ImageB64  string `json:"image_b64"`
	Mime      string `json:"mime,omitempty"`
	GradeHint int    `json:"grade_hint,omitempty"`
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

type ParseInput struct {
	ImageB64 string       `json:"image_b64"`
	Options  ParseOptions `json:"options"`
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

type NormalizeInput struct {
	TaskID        string          `json:"task_id"`
	UserIDAnon    string          `json:"user_id_anon"`
	Grade         int             `json:"grade"`
	Subject       string          `json:"subject"`
	TaskType      string          `json:"task_type"`
	SolutionShape string          `json:"solution_shape"`
	Answer        NormalizeAnswer `json:"answer"`
	ParseContext  json.RawMessage `json:"parse_context"`
	Provider      string          `json:"provider,omitempty"`
	Model         string          `json:"model,omitempty"`
}

type NormalizeAnswer struct {
	Source   string `json:"source"` // text | photo
	Text     string `json:"text,omitempty"`
	PhotoB64 string `json:"photo_b64,omitempty"` // предпочтительно base64
	Mime     string `json:"mime,omitempty"`      // image/jpeg, image/png
	Lang     string `json:"lang,omitempty"`
}

// NormalizeResult — строгий JSON-ответ нормализации (см. NORMALIZE_ANSWER v1.2)
// Поля подобраны так, чтобы покрыть все случаи из политики: число/строка/шаги/список,
// неоднозначности, кандидаты, OCR-метаданные и предупреждения.
type NormalizeResult struct {
	Success       bool        `json:"success"`
	Shape         string      `json:"shape"`                    // target: number|string|steps|list
	ShapeDetected string      `json:"shape_detected,omitempty"` // фактическая форма ответа
	Value         interface{} `json:"value"`                    // число | строка | []string | null
	NumberKind    string      `json:"number_kind,omitempty"`    // integer|decimal|fraction|mixed_fraction|time|range|unknown
	Confidence    float64     `json:"confidence,omitempty"`     // 0..1

	AnswerSource        string   `json:"answer_source,omitempty"`         // text|photo
	SourceOCRConfidence *float64 `json:"source_ocr_confidence,omitempty"` // 0..1, при source=photo
	OCREngine           string   `json:"ocr_engine,omitempty"`

	Normalized *NormalizedInfo `json:"normalized,omitempty"` // как чистили исходный ответ
	Units      *UnitsInfo      `json:"units,omitempty"`      // единицы измерения

	Warnings   []string    `json:"warnings,omitempty"`   // мягкие предупреждения
	Spans      []Span      `json:"spans,omitempty"`      // позиции сущностей в тексте (опц.)
	Candidates []Candidate `json:"candidates,omitempty"` // альтернативные значения

	UncertainReasons       []string `json:"uncertain_reasons,omitempty"`
	NeedsClarification     bool     `json:"needs_clarification,omitempty"`
	NeedsUserActionMessage string   `json:"needs_user_action_message,omitempty"` // короткая подсказка ребенку (≤120)
	Error                  *string  `json:"error"`                               // null | код ошибки

	PIIFlag bool `json:"pii_flag,omitempty"` // найдены ли персональные данные
}

// NormalizedInfo — сведения о «чистке» исходного ответа.
type NormalizedInfo struct {
	Raw   string   `json:"raw,omitempty"`
	Clean string   `json:"clean,omitempty"`
	Notes []string `json:"notes,omitempty"`
}

// UnitsInfo — обнаруженные/нормализованные единицы измерения.
// Detected/Canonical держим указателями, чтобы уметь сериализовать null.
type UnitsInfo struct {
	Detected   *string  `json:"detected"`
	Canonical  *string  `json:"canonical"`
	IsCompound bool     `json:"is_compound"`
	Parts      []string `json:"parts,omitempty"`
	Kept       bool     `json:"kept"`
	Mismatch   bool     `json:"mismatch"`
}

// Span — позиция сущности в исходном тексте (необязательно).
type Span struct {
	SpanFrom int    `json:"span_from"`
	SpanTo   int    `json:"span_to"`
	Label    string `json:"label,omitempty"`
}

// Candidate — альтернативное прочтение значения (в т.ч. из-за исправлений/наложений).
type Candidate struct {
	Value    interface{} `json:"value"`
	SpanFrom int         `json:"span_from"`
	SpanTo   int         `json:"span_to"`
	Kind     string      `json:"kind"` // digit_number|word_number|time|range|operator|unit|strikethrough|overwritten|superscript|caret_insert|unknown
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
	Type  string `json:"type,omitempty"`  // digit|unit|format|step|item|position
	Index *int   `json:"index,omitempty"` // 0-based для шагов/элементов
	Note  string `json:"note,omitempty"`
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
	Expected        string   `json:"expected,omitempty"`
	ExpectedPrimary string   `json:"expected_primary,omitempty"`
	Alternatives    []string `json:"alternatives,omitempty"`
	Detected        string   `json:"detected,omitempty"`
	Policy          string   `json:"policy,omitempty"`
	Convertible     bool     `json:"convertible,omitempty"`
	Applied         string   `json:"applied,omitempty"` // пример: "mm->cm"
	Factor          *float64 `json:"factor,omitempty"`  // пример: 0.1
}

type NumberDiff struct {
	Abs              *float64 `json:"abs,omitempty"`
	Rel              *float64 `json:"rel,omitempty"`
	WithinTolerance  bool     `json:"within_tolerance,omitempty"`
	EquivalentByRule bool     `json:"equivalent_by_rule,omitempty"`
}

type StringMatch struct {
	Method string `json:"method,omitempty"` // exact|synonym|regex|case_fold|typo_lev1
	Passed bool   `json:"passed"`
}

type ListMatch struct {
	Matched        int      `json:"matched,omitempty"`
	Total          int      `json:"total,omitempty"`
	Extra          []string `json:"extra,omitempty"`
	Missing        []string `json:"missing,omitempty"`
	ExtraItemsList []string `json:"extra_items_list,omitempty"`
	OrderOK        bool     `json:"order_ok,omitempty"`
	PartialOK      bool     `json:"partial_ok,omitempty"`
}

type StepsMatch struct {
	Covered    int   `json:"covered,omitempty"`
	Total      int   `json:"total,omitempty"`
	Missing    []int `json:"missing,omitempty"`
	ExtraSteps []int `json:"extra_steps,omitempty"`
	OrderOK    bool  `json:"order_ok,omitempty"`
	PartialOK  bool  `json:"partial_ok,omitempty"`
}

type CheckSafety struct {
	NoFinalAnswerLeak  bool `json:"no_final_answer_leak"`
	NoMathResultInText bool `json:"no_math_result_in_text,omitempty"`
}

// --- ANALOGUE SOLUTION ----------------------------------------------
// Даёт разбор похожего задания тем же приёмом без утечки ответа исходной задачи.

// AnalogueSolutionInput — вход генерации аналога
// Важно: original_task_essence не должен содержать числа/слова из исходной задачи.
type AnalogueSolutionInput struct {
	TaskID              string `json:"task_id,omitempty"`
	UserIDAnon          string `json:"user_id_anon,omitempty"`
	Grade               int    `json:"grade,omitempty"`
	Subject             string `json:"subject,omitempty"` // math|russian|...
	TaskType            string `json:"task_type,omitempty"`
	MethodTag           string `json:"method_tag,omitempty"` // тот же приём решения
	DifficultyHint      string `json:"difficulty_hint,omitempty"`
	OriginalTaskEssence string `json:"original_task_essence"` // краткая суть без исходных чисел/слов
	Locale              string `json:"locale,omitempty"`      // ru (по умолчанию)
}

// AnalogueSolutionResult — строгий JSON по analogue.schema.json v1.1
// Не должен повторять исходные данные и не раскрывает правильный ответ оригинала.
type AnalogueSolutionResult struct {
	AnalogyTitle  string      `json:"analogy_title"`
	AnalogyTask   string      `json:"analogy_task"`
	AnalogyData   AnalogyData `json:"analogy_data"`
	SolutionSteps []string    `json:"solution_steps"` // 3–4 шага, короткие предложения

	// Мини‑проверки: структурные (yn/single_word/choice). Поддержан и старый строковый формат.
	MiniChecks []MiniCheckItem `json:"mini_checks,omitempty"`

	// Типовые ошибки: коды + сообщения; поддержан старый строковый формат (только сообщение)
	CommonMistakes []MistakeItem `json:"common_mistakes,omitempty"`

	SelfCheckRule  string `json:"self_check_rule,omitempty"`
	TransferBridge string `json:"transfer_bridge,omitempty"` // 2–3 шага переноса
	TransferCheck  string `json:"transfer_check,omitempty"`  // 1 вопрос для самопроверки переноса

	NextActionCode string `json:"next_action_code,omitempty"` // e.g. offer_micro_quiz

	// Доп. контроль когнитивной нагрузки и методической связки
	GradeTarget              *int   `json:"grade_target,omitempty"`
	ReadabilityHint          string `json:"readability_hint,omitempty"`            // ≤12 слов в предложении
	MethodRationale          string `json:"method_rationale,omitempty"`            // почему это тот же приём
	ContrastNote             string `json:"contrast_note,omitempty"`               // чем аналог отличается
	DistanceFromOriginalHint string `json:"distance_from_original_hint,omitempty"` // low|medium|high

	// Безопасность/антиликовая защита
	Safety                  AnalogueSafety `json:"safety"`
	LeakGuardPassed         bool           `json:"leak_guard_passed"`
	NoOriginalOverlapReport *OverlapReport `json:"no_original_overlap_report,omitempty"`
}

type AnalogyData struct {
	NumbersOrWords []string `json:"numbers_or_words,omitempty"`
	Units          []string `json:"units,omitempty"`
	Context        string   `json:"context,omitempty"`
}

// MiniCheckItem — поддерживает как структурный формат, так и старый строковый.
type MiniCheckItem struct {
	Type         string   `json:"type,omitempty"` // yn|single_word|choice
	Prompt       string   `json:"prompt,omitempty"`
	Options      []string `json:"options,omitempty"`       // для choice
	ExpectedForm string   `json:"expected_form,omitempty"` // форма ответа, не сам ответ
	Raw          string   `json:"raw,omitempty"`           // если пришла строка
}

func (m *MiniCheckItem) UnmarshalJSON(b []byte) error {
	// Строковый старый формат
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		m.Raw = s
		return nil
	}
	// Новый объектный формат
	type _mini MiniCheckItem
	var v _mini
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*m = MiniCheckItem(v)
	return nil
}

// MistakeItem — типовая ошибка: код + сообщение; поддерживает старый строковый формат.
type MistakeItem struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Raw     string `json:"raw,omitempty"` // если пришла строка
}

func (m *MistakeItem) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		m.Raw = s
		m.Message = s
		return nil
	}
	type _mist MistakeItem
	var v _mist
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*m = MistakeItem(v)
	return nil
}

// AnalogueSafety — базовые флаги безопасности
type AnalogueSafety struct {
	NoOriginalAnswerLeak bool `json:"no_original_answer_leak"`
}

type OverlapReport struct {
	OverlapPercent float64  `json:"overlap_percent,omitempty"`
	Notes          []string `json:"notes,omitempty"`
}
