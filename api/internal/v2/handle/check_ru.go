package handle

import (
	"context"
	"log"
	"net/http"

	"llm-proxy/api/internal/v2/ocr/types"
)

type CheckRURequest struct {
	LLMName string `json:"llm_name"`
	types.CheckRUCompactInput
}

func (h *Handle) CheckRU(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req CheckRURequest
	if err := readAndLimitBody(w, r, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	deadline := parseDeadline(r)
	ctx, cancel := context.WithTimeout(r.Context(), deadline)
	defer cancel()

	var out types.CheckRUResponse
	var stats *types.LLMStats

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		log.Printf("[check_ru] engine error: %v", err)
		http.Error(w, "engine not available", http.StatusBadGateway)
		return
	}

	out, stats, err = engine.CheckRU(ctx, req.CheckRUCompactInput)
	if err != nil {
		log.Printf("[check_ru] LLM error: %v", err)
		http.Error(w, "check_ru processing failed", http.StatusBadGateway)
		return
	}

	writeStatsHeaders(w, stats)

	writeJSON(w, http.StatusOK, out)
}
