package main

import (
	"log"
	"net/http"
	"os"
	"strings"
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

	engines2 := &ocr2.Engines{
		OpenAI: gpt2.New(cfg.OpenAIAPIKey, cfg.OpenAIModel),
		Gemini: gemini2.New(cfg.GeminiAPIKey, cfg.GeminiModel),
	}
	h2 := handle2.New(engines2)

	mux.HandleFunc("/v1/detect", h1.Detect)
	mux.HandleFunc("/v1/parse", h1.Parse)
	mux.HandleFunc("/v1/hint", h1.Hint)
	mux.HandleFunc("/v1/normalize", h1.Normalize)
	mux.HandleFunc("/v1/check_solution", h1.CheckSolution)
	mux.HandleFunc("/v1/analogue_solution", h1.AnalogueSolution)
	mux.HandleFunc("/v1/system_prompt", h1.UpdateSystemPromptHandler)

	mux.HandleFunc("/v2/detect", h2.Detect)
	mux.HandleFunc("/v2/parse", h2.Parse)
	mux.HandleFunc("/v2/hint", h2.Hint)
	mux.HandleFunc("/v2/normalize", h2.Normalize)
	mux.HandleFunc("/v2/check_solution", h2.CheckSolution)
	mux.HandleFunc("/v2/analogue_solution", h2.AnalogueSolution)
	mux.HandleFunc("/v2/system_prompt", h2.UpdateSystemPromptHandler)

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	log.Printf("llm-proxy listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
