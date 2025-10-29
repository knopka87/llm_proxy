package handle

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"llm-proxy/api/internal/v2/ocr/types"
)

// --- NORMALIZE ---------------------------------------------------------------

type normalizeReq struct {
	LLMName string `json:"llm_name"`
	types.NormalizeInput
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

	if strings.TrimSpace(req.ExpectedShape) == "" {
		http.Error(w, "expected_shape is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.StudentAnswerText) == "" {
		http.Error(w, "student_answer_text is required", http.StatusBadRequest)
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

	var out types.NormalizeResult

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
