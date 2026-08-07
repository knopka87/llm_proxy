package types

// LLMStats содержит реальные метрики одного LLM-вызова,
// полученные непосредственно из ответа API.
type LLMStats struct {
	InputTokens  int     // реальные входные токены от API
	OutputTokens int     // реальные выходные токены от API
	LatencyMs    int64   // время от отправки запроса до получения ответа
	Model        string  // конкретная модель, обработавшая запрос
	PromptHash   string  // короткий SHA-256 фактически отправленного system prompt
	PromptBlocks string  // подключённые динамические блоки через запятую
	CostUSD      float64 // provider-reported cost, если доступен
}

// Add добавляет метрики другого вызова к накопленным.
func (s *LLMStats) Add(other *LLMStats) {
	if other == nil {
		return
	}
	s.InputTokens += other.InputTokens
	s.OutputTokens += other.OutputTokens
	s.LatencyMs += other.LatencyMs
	s.CostUSD += other.CostUSD
	if s.PromptHash == "" {
		s.PromptHash = other.PromptHash
	}
	if s.PromptBlocks == "" {
		s.PromptBlocks = other.PromptBlocks
	}
}
