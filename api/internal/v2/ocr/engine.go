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
	Normalize(ctx context.Context, in types.NormalizeRequest) (types.NormalizeResponse, error)
	CheckSolution(ctx context.Context, in types.CheckRequest) (types.CheckResponse, error)
	AnalogueSolution(ctx context.Context, in types.AnalogueRequest) (types.AnalogueResponse, error)
	OCR(ctx context.Context, request types.OCRRequest) (types.OCRResponse, error)
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
