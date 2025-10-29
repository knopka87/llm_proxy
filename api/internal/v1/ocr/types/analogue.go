package types

import "encoding/json"

// --- ANALOGUE SOLUTION ----------------------------------------------
// Даёт разбор похожего задания тем же приёмом без утечки ответа исходной задачи.

// AnalogueSolutionInput — вход генерации аналога
// Важно: original_task_essence не должен содержать числа/слова из исходной задачи.
type AnalogueSolutionInput struct {
	TaskID              string `json:"task_id,omitempty"`
	UserIDAnon          string `json:"user_id_anon,omitempty"`
	Grade               int    `json:"grade,omitempty"`
	Subject             string `json:"subject,omitempty"` // math|russian|...
	TaskType            string `json:"task_type,omitempty"`
	MethodTag           string `json:"method_tag,omitempty"` // тот же приём решения
	DifficultyHint      string `json:"difficulty_hint,omitempty"`
	OriginalTaskEssence string `json:"original_task_essence"` // краткая суть без исходных чисел/слов
	Locale              string `json:"locale,omitempty"`      // ru (по умолчанию)
}

// AnalogueSolutionResult — строгий JSON по analogue.schema.json v1.1
// Не должен повторять исходные данные и не раскрывает правильный ответ оригинала.
type AnalogueSolutionResult struct {
	AnalogyTitle  string   `json:"analogy_title"`
	AnalogyTask   string   `json:"analogy_task"`
	SolutionSteps []string `json:"solution_steps"` // 3–4 шага, короткие предложения

	// Мини‑проверки: структурные (yn/single_word/choice). Поддержан и старый строковый формат.
	MiniChecks []MiniCheckItem `json:"mini_checks"`

	TransferBridge string `json:"transfer_bridge"`          // 2–3 шага переноса
	TransferCheck  string `json:"transfer_check,omitempty"` // 1 вопрос для самопроверки переноса

	// Безопасность/антиликовая защита
	Safety          AnalogueSafety `json:"safety"`
	LeakGuardPassed bool           `json:"leak_guard_passed,omitempty"`
}

// MiniCheckItem — поддерживает как структурный формат, так и старый строковый.
// ВАЖНО: поле Raw используется только для приёма строкового старого формата и наружу не сериализуется.
type MiniCheckItem struct {
	Prompt string `json:"prompt,omitempty"`
	Raw    string `json:"-"` // если пришла строка
}

func (m *MiniCheckItem) UnmarshalJSON(b []byte) error {
	// Строковый старый формат
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		m.Raw = s
		return nil
	}
	// Новый объектный формат
	type _mini MiniCheckItem
	var v _mini
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*m = MiniCheckItem(v)
	return nil
}

// MistakeItem — типовая ошибка: код + сообщение; поддерживает старый строковый формат.
// Поле Raw скрыто наружу, чтобы соответствовать schema (additionalProperties=false).
type MistakeItem struct {
	Message string `json:"message,omitempty"`
	Raw     string `json:"-"` // если пришла строка
}

func (m *MistakeItem) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		m.Raw = s
		m.Message = s
		return nil
	}
	type _mist MistakeItem
	var v _mist
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*m = MistakeItem(v)
	return nil
}

// AnalogueSafety — базовые флаги безопасности
// AnalogueSafety — базовые флаги безопасности
type AnalogueSafety struct {
	NoOriginalAnswerLeak bool `json:"no_original_answer_leak"`
}
