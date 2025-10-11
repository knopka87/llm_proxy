package handle

import (
	"encoding/json"
	"net/http"

	"llm-proxy/api/internal/ocr"
)

type Handle struct {
	engs *ocr.Engines
}

func New(engs *ocr.Engines) *Handle {
	return &Handle{
		engs: engs,
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
