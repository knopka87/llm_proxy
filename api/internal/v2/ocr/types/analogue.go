package types

// --- ANALOGUE -------------------------------------------------------
// Генерация аналогичного задания тем же приёмом без утечки ответа исходной задачи.
// Соответствует схемам ANALOGUE_Input.v1.2a и AnalogueSchemas.v1.0a.

// AnalogueSolutionInput — вход генерации аналога (ANALOGUE_Input.v1.2a)
type AnalogueSolutionInput struct {
	Subject             string `json:"subject"`               // "math" | "russian" | "generic"
	Grade               int    `json:"grade"`                 // 1..4
	MethodTag           string `json:"method_tag"`            // приём/метод решения
	OriginalTaskEssence string `json:"original_task_essence"` // суть без чисел и уникальных слов
}

// AnalogyItem — элемент массива analogies (AnalogueSchemas.v1.0a)
type AnalogyItem struct {
	Text    string            `json:"text"`    // ≤ 320 символов (валидируется на уровне схемы)
	Outline string            `json:"outline"` // ≤ 160 символов
	Mapping map[string]string `json:"mapping"` // additionalProperties: string (≤ 80), ограничение проверяется валидатором схем
}

// AnalogueSafety — объект safety
type AnalogueSafety struct {
	RefusalReason string `json:"refusal_reason,omitempty"` // "oversolve_request"
}

// AnalogueSolutionResult — выход ANALOGUE
type AnalogueSolutionResult struct {
	Analogies []AnalogyItem   `json:"analogies"`        // minItems=1, maxItems=1 (валидируется на уровне схемы)
	Safety    *AnalogueSafety `json:"safety,omitempty"` // опционально
}
