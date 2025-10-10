package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver

	"child-bot/api/internal/config"
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

	// Prefer platform PORT env var; fallback to cfg.Port; then to 8000
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		cfg.Port = p
	} else if strings.TrimSpace(cfg.Port) == "" {
		cfg.Port = "8080"
	}

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
		log.Printf("db connected: %s", safeDSNSummary(dsn))
	}

	parseRepo := store.NewParseRepo(db)
	hintRepo := store.NewHintRepo(db)

	// --- Telegram bot ---
	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatal(err)
	}
	bot.Debug = false

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

	// --- HTTP mux (DefaultServeMux) ---
	// Используем DefaultServeMux, чтобы ListenForWebhook, который регистрирует обработчик на default mux, работал корректно.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("db: not ok\n" + err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := "0.0.0.0:" + cfg.Port

	// --- Choose mode: Webhook vs Polling ---
	webhookURL := strings.TrimSpace(cfg.WebhookURL)
	if webhookURL != "" {
		startWebhookMode(addr, bot, r, webhookURL, engines)
	} else {
		startPollingMode(addr, bot, r)
	}
}

// ---------------- Modes -----------------

func startWebhookMode(addr string, bot *tgbotapi.BotAPI, r *telegram.Router, baseURL string, engines telegram.Engines) {
	// секретный путь вебхука
	path := "/webhook/" + shortHash(r.Bot.Token)
	public := strings.TrimRight(baseURL, "/") + path

	wh, err := tgbotapi.NewWebhook(public)
	if err != nil {
		log.Fatal(err)
	}
	wh.DropPendingUpdates = true
	if _, err := bot.Request(wh); err != nil {
		log.Fatal(err)
	}

	// tgbotapi.ListenForWebhook регистрирует обработчик на DefaultServeMux
	updates := bot.ListenForWebhook(path)

	go func() {
		for upd := range updates {
			r.HandleUpdate(upd, engines)
		}
		log.Printf("webhook updates channel closed")
	}()

	log.Printf("health server listening on %s/healthz", addr)
	log.Printf("webhook listening on %s%s", addr, path)
	if err := http.ListenAndServe(addr, nil); err != nil { // DefaultServeMux
		log.Fatal(err)
	}
}

func startPollingMode(addr string, bot *tgbotapi.BotAPI, r *telegram.Router) {
	// Запускаем HTTP server (healthz), хотя для polling он не обязателен
	go func() {
		log.Printf("health server listening on %s/healthz", addr)
		if err := http.ListenAndServe(addr, nil); err != nil { // DefaultServeMux
			log.Fatal(err)
		}
	}()

	// Устойчивый поллинг с backoff без log.Fatal/os.Exit
	ctx := context.Background()
	runPolling(ctx, bot, func(upd tgbotapi.Update) {
		r.HandleUpdate(upd, telegram.Engines{}) // engines менеджер внутри роутера
	})
}

// ---------------- Polling loop -----------------

var reRetryAfter = regexp.MustCompile(`(?i)retry after\s+(\d+)`)

func retryDelayFromError(err error) time.Duration {
	if err == nil {
		return 0
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "too many requests") { // HTTP 429 от Telegram
		if m := reRetryAfter.FindStringSubmatch(s); len(m) == 2 {
			if n, _ := strconv.Atoi(m[1]); n > 0 {
				return time.Duration(n) * time.Second
			}
		}
		return 3 * time.Second
	}
	var ne net.Error
	if errors.As(err, &ne) {
		if ne.Timeout() {
			return 2 * time.Second
		}
	}
	return 1 * time.Second
}

func runPolling(ctx context.Context, bot *tgbotapi.BotAPI, handle func(tgbotapi.Update)) {
	offset := 0
	baseDelay := 1 * time.Second
	maxDelay := 15 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Printf("polling: context cancelled")
			return
		default:
		}

		u := tgbotapi.NewUpdate(offset)
		u.Timeout = 30 // long polling timeout (sec)

		updates, err := bot.GetUpdates(u)
		if err != nil {
			d := retryDelayFromError(err)
			if d < baseDelay {
				d = baseDelay
			}
			if d > maxDelay {
				d = maxDelay
			}
			log.Printf("polling error: %v; retry in %v", err, d)
			time.Sleep(d)
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			handle(upd)
		}

		if len(updates) == 0 {
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// ---------------- Helpers -----------------

func resolveDSN() string {
	// Prefer DATABASE_URL if provided
	if v := strings.TrimSpace(os.Getenv("DATABASE_URL")); v != "" {
		return v
	}
	// Build DSN from POSTGRES_* / PG* env vars (single-container default)
	user := getenvDefault("POSTGRES_USER", "childbot")
	pass := os.Getenv("POSTGRES_PASSWORD")
	host := getenvDefault("PGHOST", "db")
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
	// лёгкий хэш для пути вебхука (не крипто, но стабильно для токена)
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

func safeDSNSummary(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "dsn: parse error"
	}
	user := u.User.Username()
	host := u.Host
	port := ""
	if h, p, err := net.SplitHostPort(u.Host); err == nil {
		host, port = h, p
	}
	db := strings.TrimPrefix(u.Path, "/")
	if port == "" {
		return fmt.Sprintf("host=%s db=%s user=%s", host, db, user)
	}
	return fmt.Sprintf("host=%s port=%s db=%s user=%s", host, port, db, user)
}
