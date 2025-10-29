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
	types.CheckSolutionInput
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
	if strings.TrimSpace(req.StudentNormalized.Shape) == "" {
		http.Error(w, "student_normalized.shape is required", http.StatusBadRequest)
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

	// Normalize engine output to CheckSchema v1.2 (drop unknown fields, enforce required ones)
	if b, err := json.Marshal(out); err == nil {
		var raw map[string]any
		if err := json.Unmarshal(b, &raw); err == nil {
			normalized := normalizeCheckV12(raw)
			writeJSON(w, http.StatusOK, normalized)
			return
		}
	}

	// Fallback: if normalization failed for any reason, return raw output
	writeJSON(w, http.StatusOK, out)
}

// normalizeCheckV12 converts arbitrary engine output into the strict CheckSchema v1.2 shape.
// It enforces required fields, clamps values, truncates strings, and drops unknown properties.
func normalizeCheckV12(m map[string]any) map[string]any {
	res := map[string]any{}

	// version (const)
	res["version"] = "1.2"

	// branch (enum)
	branch := "generic_branch"
	if v, ok := m["branch"].(string); ok {
		switch v {
		case "math_branch", "ru_branch", "generic_branch":
			branch = v
		}
	}
	res["branch"] = branch

	// verdict (enum)
	verdict := "needs_more_info"
	if v, ok := m["verdict"].(string); ok {
		switch v {
		case "pass", "fail", "needs_more_info":
			verdict = v
		}
	}
	res["verdict"] = verdict

	// confidence (0..1)
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
	default:
		conf = 0
	}
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}
	res["confidence"] = conf

	// issues (array of up to 3)
	if rawIssues, ok := m["issues"].([]any); ok && len(rawIssues) > 0 {
		issues := make([]any, 0, 3)
		for _, it := range rawIssues {
			if len(issues) >= 3 {
				break
			}
			obj, ok := it.(map[string]any)
			if !ok {
				continue
			}
			reason, _ := obj["reason"].(string)
			if reason == "" {
				continue // required
			}
			reason = clampRunes(reason, 180)
			item := map[string]any{"reason": reason}

			if fs, ok := obj["fix_suggestions"].([]any); ok && len(fs) > 0 {
				fixes := make([]any, 0, 2)
				for _, fv := range fs {
					if len(fixes) >= 2 {
						break
					}
					if s, ok := fv.(string); ok {
						fixes = append(fixes, clampRunes(s, 240))
					}
				}
				if len(fixes) > 0 {
					item["fix_suggestions"] = fixes
				}
			}
			issues = append(issues, item)
		}
		if len(issues) > 0 {
			res["issues"] = issues
		}
	}

	// math_checks (boolean flags, drop unknowns)
	if mcRaw, ok := m["math_checks"].(map[string]any); ok {
		mc := map[string]any{}
		if v, ok := mcRaw["units_ok"].(bool); ok {
			mc["units_ok"] = v
		}
		if v, ok := mcRaw["rounding_ok"].(bool); ok {
			mc["rounding_ok"] = v
		}
		if v, ok := mcRaw["substitution_ok"].(bool); ok {
			mc["substitution_ok"] = v
		}
		if len(mc) > 0 {
			res["math_checks"] = mc
		}
	}

	// ru_checks (strings, drop unknowns)
	if rcRaw, ok := m["ru_checks"].(map[string]any); ok {
		rc := map[string]any{}
		if v, ok := rcRaw["rule_ref"].(string); ok {
			rc["rule_ref"] = v
		}
		if v, ok := rcRaw["counterexample"].(string); ok {
			rc["counterexample"] = v
		}
		if len(rc) > 0 {
			res["ru_checks"] = rc
		}
	}

	// safety (required object, with required booleans)
	safety := map[string]any{}
	if sRaw, ok := m["safety"].(map[string]any); ok {
		if v, ok := sRaw["pii_removed"].(bool); ok {
			safety["pii_removed"] = v
		} else {
			safety["pii_removed"] = false
		}
		if v, ok := sRaw["banned_content"].(bool); ok {
			safety["banned_content"] = v
		} else {
			safety["banned_content"] = false
		}
		if pcs, ok := sRaw["pii_categories"].([]any); ok {
			out := make([]any, 0, len(pcs))
			for _, item := range pcs {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			if len(out) > 0 {
				safety["pii_categories"] = out
			}
		}
		if v, ok := sRaw["notes"].(string); ok {
			safety["notes"] = v
		}
	} else {
		safety["pii_removed"] = false
		safety["banned_content"] = false
	}
	res["safety"] = safety

	return res
}

// clampRunes ensures a string does not exceed max runes.
func clampRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
