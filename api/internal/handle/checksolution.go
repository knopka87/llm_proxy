package handle

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"llm-proxy/api/internal/ocr"
)

// --- CHECK SOLUTION ---------------------------------------------------------

type checkReq struct {
	LLMName string `json:"llm_name"`
	ocr.CheckSolutionInput
}

func (h *Handle) CheckSolution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req checkReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Минимальные проверки входа
	if strings.TrimSpace(req.Expected.Shape) == "" {
		http.Error(w, "expected_solution.shape is required", http.StatusBadRequest)
		return
	}
	if !req.Student.Success {
		http.Error(w, "student normalized answer is not successful (success=false)", http.StatusBadRequest)
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

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		http.Error(w, "check error: "+err.Error(), http.StatusBadGateway)
		return
	}

	out, err := engine.CheckSolution(ctx, req.CheckSolutionInput)
	if err != nil {
		http.Error(w, "check error: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, out)
}
