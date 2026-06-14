package config

import (
	"log"
	"os"
)

type Config struct {
	Port string

	GeminiAPIKey      string
	GeminiModel       string // используется v1
	GeminiDetectModel string // v2: detect (gemini-2.0-flash-lite)
	GeminiParseModel  string // v2: parse  (gemini-2.5-flash)
	OpenAIAPIKey      string
	OpenAIModel       string

	// OpenRouter — единый API для 300+ моделей.
	// Модели задаются отдельно для каждого шага; ни одна не захардкожена.
	// Если ключ не задан — движок "openrouter" недоступен.
	OpenRouterAPIKey      string // OPENROUTER_API_KEY
	OpenRouterDetectModel string // OPENROUTER_DETECT_MODEL
	OpenRouterParseModel  string // OPENROUTER_PARSE_MODEL
	OpenRouterHintModel   string // OPENROUTER_HINT_MODEL
	OpenRouterCheckModel  string // OPENROUTER_CHECK_MODEL
	OpenRouterAnalogueModel string // OPENROUTER_ANALOGUE_MODEL
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing required env %s", k)
	}
	return v
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() *Config {
	return &Config{
		Port: getEnv("PORT", "8000"),

		GeminiAPIKey:      mustEnv("GEMINI_API_KEY"),
		GeminiModel:       getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiDetectModel: getEnv("GEMINI_DETECT_MODEL", "gemini-2.0-flash-lite"),
		GeminiParseModel:  getEnv("GEMINI_PARSE_MODEL", "gemini-2.5-flash"),
		OpenAIAPIKey:      mustEnv("OPENAI_API_KEY"),
		OpenAIModel:       getEnv("OPENAI_MODEL", "gpt-4o-mini"),

		// OpenRouter необязателен: если ключ не задан, движок просто недоступен.
		OpenRouterAPIKey:        getEnv("OPENROUTER_API_KEY", ""),
		OpenRouterDetectModel:   getEnv("OPENROUTER_DETECT_MODEL", ""),
		OpenRouterParseModel:    getEnv("OPENROUTER_PARSE_MODEL", ""),
		OpenRouterHintModel:     getEnv("OPENROUTER_HINT_MODEL", ""),
		OpenRouterCheckModel:    getEnv("OPENROUTER_CHECK_MODEL", ""),
		OpenRouterAnalogueModel: getEnv("OPENROUTER_ANALOGUE_MODEL", ""),
	}
}
