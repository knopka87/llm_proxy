package handle

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"llm-proxy/api/internal/v2/ocr/types"
)

// --- ANALOGUE SOLUTION (v1.1) ----------------------------------------------

// analogueReq — входной контракт ручки /v1/analogue
// llm_name — явный выбор провайдера (gemini|gpt), если не задан — берётся дефолт
// Поля запроса соответствуют AnalogueSolutionRequest (см. types.go)
type analogueReq struct {
	LLMName string `json:"llm_name"`
	types.AnalogueRequest
}

// AnalogueSolution — HTTP-хендлер генерации «похожего задания тем же приёмом»
// Поведение соответствует инструкции ANALOGUE_SOLUTION v1.1: строгий JSON,
// анти-лик, методическая связка, когнитивная нагрузка и др.
func (h *Handle) AnalogueSolution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req analogueReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
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
		http.Error(w, "analogue error: "+err.Error(), http.StatusBadGateway)
		return
	}

	out, err := engine.AnalogueSolution(ctx, req.AnalogueRequest)
	if err != nil {
		http.Error(w, "analogue error: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, out)
}
