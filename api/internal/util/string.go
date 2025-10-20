package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ---- Responses API helpers ----

type ResponsesEnvelope struct {
	Output []struct {
		Content []struct {
			Type string `json:"type"` // e.g. "output_text"
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

// ExtractResponsesText reads the OpenAI Responses API JSON and returns the first output_text text.
func ExtractResponsesText(r io.Reader) (string, error) {
	var env ResponsesEnvelope
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		return "", err
	}
	if len(env.Output) == 0 || len(env.Output[0].Content) == 0 {
		return "", fmt.Errorf("responses: empty output")
	}
	for _, c := range env.Output[0].Content {
		if strings.EqualFold(c.Type, "output_text") && strings.TrimSpace(c.Text) != "" {
			return strings.TrimSpace(c.Text), nil
		}
	}
	// fallback: first chunk with any text
	for _, c := range env.Output[0].Content {
		if strings.TrimSpace(c.Text) != "" {
			return strings.TrimSpace(c.Text), nil
		}
	}
	return "", fmt.Errorf("responses: no text content")
}

func StripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// SHA256Hex возвращает SHA-256 хэш входных данных в виде шестнадцатеричной строки (нижний регистр).
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
