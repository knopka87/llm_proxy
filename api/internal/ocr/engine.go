package ocr

import (
	"context"
	"errors"
)

type Engine interface {
	Name() string
	Detect(ctx context.Context, img []byte, mime string, gradeHint int) (DetectResult, error)
	Parse(ctx context.Context, image []byte, options ParseOptions) (ParseResult, error)
	Hint(ctx context.Context, in HintInput) (HintResult, error)
	Normalize(ctx context.Context, in NormalizeInput) (NormalizeResult, error)
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
