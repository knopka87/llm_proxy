package types

// NormalizeRequest — вход запроса (NORMALIZE.request.v1)
// required: task_struct, raw_answer_text
type NormalizeRequest struct {
	TaskStruct    TaskStruct `json:"task_struct"`
	RawTaskText   string     `json:"raw_task_text"`
	RawAnswerText string     `json:"raw_answer_text"`
}

// NormTask соответствует полю norm_task в NORMALIZE.response.v1
// Kind: "math" | "ru" | "generic"
type NormTask struct {
	Kind string `json:"kind"`
	Data string `json:"data"`
}

// NormAnswer соответствует полю norm_answer в NORMALIZE.response.v1
type NormAnswer struct {
	Value string  `json:"value"`
	Units *string `json:"units,omitempty"`
}

// NormalizeResponse — выход (NORMALIZE.response.v1)
// required: norm_task, norm_answer
// Schema: normalize.schema.json
//
//	norm_task.kind ∈ {"math","ru","generic"}
//	norm_task.data — JSON object
//	norm_answer.value — string
//	norm_answer.units — optional string
type NormalizeResponse struct {
	NormTask   NormTask   `json:"norm_task"`
	NormAnswer NormAnswer `json:"norm_answer"`
}
