package types

type DetectInput struct {
	ImageB64  string `json:"image_b64"`
	Mime      string `json:"mime,omitempty"`
	GradeHint int    `json:"grade_hint,omitempty"`
}

// DetectResult — корневой объект DETECT
type DetectResult struct {
	Tasks []DetectTask `json:"tasks"`
}

// DetectTask — одна «задача» на странице
type DetectTask struct {
	OriginalNumber        string        `json:"original_number"` // например, "№1" или "1."
	TitleRaw              string        `json:"title_raw"`       // как распознано в заголовке
	MultipleTasksDetected bool          `json:"multiple_tasks_detected,omitempty"`
	Blocks                []DetectBlock `json:"blocks"`
}

// DetectBlock — структурированный блок текста/таблицы/сетки
type DetectBlock struct {
	BlockRaw string `json:"block_raw"` // сырой текст блока (без потери пробелов)
}
