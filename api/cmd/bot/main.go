package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver

	"child-bot/api/internal/config"
	"child-bot/api/internal/httpserver"
	"child-bot/api/internal/ocr"
	"child-bot/api/internal/ocr/deepseek"
	"child-bot/api/internal/ocr/gemini"
	"child-bot/api/internal/ocr/openai"
	"child-bot/api/internal/ocr/yandex"
	"child-bot/api/internal/store"
	"child-bot/api/internal/telegram"
)

func main() {
	cfg := config.Load()

	// --- Postgres ---
	dsn := resolveDSN()
	if dsn == "" {
		log.Fatal("database DSN is empty: set DATABASE_URL or POSTGRES_* env vars")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}
	// connection pool tune (нагрузка до ~20 rps)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(1 * time.Hour)

	// health check
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			log.Fatalf("db.Ping: %v", err)
		}
	}

	parseRepo := store.NewParseRepo(db)
	hintRepo := store.NewHintRepo(db)

	// --- Telegram bot ---
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
	engines := telegram.Engines{
		Yandex:   yandex.New(cfg.YCOAuthToken, cfg.YCFolderID),
		Gemini:   gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel),
		OpenAI:   openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel),
		Deepseek: deepseek.New(cfg.DeepseekAPIKey, cfg.DeepseekModel),
	}

	// Менеджер движков (дефолт — Gemini; тип должен удовлетворять объединённому интерфейсу EngineFull/ocr.Engine)
	manager := ocr.NewManager(engines.Gemini)

	r := &telegram.Router{
		Bot:           bot,
		EngManager:    manager,
		GeminiModel:   cfg.GeminiModel,
		OpenAIModel:   cfg.OpenAIModel,
		DeepseekModel: cfg.DeepseekModel,

		// репозитории для кэша PARSE/подсказок
		ParseRepo: parseRepo,
		HintRepo:  hintRepo,
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

func resolveDSN() string {
	// Prefer DATABASE_URL if provided
	if v := strings.TrimSpace(os.Getenv("DATABASE_URL")); v != "" {
		return v
	}
	// Build DSN from POSTGRES_* / PG* env vars (single-container default)
	user := getenvDefault("POSTGRES_USER", "childbot")
	pass := os.Getenv("POSTGRES_PASSWORD")
	host := getenvDefault("PGHOST", "127.0.0.1")
	port := getenvDefault("PGPORT", "5432")
	db := getenvDefault("POSTGRES_DB", "childbot")

	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, pass),
		Host:     net.JoinHostPort(host, port),
		Path:     "/" + db,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

func getenvDefault(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
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
