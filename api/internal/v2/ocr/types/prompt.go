package types

import (
	"fmt"
	"regexp"
	"strings"
)

// Allowed characters to avoid path traversal and weird names.
var (
	allowedProviderRe = regexp.MustCompile(`^[a-z0-9_-]+$`)
	allowedNameRe     = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// UpdatePromptRequest Update API request payload.
type UpdatePromptRequest struct {
	Provider string `json:"provider"` // e.g. "gpt" | "gemini"
	Name     string `json:"name"`     // filename WITHOUT extension (e.g. "detect")
	Text     string `json:"text"`     // new prompt body
}

// UpdatePromptResponse Update API response payload.
type UpdatePromptResponse struct {
	OK       bool   `json:"ok"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int    `json:"size"`
	Updated  string `json:"updated_at"`
}

func (req *UpdatePromptRequest) Validate() error {
	if req.Provider == "" || !allowedProviderRe.MatchString(strings.ToLower(req.Provider)) {
		return fmt.Errorf("invalid provider: must match %q", allowedProviderRe.String())
	}
	if req.Name == "" || !allowedNameRe.MatchString(req.Name) ||
		strings.Contains(req.Name, "/") || strings.Contains(req.Name, `\`) || strings.Contains(req.Name, "..") {
		return fmt.Errorf("invalid name: must be a simple basename without extension; allowed %q", allowedNameRe.String())
	}
	if len(req.Text) == 0 {
		return fmt.Errorf("text is required")
	}
	// Optional: small sanity limit to avoid accidental megabytes; adjust as needed.
	if len(req.Text) > 2*1024*1024 {
		return fmt.Errorf("text too large (max 2 MiB)")
	}
	return nil
}
