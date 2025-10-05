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

func (e *Engine) Analyze(_ context.Context, _ []byte, _ ocr.Options) (ocr.Result, error) {
	return ocr.Result{}, fmt.Errorf("DeepSeek Chat API не поддерживает анализ изображений. Используйте /engine yandex | gemini | gpt")
}
