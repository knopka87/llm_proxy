package types

// --- ANALOGUE -------------------------------------------------------
// Генерация аналогичного задания тем же приёмом без утечки ответа исходной задачи.
// Соответствует схемам ANALOGUE_Input.v1.2a и AnalogueSchemas.v1.0a.

// AnalogueRequest — вход запроса (ANALOGUE.request.v1)
type AnalogueRequest struct {
	TaskStruct  TaskStruct     `json:"task_struct"`
	Reason      AnalogueReason `json:"reason"`
	Locale      string         `json:"locale,omitempty"` // "ru-RU" | "en-US"
	RawTaskText string         `json:"raw_task_text"`
	Grade       int64          `json:"grade"` // 1..4
}

// TaskStruct — структура задачи из запроса
type TaskStruct struct {
	Subject           string `json:"subject"`            // "math" | "russian" | "generic"
	Type              string `json:"type,omitempty"`     // произвольная метка, например "arithmetic", "grammar"
	CombinedSubpoints bool   `json:"combined_subpoints"` // по схеме: const=true (валидируется на уровне схемы)
}

// AnalogueReason — допустимые значения поля reason в запросе
type AnalogueReason string

const (
	ReasonAfter3Hints    AnalogueReason = "after_3_hints"
	ReasonAfterIncorrect AnalogueReason = "after_incorrect"
)

// AnalogueResponse — выход (ANALOGUE.response.v1)
type AnalogueResponse struct {
	ExampleTask   string   `json:"example_task"`
	SolutionSteps []string `json:"solution_steps"`
}
