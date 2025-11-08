package handle

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"llm-proxy/api/internal/v1/ocr/types"
)

type OcrRequest struct {
	LLMName string `json:"llm_name"`
	types.OCRRequest
}

func (h *Handle) Ocr(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req OcrRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	deadline := 180 * time.Second
	if ts := r.Header.Get("X-Request-Timeout"); ts != "" {
		if v, _ := strconv.Atoi(ts); v > 0 {
			deadline = time.Duration(v) * time.Second
		}
	} else if ts := r.URL.Query().Get("timeoutSec"); ts != "" {
		if v, _ := strconv.Atoi(ts); v > 0 {
			deadline = time.Duration(v) * time.Second
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), deadline)
	defer cancel()

	var out types.OCRResponse

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		http.Error(w, "Ocr error: "+err.Error(), http.StatusBadGateway)
		return
	}

	out, err = engine.OCR(ctx, req.OCRRequest)
	if err != nil {
		http.Error(w, "Ocr error: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, out)
}
