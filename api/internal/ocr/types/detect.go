package types

type DetectInput struct {
	ImageB64  string `json:"image_b64"`
	Mime      string `json:"mime,omitempty"`
	GradeHint int    `json:"grade_hint,omitempty"`
}

// DetectResult — корневой объект DETECT
type DetectResult struct {
	VerbatimMode     bool           `json:"verbatim_mode"`
	WhitespacePolicy string         `json:"whitespace_policy"` // e.g. "preserve_all"
	OperatorsStrict  bool           `json:"operators_strict"`
	PageMeta         DetectPageMeta `json:"page_meta"`
	Tasks            []DetectTask   `json:"tasks"`
}

// DetectPageMeta — метаданные страницы/кадра
type DetectPageMeta struct {
	SourceID       string `json:"source_id"`
	PageNumber     int    `json:"page_number"`
	ImageBBoxUnits string `json:"image_bbox_units,omitempty"` // обычно "px"
}

// DetectTask — одна «задача» на странице
type DetectTask struct {
	OriginalNumber            string               `json:"original_number"`            // например, "№1" или "1."
	TitleRaw                  string               `json:"title_raw"`                  // как распознано в заголовке
	LettersDetected           []string             `json:"letters_detected,omitempty"` // A, Б, В...
	ExamplesPerLetterExpected *int                 `json:"examples_per_letter_expected,omitempty"`
	HasFaces                  bool                 `json:"has_faces"`
	PIIDetected               bool                 `json:"pii_detected"`
	MultipleTasksDetected     bool                 `json:"multiple_tasks_detected,omitempty"`
	Blocks                    []DetectBlock        `json:"blocks"`
	QualityChecks             *DetectQualityChecks `json:"quality_checks,omitempty"`
}

// DetectBlock — структурированный блок текста/таблицы/сетки
type DetectBlock struct {
	BlockID     string          `json:"block_id"`               // "title","conditions","examples","solution"...
	BlockLabel  string          `json:"block_label,omitempty"`  // произвольная метка/подзаголовок
	BlockRaw    string          `json:"block_raw"`              // сырой текст блока (без потери пробелов)
	ItemsRaw    []DetectItem    `json:"items_raw"`              // списковые элементы/строки/ячейки
	LayoutRaw   string          `json:"layout_raw,omitempty"`   // "table","grid","plain","two_column"...
	SemanticRaw *DetectSemantic `json:"semantic_raw,omitempty"` // структурная семантика блока (если удалось)
}

// DetectItem — элемент внутри блока (строка/ячейка/пункт)
type DetectItem struct {
	Text    string `json:"text"`
	GroupID string `json:"group_id"` // "A","B","C" или "row-1","left","right" и т.п.
}

// DetectSemantic — попытка восстановить табличную/столбцовую структуру
type DetectSemantic struct {
	Columns []string            `json:"columns,omitempty"`
	Rows    []DetectSemanticRow `json:"rows,omitempty"`
	Boxes   []DetectBox         `json:"boxes,omitempty"` // координатные ячейки (например, для сеток)
}

type DetectSemanticRow struct {
	Kind  string   `json:"kind"`  // "operand"|"operator"|"sum_line"|"result"|...
	Cells []string `json:"cells"` // содержимое по колонкам
}

type DetectBox struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

// DetectQualityChecks — быстрые проверки качества извлечения
type DetectQualityChecks struct {
	SentenceOrderPreserved         *bool `json:"sentence_order_preserved,omitempty"`
	LettersCountMatchesSource      *bool `json:"letters_count_matches_source,omitempty"`
	ExamplesPerLetterMatchesSource *bool `json:"examples_per_letter_matches_source,omitempty"`
	MulSignCorrectPerItem          *bool `json:"mul_sign_correct_per_item,omitempty"`
	ThousandsSpacePreserved        *bool `json:"thousands_space_preserved,omitempty"`
	LayoutRoundtripOK              *bool `json:"layout_roundtrip_ok,omitempty"`
	BoxesCountMatch                *bool `json:"boxes_count_match,omitempty"`
	AlignmentCheck                 *bool `json:"alignment_check,omitempty"`
}
