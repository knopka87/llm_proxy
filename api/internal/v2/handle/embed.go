package handle

import (
	"context"
	"log"
	"net/http"

	"llm-proxy/api/internal/v2/ocr/types"
)

func (h *Handle) Embed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}

	var req struct {
		LLMName string   `json:"llm_name"`
		Input   []string `json:"input"`
	}

	if err := readAndLimitBody(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}

	// Валидация входных данных.
	if len(req.Input) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required and must not be empty"})
		return
	}

	deadline := parseDeadline(r)
	ctx, cancel := context.WithTimeout(r.Context(), deadline)
	defer cancel()

	log.Printf("[embed] llm_name=%q, input_count=%d", req.LLMName, len(req.Input))

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		log.Printf("[embed] engine error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "engine not available"})
		return
	}

	out, stats, err := engine.Embed(ctx, types.EmbedRequest{
		LLMName: req.LLMName,
		Input:   req.Input,
	})
	if err != nil {
		log.Printf("[embed] LLM error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "embed processing failed"})
		return
	}

	writeStatsHeaders(w, stats)
	writeJSON(w, http.StatusOK, out)
}
