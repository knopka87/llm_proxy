// Package tmplrouter selects a pedagogical template (T1–T52) for a HINT request.
// Templates live in TEMPLATES_DIR/math/ (default: api/internal/v2/templates/math/).
// Ported from child_bot/api/internal/llm/template_router.go; routing logic is identical.
package tmplrouter

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Router selects a pedagogical template for a given task text and visual context.
type Router struct {
	entries []entry
}

// Len возвращает число валидных загруженных шаблонов.
func (r *Router) Len() int {
	if r == nil {
		return 0
	}
	return len(r.entries)
}

type entry struct {
	profile json.RawMessage // extracted teaching profile sent to LLM
	code    string
	rules   []rule
}

type rule struct {
	mustHaveText   []*regexp.Regexp
	mustHaveVisual []string
	mustNotText    []*regexp.Regexp
	mustNotVisual  []string
	priority       int
}

// templateSchema parses routing metadata from T*.json.
type templateSchema struct {
	TemplateRegistry struct {
		Templates []struct {
			TemplateCode string `json:"template_code"`
			TemplateID   string `json:"template_id"`
			Routing      struct {
				RoutingRules []struct {
					MustHave struct {
						TextPatternsAny []string `json:"text_patterns_any"`
						VisualKindsAny  []string `json:"visual_kinds_any"`
					} `json:"must_have"`
					MustNot struct {
						TextPatternsAny []string `json:"text_patterns_any"`
						VisualKindsAny  []string `json:"visual_kinds_any"`
					} `json:"must_not"`
					RoutingPriority int `json:"routing_priority"`
				} `json:"routing_rules"`
			} `json:"routing"`
		} `json:"templates"`
	} `json:"template_registry"`
	TemplateProfiles map[string]json.RawMessage `json:"template_profiles"`
}

// New loads all T*.json files from the templates directory.
// TEMPLATES_DIR env var overrides the default path.
// Non-fatal: if no templates are found, routing is silently disabled.
func New() *Router {
	dir := templatesDir()
	r := &Router{}
	paths, _ := filepath.Glob(filepath.Join(dir, "T*.json"))
	if len(paths) == 0 {
		log.Printf("[tmplrouter] no templates found in %q — hint routing disabled", dir)
		return r
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			log.Printf("[tmplrouter] skip %s: %v", filepath.Base(p), err)
			continue
		}
		e, err := buildEntry(raw)
		if err != nil {
			log.Printf("[tmplrouter] skip %s: %v", filepath.Base(p), err)
			continue
		}
		r.entries = append(r.entries, *e)
	}
	log.Printf("[tmplrouter] loaded %d templates from %q", len(r.entries), dir)
	return r
}

// RouteProfile returns the JSON teaching profile for the best-matching template,
// or "" if nothing matched. taskText is task_text_clean; visualKinds are
// visual_facts[].kind values from PARSE.
func (r *Router) RouteProfile(taskText string, visualKinds []string) string {
	if len(r.entries) == 0 || strings.TrimSpace(taskText) == "" {
		return ""
	}
	lower := strings.ToLower(taskText)
	var best json.RawMessage
	bestPriority := -1
	for i := range r.entries {
		for _, ru := range r.entries[i].rules {
			if ru.matches(lower, visualKinds) && ru.priority > bestPriority {
				bestPriority = ru.priority
				best = r.entries[i].profile
			}
		}
	}
	if best == nil {
		return ""
	}
	return string(best)
}

func (ru *rule) matches(textLower string, visualKinds []string) bool {
	if len(ru.mustHaveText) > 0 {
		ok := false
		for _, rx := range ru.mustHaveText {
			if rx.MatchString(textLower) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(ru.mustHaveVisual) > 0 {
		ok := false
		for _, need := range ru.mustHaveVisual {
			for _, have := range visualKinds {
				if strings.EqualFold(have, need) {
					ok = true
					break
				}
			}
			if ok {
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, rx := range ru.mustNotText {
		if rx.MatchString(textLower) {
			return false
		}
	}
	for _, banned := range ru.mustNotVisual {
		for _, have := range visualKinds {
			if strings.EqualFold(have, banned) {
				return false
			}
		}
	}
	return true
}

func buildEntry(raw []byte) (*entry, error) {
	var schema templateSchema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	templates := schema.TemplateRegistry.Templates
	if len(templates) == 0 {
		return nil, fmt.Errorf("no templates in registry")
	}
	tmpl := templates[0]

	profile := extractProfile(tmpl.TemplateID, schema.TemplateProfiles)

	e := &entry{
		code:    tmpl.TemplateCode,
		profile: profile,
	}
	for _, rr := range tmpl.Routing.RoutingRules {
		ru := rule{
			mustHaveVisual: rr.MustHave.VisualKindsAny,
			mustNotVisual:  rr.MustNot.VisualKindsAny,
			priority:       rr.RoutingPriority,
		}
		ru.mustHaveText = compilePatterns(rr.MustHave.TextPatternsAny)
		ru.mustNotText = compilePatterns(rr.MustNot.TextPatternsAny)
		if len(ru.mustHaveText) == 0 && len(ru.mustHaveVisual) == 0 {
			continue
		}
		e.rules = append(e.rules, ru)
	}
	if len(e.rules) == 0 {
		return nil, fmt.Errorf("template %s: no valid routing rules", tmpl.TemplateCode)
	}
	return e, nil
}

// extractProfile extracts the compact teaching profile for templateID.
// Returns a JSON object with template_id + the profile fields.
func extractProfile(templateID string, profiles map[string]json.RawMessage) json.RawMessage {
	raw, ok := profiles[templateID]
	if !ok {
		// fallback: first profile
		for _, v := range profiles {
			raw = v
			break
		}
	}
	if raw == nil {
		return nil
	}
	// Wrap with template_id so LLM knows which template was selected.
	wrapped := map[string]interface{}{
		"template_id": templateID,
		"profile":     json.RawMessage(raw),
	}
	b, err := json.Marshal(wrapped)
	if err != nil {
		return raw
	}
	return b
}

func compilePatterns(patterns []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range patterns {
		rx, err := regexp.Compile("(?i)" + p)
		if err != nil {
			continue
		}
		out = append(out, rx)
	}
	return out
}

func templatesDir() string {
	base := os.Getenv("TEMPLATES_DIR")
	if base == "" {
		base = filepath.Join("api", "internal", "v2", "templates")
	}
	return filepath.Join(base, "math")
}
