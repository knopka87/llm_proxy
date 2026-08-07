package config

import (
	"os"
)

type Config struct {
	Port               string
	APIKey             string
	AllowedClientCIDRs string
	TrustedProxyCIDRs  string

	GeminiAPIKey      string
	GeminiModel       string // используется v1
	GeminiDetectModel string // v2: detect (gemini-2.0-flash-lite)
	GeminiParseModel  string // v2: parse  (gemini-2.5-flash)
	OpenAIAPIKey      string
	OpenAIModel       string

	// OpenRouter — единый API для 300+ моделей.
	// Модели задаются отдельно для каждого шага; ни одна не захардкожена.
	// Если ключ не задан — движок "openrouter" недоступен.
	OpenRouterAPIKey        string // OPENROUTER_API_KEY
	OpenRouterDetectModel   string // OPENROUTER_DETECT_MODEL
	OpenRouterParseModel    string // OPENROUTER_PARSE_MODEL
	OpenRouterHintModel     string // OPENROUTER_HINT_MODEL
	OpenRouterCheckModel    string // OPENROUTER_CHECK_MODEL
	OpenRouterAnalogueModel string // OPENROUTER_ANALOGUE_MODEL
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() *Config {
	return &Config{
		Port:               getEnv("PORT", "8000"),
		APIKey:             getEnv("LLM_PROXY_API_KEY", ""),
		AllowedClientCIDRs: getEnv("LLM_PROXY_ALLOWED_CLIENT_CIDRS", ""),
		TrustedProxyCIDRs:  getEnv("LLM_PROXY_TRUSTED_PROXY_CIDRS", ""),

		GeminiAPIKey:      getEnv("GEMINI_API_KEY", ""),
		GeminiModel:       getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiDetectModel: getEnv("GEMINI_DETECT_MODEL", "gemini-2.0-flash-lite"),
		GeminiParseModel:  getEnv("GEMINI_PARSE_MODEL", "gemini-2.5-flash"),
		OpenAIAPIKey:      getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:       getEnv("OPENAI_MODEL", "gpt-4o-mini"),

		// OpenRouter необязателен: если ключ не задан, движок просто недоступен.
		OpenRouterAPIKey:        getEnv("OPENROUTER_API_KEY", ""),
		OpenRouterDetectModel:   getEnv("OPENROUTER_DETECT_MODEL", "google/gemini-2.5-flash-lite"),
		OpenRouterParseModel:    getEnv("OPENROUTER_PARSE_MODEL", "openai/gpt-4.1-mini"),
		OpenRouterHintModel:     getEnv("OPENROUTER_HINT_MODEL", "google/gemini-2.5-flash"),
		OpenRouterCheckModel:    getEnv("OPENROUTER_CHECK_MODEL", "openai/gpt-4.1-mini"),
		OpenRouterAnalogueModel: getEnv("OPENROUTER_ANALOGUE_MODEL", ""),
	}
}
