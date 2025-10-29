package ocr

import (
	"context"
	"errors"

	"llm-proxy/api/internal/v2/ocr/types"
)

type Engine interface {
	Name() string
	Detect(ctx context.Context, in types.DetectInput) (types.DetectResult, error)
	Parse(ctx context.Context, in types.ParseInput) (types.ParseResult, error)
	Hint(ctx context.Context, in types.HintInput) (types.HintResult, error)
	Normalize(ctx context.Context, in types.NormalizeInput) (types.NormalizeResult, error)
	CheckSolution(ctx context.Context, in types.CheckSolutionInput) (types.CheckSolutionResult, error)
	AnalogueSolution(ctx context.Context, in types.AnalogueSolutionInput) (types.AnalogueSolutionResult, error)
}

type Engines struct {
	Gemini Engine
	OpenAI Engine
}

func (e *Engines) GetEngine(llmName string) (Engine, error) {
	switch llmName {
	case "gemini", "google":
		return e.Gemini, nil
	case "gpt", "openai":
		return e.OpenAI, nil
	default:
		return nil, errors.New("unknown llm_name; use 'gemini' or 'gpt'")
	}
}
