package yandex

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"child-bot/api/internal/ocr"
)

type Engine struct {
	iamc     *IamClient
	folderID string
	httpc    *http.Client
}

func New(oauth2Token, folderID string) *Engine {
	return &Engine{
		iamc:     NewIamClient(oauth2Token),
		folderID: folderID,
		httpc:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *Engine) Name() string { return "yandex" }

func (e *Engine) GetModel() string { return "" }

func (e *Engine) Detect(_ context.Context, _ []byte, _ string, _ int) (ocr.DetectResult, error) {
	return ocr.DetectResult{}, fmt.Errorf("Yandex не поддерживает анализ изображений. Используйте /engine gemini | gpt")
}

func (e *Engine) Parse(_ context.Context, _ []byte, _ ocr.ParseOptions) (ocr.ParseResult, error) {
	return ocr.ParseResult{}, fmt.Errorf("Yandex не поддерживает анализ изображений. Используйте /engine gemini | gpt")
}

func (e *Engine) Hint(_ context.Context, _ ocr.HintInput) (ocr.HintResult, error) {
	return ocr.HintResult{}, fmt.Errorf("Yandex не поддерживает анализ изображений. Используйте /engine gemini | gpt")
}
