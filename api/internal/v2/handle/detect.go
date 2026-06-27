package handle

import (
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"strconv"
	"strings"

	"llm-proxy/api/internal/v2/ocr/types"
)

type DetectRequest struct {
	LLMName string `json:"llm_name"`
	types.DetectRequest
}

func (h *Handle) Detect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	var req DetectRequest
	if err := readAndLimitBody(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}

	req.Image = stripDataURL(req.Image)
	img, err := base64.StdEncoding.DecodeString(req.Image)
	if err != nil || len(img) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad image_b64"})
		return
	}

	deadline := parseDeadline(r)
	ctx, cancel := context.WithTimeout(r.Context(), deadline)
	defer cancel()

	var out types.DetectResponse
	var stats *types.LLMStats

	log.Printf("[detect] llm_name=%q", req.LLMName)
	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		log.Printf("[detect] engine error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "engine not available"})
		return
	}

	out, stats, err = engine.Detect(ctx, req.DetectRequest)
	if err != nil {
		log.Printf("[detect] LLM error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "detect processing failed"})
		return
	}

	if stats != nil {
		w.Header().Set("X-LLM-Input-Tokens", strconv.Itoa(stats.InputTokens))
		w.Header().Set("X-LLM-Output-Tokens", strconv.Itoa(stats.OutputTokens))
		w.Header().Set("X-LLM-Latency-Ms", strconv.FormatInt(stats.LatencyMs, 10))
	}

	writeJSON(w, http.StatusOK, out)
}

func stripDataURL(b64 string) string {
	s := strings.TrimSpace(b64)
	if i := strings.Index(s, ","); i != -1 && strings.HasPrefix(strings.ToLower(s[:i]), "data:") {
		return s[i+1:]
	}
	return s
}
