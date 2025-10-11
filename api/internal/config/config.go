package config

import (
	"log"
	"os"
)

type Config struct {
	Port string

	GeminiAPIKey string
	GeminiModel  string
	OpenAIAPIKey string
	OpenAIModel  string
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

		GeminiAPIKey: mustEnv("GEMINI_API_KEY"),
		GeminiModel:  getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		OpenAIAPIKey: mustEnv("OPENAI_API_KEY"),
		OpenAIModel:  getEnv("OPENAI_MODEL", "gpt-4o-mini"),
	}
}
