package deepseek

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"child-bot/api/internal/ocr"
)

type Engine struct {
	APIKey string
	Model  string
	httpc  *http.Client
}

func New(key, model string) *Engine {
	return &Engine{
		APIKey: key,
		Model:  model,
		httpc:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *Engine) Name() string { return "deepseek" }

func (e *Engine) GetModel() string { return e.Model }

func (e *Engine) Detect(_ context.Context, _ []byte, _ string, _ int) (ocr.DetectResult, error) {
	return ocr.DetectResult{}, fmt.Errorf("DeepSeek Chat API не поддерживает анализ изображений. Используйте /engine yandex | gemini | gpt")
}

func (e *Engine) Parse(_ context.Context, _ []byte, _ ocr.ParseOptions) (ocr.ParseResult, error) {
	return ocr.ParseResult{}, fmt.Errorf("DeepSeek Chat API не поддерживает анализ изображений. Используйте /engine yandex | gemini | gpt")
}

func (e *Engine) Hint(_ context.Context, _ ocr.HintInput) (ocr.HintResult, error) {
	return ocr.HintResult{}, fmt.Errorf("DeepSeek Chat API не поддерживает анализ изображений. Используйте /engine gemini | gpt")
}
