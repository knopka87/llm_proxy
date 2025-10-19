package handle

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"llm-proxy/api/internal/ocr/types"
)

func stripDataURL(b64 string) string {
	s := strings.TrimSpace(b64)
	if i := strings.Index(s, ","); i != -1 && strings.HasPrefix(strings.ToLower(s[:i]), "data:") {
		return s[i+1:]
	}
	return s
}

type DetectRequest struct {
	LLMName string `json:"llm_name"`
	types.DetectInput
}

func (h *Handle) Detect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	var req DetectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json: " + err.Error()})
		return
	}

	req.ImageB64 = stripDataURL(req.ImageB64)
	img, err := base64.StdEncoding.DecodeString(req.ImageB64)
	if err != nil || len(img) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad image_b64"})
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

	var out types.DetectResult

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "detect error: " + err.Error()})
		return
	}

	out, err = engine.Detect(ctx, req.DetectInput)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "detect error: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, out)
}
