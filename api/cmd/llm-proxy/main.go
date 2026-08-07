package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"llm-proxy/api/internal/config"
	handle1 "llm-proxy/api/internal/v1/handle"
	ocr1 "llm-proxy/api/internal/v1/ocr"
	gemini1 "llm-proxy/api/internal/v1/ocr/gemini"
	gpt1 "llm-proxy/api/internal/v1/ocr/gpt"
	handle2 "llm-proxy/api/internal/v2/handle"
	ocr2 "llm-proxy/api/internal/v2/ocr"
	gemini2 "llm-proxy/api/internal/v2/ocr/gemini"
	gpt2 "llm-proxy/api/internal/v2/ocr/gpt"
	mixed2 "llm-proxy/api/internal/v2/ocr/mixed"
	or2 "llm-proxy/api/internal/v2/ocr/openrouter"
	"llm-proxy/api/internal/v2/tmplrouter"
)

func main() {
	cfg := config.Load()

	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		cfg.Port = p
	} else if strings.TrimSpace(cfg.Port) == "" {
		cfg.Port = "8000"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	engines1 := &ocr1.Engines{
		OpenAI: gpt1.New(cfg.OpenAIAPIKey, cfg.OpenAIModel),
		Gemini: gemini1.New(cfg.GeminiAPIKey, cfg.GeminiModel),
	}
	h1 := handle1.New(engines1)

	gptV2 := gpt2.New(cfg.OpenAIAPIKey, cfg.OpenAIModel)
	geminiV2 := gemini2.New(cfg.GeminiAPIKey, cfg.GeminiDetectModel, cfg.GeminiParseModel)
	mixedV2 := mixed2.New(geminiV2, gptV2)

	// OpenRouter инициализируется только если задан ключ.
	// Модели для каждого шага берутся из env-переменных — без хардкода в коде.
	var openRouterV2 ocr2.Engine
	if cfg.OpenRouterAPIKey != "" {
		openRouterV2 = or2.New(cfg.OpenRouterAPIKey, or2.StepModels{
			Detect:   cfg.OpenRouterDetectModel,
			Parse:    cfg.OpenRouterParseModel,
			Hint:     cfg.OpenRouterHintModel,
			Check:    cfg.OpenRouterCheckModel,
			Analogue: cfg.OpenRouterAnalogueModel,
		})
		log.Printf("OpenRouter engine initialized (detect=%s parse=%s hint=%s check=%s)",
			cfg.OpenRouterDetectModel, cfg.OpenRouterParseModel,
			cfg.OpenRouterHintModel, cfg.OpenRouterCheckModel)
	}

	// Load pedagogical templates once at startup.
	// All math hint engines share the same router instance.
	tmplRouter := tmplrouter.New()
	if tmplRouter.Len() == 0 {
		log.Fatal("no valid pedagogical templates loaded")
	}
	gptV2.SetTemplateRouter(tmplRouter)
	if orEngine, ok := openRouterV2.(interface{ SetTemplateRouter(*tmplrouter.Router) }); ok {
		orEngine.SetTemplateRouter(tmplRouter)
	}

	engines2 := &ocr2.Engines{
		OpenAI:         gptV2,
		Gemini:         geminiV2,
		Mixed:          mixedV2,
		OpenRouter:     openRouterV2,
		TemplateRouter: tmplRouter,
	}
	h2 := handle2.New(engines2)

	mux.HandleFunc("/v1/detect", h1.Detect)
	mux.HandleFunc("/v1/parse", h1.Parse)
	mux.HandleFunc("/v1/hint", h1.Hint)
	mux.HandleFunc("/v1/ocr", h1.Ocr)
	mux.HandleFunc("/v1/normalize", h1.Normalize)
	mux.HandleFunc("/v1/check_solution", h1.CheckSolution)
	mux.HandleFunc("/v1/analogue_solution", h1.AnalogueSolution)

	mux.HandleFunc("/v2/detect", h2.Detect)
	mux.HandleFunc("/v2/parse", h2.Parse)
	mux.HandleFunc("/v2/hint", h2.Hint)
	mux.HandleFunc("/v2/check_solution", h2.CheckSolution)
	mux.HandleFunc("/v2/analogue_solution", h2.AnalogueSolution)

	mux.HandleFunc("/v2/parse_ru", h2.ParseRU)
	mux.HandleFunc("/v2/hint_ru", h2.HintRU)
	mux.HandleFunc("/v2/check_ru", h2.CheckRU)

	mux.HandleFunc("/v2/embed", h2.Embed)
	clientIPFilter, err := newClientIPFilter(cfg.AllowedClientCIDRs, cfg.TrustedProxyCIDRs)
	if err != nil {
		log.Fatalf("invalid client IP allowlist: %v", err)
	}

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           clientIPFilter.Middleware(serviceAuth(cfg.APIKey, mux)),
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5*time.Minute + 15*time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	log.Printf("llm-proxy listening on %s", addr)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("llm-proxy server failed: %v", err)
		}
	case sig := <-signals:
		log.Printf("received signal %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("llm-proxy graceful shutdown failed: %v", err)
		}
	}
}

func serviceAuth(apiKey string, next http.Handler) http.Handler {
	apiKey = strings.TrimSpace(apiKey)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		authorization := r.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		provided := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
