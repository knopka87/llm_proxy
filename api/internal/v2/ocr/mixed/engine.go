// Package mixed предоставляет Engine, который роутит каждый шаг к оптимальному провайдеру:
//   - Detect → Gemini (gemini-2.0-flash-lite: дешевле, быстрее, задача простая)
//   - Parse  → Gemini (gemini-2.5-flash: лучший OCR рукописи + русский язык)
//   - Hint, CheckSolution, AnalogueSolution → OpenAI (лучший педагогический текст на русском)
package mixed

import (
	"context"

	"llm-proxy/api/internal/v2/ocr"
	"llm-proxy/api/internal/v2/ocr/types"
)

type Engine struct {
	gemini ocr.Engine // detect + parse
	openai ocr.Engine // hint + check + analogue
}

func New(gemini, openai ocr.Engine) *Engine {
	return &Engine{gemini: gemini, openai: openai}
}

func (e *Engine) Name() string { return "mixed" }

func (e *Engine) Detect(ctx context.Context, in types.DetectRequest) (types.DetectResponse, error) {
	return e.gemini.Detect(ctx, in)
}

func (e *Engine) Parse(ctx context.Context, in types.ParseRequest) (types.ParseResponse, error) {
	return e.gemini.Parse(ctx, in)
}

func (e *Engine) Hint(ctx context.Context, in types.HintRequest) (types.HintResponse, error) {
	return e.openai.Hint(ctx, in)
}

func (e *Engine) CheckSolution(ctx context.Context, in types.CheckRequest) (types.CheckResponse, error) {
	return e.openai.CheckSolution(ctx, in)
}

func (e *Engine) AnalogueSolution(ctx context.Context, in types.AnalogueRequest) (types.AnalogueResponse, error) {
	return e.openai.AnalogueSolution(ctx, in)
}