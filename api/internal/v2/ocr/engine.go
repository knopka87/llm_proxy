package ocr

import (
	"context"
	"errors"

	"llm-proxy/api/internal/v2/ocr/types"
)

type Engine interface {
	Name() string
	Detect(ctx context.Context, in types.DetectRequest) (types.DetectResponse, error)
	Parse(ctx context.Context, in types.ParseRequest) (types.ParseResponse, error)
	Hint(ctx context.Context, in types.HintRequest) (types.HintResponse, error)
	CheckSolution(ctx context.Context, in types.CheckRequest) (types.CheckResponse, error)
	AnalogueSolution(ctx context.Context, in types.AnalogueRequest) (types.AnalogueResponse, error)
}

type Engines struct {
	OpenAI Engine
}

func (e *Engines) GetEngine(llmName string) (Engine, error) {
	switch llmName {
	case "gpt", "openai":
		return e.OpenAI, nil
	default:
		return nil, errors.New("unknown llm_name; use 'gpt'")
	}
}
