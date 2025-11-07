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

// --- CHECK SOLUTION ---------------------------------------------------------

type checkReq struct {
	LLMName string `json:"llm_name"`
	types.CheckRequest
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

	out, err := engine.CheckSolution(ctx, req.CheckRequest)
	if err != nil {
		http.Error(w, "check error: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Normalize engine output to CheckSchema v1.2 (drop unknown fields, enforce required ones)
	// if b, err := json.Marshal(out); err == nil {
	// 	var raw map[string]any
	// 	if err := json.Unmarshal(b, &raw); err == nil {
	// 		normalized := normalizeCheckV12(raw)
	// 		writeJSON(w, http.StatusOK, normalized)
	// 		return
	// 	}
	// }

	// Fallback: if normalization failed for any reason, return raw output
	writeJSON(w, http.StatusOK, out)
}

// normalizeCheckV12 converts arbitrary engine output into the strict CheckSchema v1.2 shape.
// It enforces required fields, clamps values, truncates strings, and drops unknown properties.
func normalizeCheckV12(m map[string]any) map[string]any {
	// CHECK.response.v1: { is_correct: bool, feedback: string, error_spans?: [{from:int,to:int,label?:string}], confidence?: [0..1] }
	// additionalProperties: false → only include these keys.

	out := map[string]any{}

	// ---- is_correct ----
	isCorrect := false
	if v, ok := m["is_correct"].(bool); ok {
		isCorrect = v
	} else if v, ok := m["verdict"].(string); ok {
		switch v {
		case "pass", "correct", "ok", "true":
			isCorrect = true
		default:
			isCorrect = false
		}
	} else if v, ok := m["result"].(string); ok {
		switch v {
		case "pass", "correct", "ok", "true":
			isCorrect = true
		default:
			isCorrect = false
		}
	} else {
		// Conservative default: if issues present → false, otherwise false (never assume correctness).
		if issues, ok := m["issues"].([]any); ok && len(issues) > 0 {
			isCorrect = false
		} else {
			isCorrect = false
		}
	}
	out["is_correct"] = isCorrect

	// ---- feedback ----
	feedback := ""
	if v, ok := m["feedback"].(string); ok && v != "" {
		feedback = v
	} else {
		if isCorrect {
			feedback = "Решение выглядит корректным."
		} else {
			// Try to summarize issues.reason (1–2 краткие фразы, без раскрытия ответа)
			if issues, ok := m["issues"].([]any); ok && len(issues) > 0 {
				parts := make([]string, 0, 2)
				for _, it := range issues {
					if obj, ok := it.(map[string]any); ok {
						if rs, _ := obj["reason"].(string); rs != "" {
							parts = append(parts, rs)
							if len(parts) >= 2 {
								break
							}
						}
					}
				}
				if len(parts) > 0 {
					feedback = strings.Join(parts, " ")
				}
			}
			if feedback == "" {
				if v, ok := m["verdict"].(string); ok && v == "needs_more_info" {
					feedback = "Недостаточно данных: уточните шаги решения или исходные условия."
				} else {
					feedback = "В ответе обнаружены неточности. Укажите, где допущена ошибка, и пересчитайте без раскрытия финального результата."
				}
			}
		}
	}
	feedback = clampRunes(feedback, 400)
	if feedback == "" {
		// Keep it minimal if nothing else is available.
		feedback = "Проверка выполнена."
	}
	out["feedback"] = feedback

	// ---- error_spans ----
	// Prefer exact shape if present; otherwise derive from common alternatives inside issues.
	if raw, ok := m["error_spans"].([]any); ok {
		spans := make([]any, 0, len(raw))
		for _, it := range raw {
			if obj, ok := it.(map[string]any); ok {
				from, fok := toInt(obj["from"])
				toV, tok := toInt(obj["to"])
				label, _ := obj["label"].(string)
				if fok && tok {
					if from < 0 {
						from = 0
					}
					if toV < from {
						toV = from
					}
					item := map[string]any{"from": from, "to": toV}
					if label != "" {
						item["label"] = clampRunes(label, 120)
					}
					spans = append(spans, item)
				}
			}
		}
		if len(spans) > 0 {
			out["error_spans"] = spans
		}
	} else if rawIssues, ok := m["issues"].([]any); ok && len(rawIssues) > 0 {
		spans := make([]any, 0, 6)
		for _, it := range rawIssues {
			obj, ok := it.(map[string]any)
			if !ok {
				continue
			}
			// Accept several common shapes: from/to; span{from,to}; range{start,end}
			from, fok := toInt(obj["from"])
			toV, tok := toInt(obj["to"])
			if !(fok && tok) {
				if rng, ok := obj["span"].(map[string]any); ok {
					from, fok = toInt(rng["from"])
					toV, tok = toInt(rng["to"])
				}
			}
			if !(fok && tok) {
				if rng, ok := obj["range"].(map[string]any); ok {
					from, fok = toInt(rng["start"])
					toV, tok = toInt(rng["end"])
				}
			}
			label, _ := obj["label"].(string)
			if label == "" {
				if s, ok := obj["reason"].(string); ok {
					label = s
				}
			}
			if fok && tok {
				if from < 0 {
					from = 0
				}
				if toV < from {
					toV = from
				}
				item := map[string]any{"from": from, "to": toV}
				if label != "" {
					item["label"] = clampRunes(label, 120)
				}
				spans = append(spans, item)
			}
			if len(spans) >= 6 {
				break
			}
		}
		if len(spans) > 0 {
			out["error_spans"] = spans
		}
	}

	// ---- confidence ----
	var conf float64
	switch v := m["confidence"].(type) {
	case float64:
		conf = v
	case int:
		conf = float64(v)
	case int32:
		conf = float64(v)
	case int64:
		conf = float64(v)
	case json.Number:
		if f, err := strconv.ParseFloat(string(v), 64); err == nil {
			conf = f
		}
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conf = f
		}
	default:
		conf = 0
	}
	if conf <= 0 {
		if isCorrect {
			conf = 0.7
		} else {
			conf = 0.5
		}
	}
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}
	out["confidence"] = conf

	// Only allowed keys are present in `out`.
	return out
}

func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case int32:
		return int(t), true
	case int64:
		return int(t), true
	case json.Number:
		if i, err := strconv.Atoi(string(t)); err == nil {
			return i, true
		}
	case string:
		if i, err := strconv.Atoi(t); err == nil {
			return i, true
		}
	}
	return 0, false
}

// clampRunes ensures a string does not exceed max runes.
func clampRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
