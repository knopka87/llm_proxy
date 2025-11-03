package types

// --- CHECK (v1) ----------------------------------------------------
// Соответствует схемам CHECK.request.v1 и CHECK.response.v1.

// CheckRequest — вход запроса (CHECK.request.v1)
type CheckRequest struct {
	NormTask   NormTask   `json:"norm_task"`   // Минимальная нормализованная форма задания
	NormAnswer NormAnswer `json:"norm_answer"` // Минимальная нормализованная форма ответа ученика
}

// ErrorSpan — диапазон ошибки в исходном ответе
type ErrorSpan struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Label string `json:"label"`
}

// CheckResponse — выход проверки (CHECK.response.v1)
type CheckResponse struct {
	IsCorrect  bool        `json:"is_correct"`
	Feedback   string      `json:"feedback"`
	ErrorSpans []ErrorSpan `json:"error_spans,omitempty"`
	Confidence float64     `json:"confidence,omitempty"` // [0,1]
}
