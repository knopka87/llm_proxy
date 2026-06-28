package handle

import (
	"context"
	"log"
	"net/http"

	"llm-proxy/api/internal/v2/ocr/types"
)

type HintRURequest struct {
	LLMName string `json:"llm_name"`
	types.HintRUCompactInput
}

func (h *Handle) HintRU(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req HintRURequest
	if err := readAndLimitBody(w, r, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	deadline := parseDeadline(r)
	ctx, cancel := context.WithTimeout(r.Context(), deadline)
	defer cancel()

	var out types.HintRUResponse
	var stats *types.LLMStats

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		log.Printf("[hint_ru] engine error: %v", err)
		http.Error(w, "engine not available", http.StatusBadGateway)
		return
	}

	out, stats, err = engine.HintRU(ctx, req.HintRUCompactInput)
	if err != nil {
		log.Printf("[hint_ru] LLM error: %v", err)
		http.Error(w, "hint_ru processing failed", http.StatusBadGateway)
		return
	}

	writeStatsHeaders(w, stats)

	writeJSON(w, http.StatusOK, out)
}
