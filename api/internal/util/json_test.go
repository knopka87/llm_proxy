package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSystemPrompt_NestedGradeDirectory(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", filepath.Clean(filepath.Join(workingDirectory, "..")))

	prompt, err := LoadSystemPrompt("hint", "openrouter", "v2", "hint", "3_class")
	if err != nil {
		t.Fatalf("LoadSystemPrompt() error: %v", err)
	}
	if !strings.Contains(prompt, "(3 класс)") {
		t.Fatalf("grade-specific prompt was not loaded: %q", prompt[:min(len(prompt), 80)])
	}
}
