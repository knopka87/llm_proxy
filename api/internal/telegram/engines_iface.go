package telegram

import (
	"child-bot/api/internal/ocr"
)

type Engines struct {
	Yandex   ocr.Engine
	Gemini   ocr.Engine
	OpenAI   ocr.Engine
	Deepseek ocr.Engine
}

func (r *Router) pickLLMEngine(chatID int64, engines Engines) ocr.Engine {
	// текущий выбранный через EngManager (если поддерживает Parse/Hint)
	if e, ok := r.EngManager.Get(chatID).(ocr.Engine); ok {
		return e
	}

	if engines.Gemini != nil {
		return engines.Gemini
	}
	if engines.OpenAI != nil {
		return engines.OpenAI
	}
	return nil
}

func (r *Router) resolveEngineByName(name string, engines Engines) ocr.Engine {
	switch name {
	case "gemini":
		return engines.Gemini
	case "gpt":
		return engines.OpenAI
	case "yandex":
		return engines.Yandex
	case "deepseek":
		return engines.Deepseek
	default:
		return nil
	}
}
