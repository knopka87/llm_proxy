package handle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"llm-proxy/api/internal/v2/ocr"
	"llm-proxy/api/internal/v2/ocr/types"
	"llm-proxy/api/internal/v2/tmplrouter"
)

const (
	// maxBodySize — максимальный размер входящего запроса (10 MB).
	// Запросы содержат base64-изображения, которые могут быть большими.
	maxBodySize = 10 << 20

	// maxTimeout — верхний лимит клиентского timeout (5 минут).
	// Защита от Slow Loris: клиент не может удерживать соединение дольше.
	maxTimeout = 5 * time.Minute

	// defaultTimeout — timeout по умолчанию если клиент не указал.
	defaultTimeout = 180 * time.Second
)

type Handle struct {
	engs *ocr.Engines
}

func New(engs *ocr.Engines) *Handle {
	return &Handle{
		engs: engs,
	}
}

// templateRouter returns the shared Router (may be nil if templates not loaded).
func (h *Handle) templateRouter() *tmplrouter.Router {
	return h.engs.TemplateRouter
}

// writeStatsHeaders пишет метрики LLM-вызова в заголовки ответа.
// X-LLM-Model позволяет child_bot логировать, какая именно модель
// обработала запрос, и сохранять это в llm_steps_json.
func writeStatsHeaders(w http.ResponseWriter, stats *types.LLMStats) {
	if stats == nil {
		return
	}
	w.Header().Set("X-LLM-Input-Tokens", strconv.Itoa(stats.InputTokens))
	w.Header().Set("X-LLM-Output-Tokens", strconv.Itoa(stats.OutputTokens))
	w.Header().Set("X-LLM-Latency-Ms", strconv.FormatInt(stats.LatencyMs, 10))
	if stats.Model != "" {
		w.Header().Set("X-LLM-Model", stats.Model)
	}
	if stats.PromptHash != "" {
		w.Header().Set("X-LLM-Prompt-Hash", stats.PromptHash)
	}
	if stats.PromptBlocks != "" {
		w.Header().Set("X-LLM-Prompt-Blocks", stats.PromptBlocks)
	}
	if stats.CostUSD > 0 {
		w.Header().Set("X-LLM-Cost-USD", strconv.FormatFloat(stats.CostUSD, 'f', 9, 64))
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[http] encode response: %v", err)
	}
}

// parseDeadline извлекает timeout из запроса с верхним лимитом maxTimeout.
func parseDeadline(r *http.Request) time.Duration {
	deadline := defaultTimeout
	if ts := r.Header.Get("X-Request-Timeout"); ts != "" {
		if v, _ := strconv.Atoi(ts); v > 0 {
			deadline = time.Duration(v) * time.Second
		}
	} else if ts := r.URL.Query().Get("timeoutSec"); ts != "" {
		if v, _ := strconv.Atoi(ts); v > 0 {
			deadline = time.Duration(v) * time.Second
		}
	}
	if deadline > maxTimeout {
		deadline = maxTimeout
	}
	return deadline
}

// limitBodyReader оборачивает r.Body лимитом размера.
// Должен вызываться ДО json.NewDecoder(r.Body).Decode().
func limitBodyReader(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
}

// readAndLimitBody читает тело запроса с лимитом и декодирует JSON.
// Возвращает ошибку, если тело превышает maxBodySize или невалидный JSON.
func readAndLimitBody(w http.ResponseWriter, r *http.Request, dst any) error {
	limitBodyReader(w, r)
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize+1))
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if len(body) > maxBodySize {
		return fmt.Errorf("request body exceeds %d bytes", maxBodySize)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain exactly one JSON value")
	}
	return nil
}
