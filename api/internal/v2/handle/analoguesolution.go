package handle

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"llm-proxy/api/internal/v2/ocr/types"
)

// --- ANALOGUE SOLUTION (v1.1) ----------------------------------------------

// analogueReq — входной контракт ручки /v1/analogue
// llm_name — явный выбор провайдера (gpt), если не задан — берётся дефолт
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
	if err := readAndLimitBody(w, r, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	deadline := parseDeadline(r)
	ctx, cancel := context.WithTimeout(r.Context(), deadline)
	defer cancel()

	engine, err := h.engs.GetEngine(req.LLMName)
	if err != nil {
		log.Printf("[analogue] engine error: %v", err)
		http.Error(w, "engine not available", http.StatusBadGateway)
		return
	}

	out, stats, err := engine.AnalogueSolution(ctx, req.AnalogueRequest)
	if err != nil {
		log.Printf("[analogue] LLM error: %v", err)
		http.Error(w, "analogue processing failed", http.StatusBadGateway)
		return
	}

	if stats != nil {
		w.Header().Set("X-LLM-Input-Tokens", strconv.Itoa(stats.InputTokens))
		w.Header().Set("X-LLM-Output-Tokens", strconv.Itoa(stats.OutputTokens))
		w.Header().Set("X-LLM-Latency-Ms", strconv.FormatInt(stats.LatencyMs, 10))
	}

	writeJSON(w, http.StatusOK, out)
}
