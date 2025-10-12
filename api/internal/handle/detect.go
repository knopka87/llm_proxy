package handle

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"llm-proxy/api/internal/ocr"
)

type DetectRequest struct {
	LLMName   string `json:"llm_name"`
	ImageB64  string `json:"image_b64"`
	Mime      string `json:"mime,omitempty"`
	GradeHint int    `json:"grade_hint,omitempty"`
}

func (h *Handle) Detect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req DetectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	img, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.ImageB64))
	if err != nil || len(img) == 0 {
		http.Error(w, "bad image_b64", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 70*time.Second)
	defer cancel()

	var out ocr.DetectResult

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		http.Error(w, "detect error: "+err.Error(), http.StatusBadGateway)
		return
	}

	out, err = engine.Detect(ctx, img, req.Mime, req.GradeHint)
	if err != nil {
		http.Error(w, "detect error: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, out)
}
