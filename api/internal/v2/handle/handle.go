package handle

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"llm-proxy/api/internal/v2/ocr"
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
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
	// io.LimitReader как дополнительная страховка
	limited := io.LimitReader(r.Body, maxBodySize+1)
	return json.NewDecoder(limited).Decode(dst)
}
