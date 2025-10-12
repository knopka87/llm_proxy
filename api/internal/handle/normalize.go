package handle

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"llm-proxy/api/internal/ocr"
)

// --- NORMALIZE ---------------------------------------------------------------

type normalizeReq struct {
	LLMName string `json:"llm_name"`
	ocr.NormalizeInput
}

func (h *Handle) Normalize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req normalizeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.SolutionShape) == "" {
		http.Error(w, "solution_shape is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Answer.Source) == "" {
		http.Error(w, "answer.source is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 70*time.Second)
	defer cancel()

	var out ocr.NormalizeResult

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		http.Error(w, "normalize error: "+err.Error(), http.StatusBadGateway)
		return
	}

	out, err = engine.Normalize(ctx, req.NormalizeInput)
	if err != nil {
		http.Error(w, "normalize error: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, out)
}
