package types

// NormalizeRequest — вход запроса (NORMALIZE.request.v1)
// required: task_struct, raw_answer_text
type NormalizeRequest struct {
	TaskStruct    TaskStruct `json:"task_struct"`
	RawAnswerText string     `json:"raw_answer_text"`
}

// NormalizeResponse — выход (NORMALIZE.response.v1)
// required: norm_task, norm_answer
type NormalizeResponse struct {
	NormTask   map[string]interface{} `json:"norm_task"`
	NormAnswer map[string]interface{} `json:"norm_answer"`
}
