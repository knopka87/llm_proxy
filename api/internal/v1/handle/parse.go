package handle

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"llm-proxy/api/internal/v1/ocr/types"
)

type ParseRequest struct {
	LLMName string `json:"llm_name"`
	types.ParseInput
}

func (h *Handle) Parse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req ParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	img, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.ImageB64))
	if err != nil || len(img) == 0 {
		http.Error(w, "bad image_b64", http.StatusBadRequest)
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

	var out types.ParseResult

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		http.Error(w, "parse error: "+err.Error(), http.StatusBadGateway)
		return
	}

	out, err = engine.Parse(ctx, req.ParseInput)
	if err != nil {
		http.Error(w, "parse error: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, out)
}
