package handle

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"llm-proxy/api/internal/ocr"
)

type HintRequest struct {
	LLMName string `json:"llm_name"`
	ocr.HintInput
}

func (d *Handle) Hint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req HintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 70*time.Second)
	defer cancel()

	var out ocr.HintResult

	engine, err := d.engs.GetEngine(req.LLMName)
	if err != nil {
		http.Error(w, "detect error: "+err.Error(), http.StatusBadGateway)
		return
	}

	out, err = engine.Hint(ctx, req.HintInput)
	if err != nil {
		http.Error(w, "detect error: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, out)
}
