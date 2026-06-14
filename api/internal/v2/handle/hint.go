package handle

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"llm-proxy/api/internal/v2/ocr/types"
)

type HintRequest struct {
	LLMName string `json:"llm_name"`
	types.HintRequest
}

func (h *Handle) Hint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req HintRequest
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

	var out types.HintResponse
	var stats *types.LLMStats

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		http.Error(w, "detect error: "+err.Error(), http.StatusBadGateway)
		return
	}

	out, stats, err = engine.Hint(ctx, req.HintRequest)
	if err != nil {
		http.Error(w, "detect error: "+err.Error(), http.StatusBadGateway)
		return
	}

	if stats != nil {
		w.Header().Set("X-LLM-Input-Tokens", strconv.Itoa(stats.InputTokens))
		w.Header().Set("X-LLM-Output-Tokens", strconv.Itoa(stats.OutputTokens))
		w.Header().Set("X-LLM-Latency-Ms", strconv.FormatInt(stats.LatencyMs, 10))
	}

	writeJSON(w, http.StatusOK, out)
}
