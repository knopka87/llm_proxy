package ocr

// Result — единый результат для всех движков.
// Для Yandex используются только Text (и, опционально, FoundTask).
type Result struct {
	// Всегда полезно иметь сырой текст, если удаётся его достать
	Text string

	// Детекция сути задачи
	FoundTask     bool
	FoundSolution bool

	// Если FoundSolution == true
	SolutionVerdict string // "correct" | "incorrect" | "uncertain"
	SolutionNote    string // краткое пояснение "где/какого рода" ошибка (без решения)

	// Подсказки L1→L3: от лёгкой наводки до подробного плана решения,
	// но без самого ответа/итогового вычисления.
	Hints []string // len=0 или 3 (предпочтительно)
}
