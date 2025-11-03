package types

// NormalizeRequest — вход запроса (NORMALIZE.request.v1)
// required: task_struct, raw_answer_text
type NormalizeRequest struct {
	TaskStruct    TaskStruct `json:"task_struct"`
	RawAnswerText string     `json:"raw_answer_text"`
}

// NormTask соответствует полю norm_task в NORMALIZE.response.v1
// Kind: "math" | "ru" | "generic"
type NormTask struct {
	Kind string `json:"kind"`
	Data any    `json:"data"`
}

// NormAnswer соответствует полю norm_answer в NORMALIZE.response.v1
// Value допускает string | float64 | bool | []any | map[string]any | nil
// Units — необязательное поле
type NormAnswer struct {
	Value any     `json:"value"`
	Units *string `json:"units,omitempty"`
}

// NormalizeResponse — выход (NORMALIZE.response.v1)
// required: norm_task, norm_answer
// Schema: normalize.schema.json
//
//	norm_task.kind ∈ {"math","ru","generic"}
//	norm_task.data — object (no additional properties expected)
//	norm_answer.value — string | number | boolean | array | object | null
//	norm_answer.units — optional string
type NormalizeResponse struct {
	NormTask   NormTask   `json:"norm_task"`
	NormAnswer NormAnswer `json:"norm_answer"`
}
