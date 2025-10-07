package config

import (
	"log"
	"os"
)

type Config struct {
	Port       string
	WebhookURL string

	TelegramBotToken string

	// Yandex OCR
	YCOAuthToken string
	YCFolderID   string

	// Alt engines
	GeminiAPIKey   string
	GeminiModel    string
	GeminiBase     string
	OpenAIAPIKey   string
	OpenAIModel    string
	DeepseekAPIKey string
	DeepseekModel  string
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing required env %s", k)
	}
	return v
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() *Config {
	return &Config{
		Port:             getenv("PORT", "8080"),
		WebhookURL:       mustEnv("WEBHOOK_URL"),
		TelegramBotToken: mustEnv("TELEGRAM_BOT_TOKEN"),

		YCOAuthToken: mustEnv("YC_OAUTH_TOKEN"),
		YCFolderID:   mustEnv("YC_FOLDER_ID"),

		GeminiAPIKey:   os.Getenv("GEMINI_API_KEY"),
		GeminiModel:    getenv("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiBase:     getenv("GEMINI_BASE", "https://generativelanguage.googleapis.com/v1"),
		OpenAIAPIKey:   os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:    getenv("OPENAI_MODEL", "gpt-4o-mini"),
		DeepseekAPIKey: os.Getenv("DEEPSEEK_API_KEY"),
		DeepseekModel:  getenv("DEEPSEEK_MODEL", "deepseek-vl"),
	}
}
