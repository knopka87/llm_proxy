package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"llm-proxy/api/internal/config"
	handle "llm-proxy/api/internal/handle"
	"llm-proxy/api/internal/ocr"
	"llm-proxy/api/internal/ocr/gemini"
	"llm-proxy/api/internal/ocr/openai"
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
		w.Write([]byte("ok"))
	})

	engines := &ocr.Engines{
		OpenAI: openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel),
		Gemini: gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel),
	}
	h := handle.New(engines)

	mux.HandleFunc("/v1/detect", h.Detect)
	mux.HandleFunc("/v1/parse", h.Parse)
	mux.HandleFunc("/v1/hint", h.Hint)
	mux.HandleFunc("/v1/normalize", h.Normalize)
	mux.HandleFunc("/v1/check_solution", h.CheckSolution)
	mux.HandleFunc("/v1/analogue_solution", h.AnalogueSolution)

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
