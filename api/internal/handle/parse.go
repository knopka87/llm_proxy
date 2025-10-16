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

type ParseRequest struct {
	LLMName string `json:"llm_name"`
	ocr.ParseInput
}

func (h *Handle) Parse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req ParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	img, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.ImageB64))
	if err != nil || len(img) == 0 {
		http.Error(w, "bad image_b64", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()

	var out ocr.ParseResult

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		http.Error(w, "parse error: "+err.Error(), http.StatusBadGateway)
		return
	}

	out, err = engine.Parse(ctx, req.ParseInput)
	if err != nil {
		http.Error(w, "parse error: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, out)
}
