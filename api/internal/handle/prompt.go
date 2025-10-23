package handle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Update API request payload.
type updatePromptRequest struct {
	Provider string `json:"provider"` // e.g. "gpt" | "gemini"
	Name     string `json:"name"`     // filename WITHOUT extension (e.g. "detect")
	Text     string `json:"text"`     // new prompt body
}

// Update API response payload.
type updatePromptResponse struct {
	OK       bool   `json:"ok"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int    `json:"size"`
	Updated  string `json:"updated_at"`
}

// Allowed characters to avoid path traversal and weird names.
var (
	allowedProviderRe = regexp.MustCompile(`^[a-z0-9_-]+$`)
	allowedNameRe     = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// UpdateSystemPromptHandler persists a new/updated prompt file into
// api/internal/ocr/<provider>/prompt/<name>.txt using an atomic rename.
func (h *Handle) UpdateSystemPromptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var req updatePromptRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20)) // 4 MiB limit
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := validateUpdatePromptRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	baseDir := filepath.Join("api", "internal", "ocr", strings.ToLower(req.Provider), "prompt")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		http.Error(w, "make dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure .txt extension on disk, while the "name" field is extensionless for API semantics.
	filename := req.Name
	if strings.HasSuffix(strings.ToLower(filename), ".txt") {
		// If caller accidentally supplied ".txt", strip it to avoid ".txt.txt".
		filename = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	dstPath := filepath.Join(baseDir, filename+".txt")

	// Write atomically: temp file in the same directory, then rename.
	tmp, err := os.CreateTemp(baseDir, filename+".*.tmp")
	if err != nil {
		http.Error(w, "create temp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(req.Text); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		http.Error(w, "write temp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmp.Chmod(0o644); err != nil {
		// Non-fatal, but try to keep consistent perms.
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		http.Error(w, "close temp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		http.Error(w, "rename: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := updatePromptResponse{
		OK:       true,
		Provider: strings.ToLower(req.Provider),
		Name:     filename,
		Path:     dstPath,
		Size:     len(req.Text),
		Updated:  time.Now().UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func validateUpdatePromptRequest(req updatePromptRequest) error {
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
