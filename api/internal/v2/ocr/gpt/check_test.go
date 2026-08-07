package gpt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCheckFeedbackSection(t *testing.T) {
	tmpDir := t.TempDir()
	promptDir := filepath.Join(tmpDir, "api", "internal", "v2", "prompt", "check")

	// Создаём структуру {N}_class/check.feedback.txt
	feedbackFiles := map[string]string{
		"1_class/check.feedback.txt":    "FEEDBACK_GRADE_1: очень просто",
		"2_class/check.feedback.txt":    "FEEDBACK_GRADE_2: просто",
		"3_class/check.feedback.txt":    "FEEDBACK_GRADE_3: можно термины",
		"4_class/check.feedback.txt":    "FEEDBACK_GRADE_4: можно термины",
		"1_class/check_ru.feedback.txt": "RU_FEEDBACK_GRADE_1: просто",
		"2_class/check_ru.feedback.txt": "RU_FEEDBACK_GRADE_2: просто",
		"3_class/check_ru.feedback.txt": "RU_FEEDBACK_GRADE_3: термины",
		"4_class/check_ru.feedback.txt": "RU_FEEDBACK_GRADE_4: термины",
	}
	for name, content := range feedbackFiles {
		dir := filepath.Join(promptDir, filepath.Dir(name))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(promptDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	origPromptDir := os.Getenv("PROMPT_DIR")
	os.Setenv("PROMPT_DIR", filepath.Join(tmpDir, "api", "internal"))
	defer os.Setenv("PROMPT_DIR", origPromptDir)

	tests := []struct {
		name    string
		grade   int
		want    string
		wantErr bool
	}{
		{"grade 1 math", 1, "FEEDBACK_GRADE_1", false},
		{"grade 2 math", 2, "FEEDBACK_GRADE_2", false},
		{"grade 3 math", 3, "FEEDBACK_GRADE_3", false},
		{"grade 4 math", 4, "FEEDBACK_GRADE_4", false},
		{"grade 0 math", 0, "", true},
		{"grade 5 math", 5, "", true},
		{"grade 1 RU", 1, "RU_FEEDBACK_GRADE_1", false},
		{"grade 2 RU", 2, "RU_FEEDBACK_GRADE_2", false},
		{"grade 3 RU", 3, "RU_FEEDBACK_GRADE_3", false},
		{"grade 4 RU", 4, "RU_FEEDBACK_GRADE_4", false},
		{"grade 0 RU", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			var err error
			if strings.Contains(tt.name, "RU") {
				got, err = loadCheckRUFeedbackSection(tt.grade)
			} else {
				got, err = loadCheckFeedbackSection(tt.grade)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !strings.Contains(got, tt.want) {
				t.Errorf("got %q, want containing %q", got, tt.want)
			}
		})
	}
}

func TestGradeSubdir(t *testing.T) {
	tests := []struct {
		grade int
		want  string
	}{
		{1, "1_class"},
		{2, "2_class"},
		{3, "3_class"},
		{4, "4_class"},
		{0, ""},
		{5, ""},
		{-1, ""},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("grade_%d", tt.grade)
		t.Run(name, func(t *testing.T) {
			if got := gradeSubdir(tt.grade); got != tt.want {
				t.Errorf("gradeSubdir(%d) = %q, want %q", tt.grade, got, tt.want)
			}
		})
	}
}

func TestPlaceholderSubstitution(t *testing.T) {
	basePrompt := "Правила проверки.\n\n{{GRADE_FEEDBACK_SECTION}}\n\nПравило детерминизма."
	feedback := "Стиль feedback для 1 класса."

	result := strings.ReplaceAll(basePrompt, "{{GRADE_FEEDBACK_SECTION}}", feedback)

	if strings.Contains(result, "{{GRADE_FEEDBACK_SECTION}}") {
		t.Error("placeholder not replaced")
	}
	if !strings.Contains(result, "Стиль feedback для 1 класса") {
		t.Error("feedback section not found in result")
	}
	if !strings.Contains(result, "Правила проверки") {
		t.Error("base prompt content lost")
	}
	if !strings.Contains(result, "Правило детерминизма") {
		t.Error("post-placeholder content lost")
	}
}
