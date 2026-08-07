package handle

import (
	"context"
	"log"
	"net/http"

	"llm-proxy/api/internal/v2/ocr/types"
)

type ParseRequest struct {
	LLMName string `json:"llm_name"`
	types.ParseRequest
}

func (h *Handle) Parse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req ParseRequest
	if err := readAndLimitBody(w, r, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	deadline := parseDeadline(r)
	ctx, cancel := context.WithTimeout(r.Context(), deadline)
	defer cancel()

	var out types.ParseResponse
	var stats *types.LLMStats

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		log.Printf("[parse] engine error: %v", err)
		http.Error(w, "engine not available", http.StatusBadGateway)
		return
	}

	out, stats, err = engine.Parse(ctx, req.ParseRequest)
	if err != nil {
		log.Printf("[parse] LLM error: %v", err)
		http.Error(w, "parse processing failed", http.StatusBadGateway)
		return
	}

	writeStatsHeaders(w, stats)

	writeJSON(w, http.StatusOK, out)
}
