package gpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"llm-proxy/api/internal/v2/ocr/types"
)

const embeddingsEndpoint = "https://api.openai.com/v1/embeddings"

// Embed генерирует эмбеддинги для батча входных строк используя OpenAI API.
// Модель по умолчанию: text-embedding-3-small (может быть переопределена через OPENAI_EMBED_MODEL).
func (e *Engine) Embed(ctx context.Context, in types.EmbedRequest) (types.EmbedResponse, *types.LLMStats, error) {
	if e.apiKey == "" {
		return types.EmbedResponse{}, nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	if len(in.Input) == 0 {
		return types.EmbedResponse{}, nil, fmt.Errorf("embed: input is empty")
	}

	model := os.Getenv("OPENAI_EMBED_MODEL")
	if strings.TrimSpace(model) == "" {
		model = "text-embedding-3-small"
	}

	// Подготовка запроса к OpenAI embeddings API.
	reqBody := map[string]any{
		"model": model,
		"input": in.Input,
	}

	bodyJSON, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embeddingsEndpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return types.EmbedResponse{}, nil, fmt.Errorf("embed: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	startTime := time.Now()
	resp, err := e.httpc.Do(req)
	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		return types.EmbedResponse{}, nil, fmt.Errorf("embed: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.EmbedResponse{}, nil, fmt.Errorf("embed: failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[embed] OpenAI error: status=%d, body=%s", resp.StatusCode, truncateBytes(body, 500))
		return types.EmbedResponse{}, nil, fmt.Errorf("embed: OpenAI API returned status %d", resp.StatusCode)
	}

	// Парсинг ответа OpenAI.
	type openaiEmbedding struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	}
	type openaiResponse struct {
		Data  []openaiEmbedding `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}

	var openaiResp openaiResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return types.EmbedResponse{}, nil, fmt.Errorf("embed: failed to parse OpenAI response: %w", err)
	}

	// Сортируем эмбеддинги по index, чтобы сохранить исходный порядок (OpenAI может вернуть не по порядку).
	sort.Slice(openaiResp.Data, func(i, j int) bool {
		return openaiResp.Data[i].Index < openaiResp.Data[j].Index
	})

	// Собираем векторы в ответ.
	vectors := make([][]float32, len(openaiResp.Data))
	for i, emb := range openaiResp.Data {
		vectors[i] = emb.Embedding
	}

	stats := &types.LLMStats{
		InputTokens:  openaiResp.Usage.PromptTokens,
		OutputTokens: 0, // Embeddings API не возвращает output tokens в полезном виде.
		LatencyMs:    latencyMs,
		Model:        model,
	}

	return types.EmbedResponse{
		SchemaVersion: "embed_v1",
		Vectors:       vectors,
	}, stats, nil
}
