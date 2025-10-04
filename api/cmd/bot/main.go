package main

import (
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"child-bot/api/internal/config"
	"child-bot/api/internal/httpserver"
	"child-bot/api/internal/ocr"
	"child-bot/api/internal/ocr/deepseek"
	"child-bot/api/internal/ocr/gemini"
	"child-bot/api/internal/ocr/openai"
	"child-bot/api/internal/ocr/yandex"
	"child-bot/api/internal/telegram"
)

func main() {
	cfg := config.Load()

	// Telegram bot
	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatal(err)
	}
	bot.Debug = false

	// webhook path
	path := "/webhook/" + shortHash(cfg.TelegramBotToken)
	public := strings.TrimRight(cfg.WebhookURL, "/") + path

	wh, err := tgbotapi.NewWebhook(public)
	if err != nil {
		log.Fatal(err)
	}
	wh.DropPendingUpdates = true

	if _, err := bot.Request(wh); err != nil {
		log.Fatal(err)
	}

	updates := bot.ListenForWebhook(path)

	// Engines
	yandexEng := yandex.New(cfg.YCOAuthToken, cfg.YCFolderID)
	manager := ocr.NewManager(yandexEng)

	engines := telegram.Engines{
		Yandex:   yandexEng,
		Gemini:   gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel),
		OpenAI:   openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel),
		Deepseek: deepseek.New(cfg.DeepseekAPIKey, cfg.DeepseekModel),
	}

	r := &telegram.Router{
		Bot:           bot,
		EngManager:    manager,
		GeminiModel:   cfg.GeminiModel,
		OpenAIModel:   cfg.OpenAIModel,
		DeepseekModel: cfg.DeepseekModel,
	}

	// Process updates
	go func() {
		for upd := range updates {
			r.HandleUpdate(upd, engines)
		}
	}()

	// HTTP server (healthz)
	addr := "0.0.0.0:" + cfg.Port
	if err := httpserver.StartHTTP(addr, "ok"); err != nil {
		log.Fatal(err)
	}
}

func shortHash(s string) string {
	// лёгкий хэш для пути вебхука
	h := uint64(1469598103934665603)
	const prime = 1099511628211
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	// 16-символный hex
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		out[i] = hexdigits[h&0xF]
		h >>= 4
	}
	return string(out)
}
