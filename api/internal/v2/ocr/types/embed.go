package types

// EmbedRequest — запрос на эмбеддинги (батч строк).
type EmbedRequest struct {
	LLMName string   `json:"llm_name"`
	Input   []string `json:"input"`
}

// EmbedResponse — векторы для каждой строки входа (порядок сохраняется).
type EmbedResponse struct {
	SchemaVersion string      `json:"schema_version"`
	Vectors       [][]float32 `json:"vectors"`
}
