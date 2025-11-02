package types

// OCRRequest — вход запроса (OCR.request.v1)
// required: image; optional: locale ("ru-RU" | "en-US")
type OCRRequest struct {
	Image  string `json:"image"`
	Locale string `json:"locale,omitempty"` // "ru-RU" | "en-US"
}

// OCRResponse — выход (OCR.response.v1)
// required: raw_answer_text; optional: confidence [0,1]
type OCRResponse struct {
	RawAnswerText string  `json:"raw_answer_text"`
	Confidence    float64 `json:"confidence,omitempty"` // 0..1
}
