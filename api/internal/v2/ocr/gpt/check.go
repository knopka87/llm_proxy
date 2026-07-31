package gpt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v2/ocr/types"
)

const CHECK = "check"

func (e *Engine) CheckSolution(ctx context.Context, in types.CheckRequest) (types.CheckResponse, *types.LLMStats, error) {
	log.Printf("[check] started, image_len=%d, task_text=%q, items_count=%d",
		len(in.Image), truncateStr(in.RawTaskText, 50), len(in.TaskStruct.Items))

	if e.apiKey == "" {
		log.Printf("[check] ERROR: OPENAI_API_KEY is empty")
		return types.CheckResponse{}, nil, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	model := e.GetModel()
	if model == "" {
		model = "gpt-5-mini"
	}

	system, err := util.LoadSystemPrompt(CHECK, e.Name(), e.Version(), "check")
	if err != nil {
		return types.CheckResponse{}, nil, err
	}

	// Подставляем grade-специфичную секцию feedback
	if gradeSection, serr := loadCheckFeedbackSection(int(in.Student.Grade)); serr == nil {
		system = strings.ReplaceAll(system, "{{GRADE_FEEDBACK_SECTION}}", gradeSection)
	}

	// Композиция дополнительных блоков промпта
	system = composeCheckBlocks(system, in.TaskStruct)

	schema, err := util.LoadPromptSchema(CHECK, e.Version())
	if err != nil {
		return types.CheckResponse{}, nil, err
	}
	util.FixJSONSchemaStrict(schema)

	user, err := util.LoadUserPrompt(CHECK, e.Name(), e.Version(), "check")
	if err != nil {
		return types.CheckResponse{}, nil, err
	}

	// Decode image from base64 and create data URL for multimodal input
	imgBytes, mimeFromDataURL, _ := util.DecodeBase64MaybeDataURL(in.Image)
	if len(imgBytes) == 0 {
		raw, err := base64.StdEncoding.DecodeString(in.Image)
		if err != nil {
			return types.CheckResponse{}, nil, fmt.Errorf("openai check: invalid image base64")
		}
		imgBytes = raw
	}
	mime := util.PickMIME("", mimeFromDataURL, imgBytes)
	if !isOpenAIImageMIME(mime) {
		return types.CheckResponse{}, nil, fmt.Errorf("openai check: unsupported MIME %s (need image/jpeg|png|webp)", mime)
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)
	in.Image = "" // Clear from JSON since sending as separate image block

	userObj := map[string]any{
		"task":  user,
		"input": in,
	}
	userJSON, _ := json.Marshal(userObj)

	body := map[string]any{
		"model": model,
		"input": []any{
			map[string]any{
				"role": "system",
				"content": []any{
					map[string]any{"type": "input_text", "text": system},
				},
			},
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "INPUT_JSON:\n" + string(userJSON)},
					map[string]any{"type": "input_image", "image_url": dataURL},
				},
			},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   CHECK,
				"strict": true,
				"schema": schema,
			},
		},
	}
	// Низкая температура для детерминизма проверки — одинаковый ответ должен
	// давать одинаковый результат при повторных запросах.
	// gpt-5 не поддерживает параметр temperature.
	if !strings.Contains(model, "gpt-5") {
		body["temperature"] = 0.1
	}

	payload, _ := json.Marshal(body)
	log.Printf("[check] calling OpenAI API, model=%s, payload_len=%d", model, len(payload))

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	start := time.Now()
	resp, err := e.httpc.Do(req)
	if err != nil {
		log.Printf("[check] ERROR: HTTP request failed: %v", err)
		return types.CheckResponse{}, nil, err
	}
	defer resp.Body.Close()

	log.Printf("[check] OpenAI response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		log.Printf("[check] ERROR: OpenAI returned %d: %s", resp.StatusCode, truncateStr(string(x), 500))
		return types.CheckResponse{}, nil, fmt.Errorf("openai check %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	t := time.Since(start).Milliseconds()
	inTok, outTok := parseUsage(raw)
	stats := &types.LLMStats{InputTokens: inTok, OutputTokens: outTok, LatencyMs: t, Model: model}
	log.Printf("[check] OpenAI response body_len=%d", len(raw))

	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		log.Printf("[check] ERROR: empty output from OpenAI, body=%s", truncateBytes(raw, 500))
		return types.CheckResponse{}, stats, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}

	var cr types.CheckResponse
	if err := json.Unmarshal([]byte(out), &cr); err != nil {
		log.Printf("[check] ERROR: bad JSON from OpenAI: %v, out=%s", err, truncateStr(out, 500))
		return types.CheckResponse{}, stats, fmt.Errorf("openai check: bad JSON: %w", err)
	}

	// P0.3: Нормализация Decision из IsCorrect для обратной совместимости
	cr.NormalizeDecision()
	// Заполняем IsCorrect из Decision для обратной совместимости с клиентами
	cr.SetIsCorrectFromDecision()

	log.Printf("[check] success: status=%s, can_evaluate=%v, decision=%s, is_correct=%v",
		cr.Status, cr.CanEvaluate, cr.Decision, cr.IsCorrect)
	return cr, stats, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func gradeSubdir(grade int) string {
	switch grade {
	case 1:
		return "1_class"
	case 2:
		return "2_class"
	case 3:
		return "3_class"
	case 4:
		return "4_class"
	default:
		return ""
	}
}

func loadCheckFeedbackSection(grade int) (string, error) {
	subdir := gradeSubdir(grade)
	if subdir == "" {
		return "", fmt.Errorf("unknown grade: %d", grade)
	}

	baseRoot := os.Getenv("PROMPT_DIR")
	if baseRoot == "" {
		baseRoot = filepath.Join("api", "internal")
	}
	p := filepath.Join(baseRoot, "v2", "prompt", "check", subdir, "check.feedback.txt")
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("load check feedback %s: %w", subdir, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// composeCheckBlocks добавляет к базовому промпту условные блоки для проверки ответа.
// Загружает: advanced (по task_type), format (по формату), conditional (visual, multiple_subtasks).
func composeCheckBlocks(system string, taskStruct types.TaskStructCheck) string {
	var blocks []string

	if len(taskStruct.Items) > 0 {
		item := taskStruct.Items[0]

		// Advanced блок по типу задачи
		if item.PedKeys.TaskType != "" {
			if advanced, aerr := loadCheckBlock("check.advanced_" + item.PedKeys.TaskType); aerr == nil {
				blocks = append(blocks, advanced)
			}
		}

		// Format блок по формату
		if item.PedKeys.Format != "" {
			if format, ferr := loadCheckBlock("check.format_" + item.PedKeys.Format); ferr == nil {
				blocks = append(blocks, format)
			}
		}
	}

	// Conditional блоки
	if taskStruct.VisualReasoning != nil && strings.TrimSpace(*taskStruct.VisualReasoning) != "" {
		if visual, verr := loadCheckBlock("check.visual"); verr == nil {
			blocks = append(blocks, visual)
		}
	}

	if len(taskStruct.Items) > 1 {
		if multi, merr := loadCheckBlock("check.multiple_subtasks"); merr == nil {
			blocks = append(blocks, multi)
		}
	}

	// Verify блоки по типу задачи
	if len(taskStruct.Items) > 0 {
		taskType := taskStruct.Items[0].PedKeys.TaskType
		switch taskType {
		case "arithmetic", "comparison":
			if v, err := loadCheckBlock("check.verify_arithmetic"); err == nil {
				blocks = append(blocks, v)
			}
		case "patterns":
			if v, err := loadCheckBlock("check.verify_transforms"); err == nil {
				blocks = append(blocks, v)
			}
		}
	}

	if len(blocks) > 0 {
		system = system + "\n\n" + strings.Join(blocks, "\n\n")
	}
	return system
}

// loadCheckBlock загружает промпт-блок по имени из prompt-директории.
func loadCheckBlock(name string) (string, error) {
	baseRoot := os.Getenv("PROMPT_DIR")
	if baseRoot == "" {
		baseRoot = filepath.Join("api", "internal")
	}
	p := filepath.Join(baseRoot, "v2", "prompt", "check", name+".system.txt")
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("load check block %s: %w", name, err)
	}
	return strings.TrimSpace(string(b)), nil
}
