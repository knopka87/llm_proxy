package ocr

import (
	"context"
	"errors"

	"llm-proxy/api/internal/v2/ocr/types"
)

type Engine interface {
	Name() string
	Detect(ctx context.Context, in types.DetectRequest) (types.DetectResponse, *types.LLMStats, error)
	Parse(ctx context.Context, in types.ParseRequest) (types.ParseResponse, *types.LLMStats, error)
	Hint(ctx context.Context, in types.HintRequest) (types.HintResponse, *types.LLMStats, error)
	CheckSolution(ctx context.Context, in types.CheckRequest) (types.CheckResponse, *types.LLMStats, error)
	AnalogueSolution(ctx context.Context, in types.AnalogueRequest) (types.AnalogueResponse, *types.LLMStats, error)
}

type Engines struct {
	OpenAI Engine // OpenAI GPT — hint, check, analogue
	Gemini Engine // Gemini    — detect, parse
	Mixed  Engine // detect+parse→Gemini, hint+check→OpenAI
}

// GetEngine возвращает движок по llm_name из запроса:
//   - "gpt" / "openai" → чистый OpenAI (все шаги через GPT)
//   - "gemini"         → чистый Gemini (все шаги через Gemini)
//   - "mixed"          → detect+parse через Gemini, hint+check через OpenAI
func (e *Engines) GetEngine(llmName string) (Engine, error) {
	switch llmName {
	case "gpt", "openai":
		if e.OpenAI == nil {
			return nil, errors.New("OpenAI engine not initialized")
		}
		return e.OpenAI, nil
	case "gemini":
		if e.Gemini == nil {
			return nil, errors.New("Gemini engine not initialized")
		}
		return e.Gemini, nil
	case "mixed":
		if e.Mixed == nil {
			return nil, errors.New("Mixed engine not initialized")
		}
		return e.Mixed, nil
	default:
		return nil, errors.New("unknown llm_name; use 'gpt', 'gemini' or 'mixed'")
	}
}