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
	}
}
