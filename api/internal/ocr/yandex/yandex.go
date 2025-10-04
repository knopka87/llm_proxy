package yandex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"child-bot/api/internal/ocr"
	"child-bot/api/internal/util"
)

type Engine struct {
	iamc     *IamClient
	folderID string
	httpc    *http.Client
}

func New(oauth2Token, folderID string) *Engine {
	return &Engine{
		iamc:     NewIamClient(oauth2Token),
		folderID: folderID,
		httpc:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *Engine) Name() string { return "yandex" }

type request struct {
	Content       string   `json:"content"`
	MimeType      string   `json:"mimeType,omitempty"`      // "JPEG" | "PNG" | "PDF"
	LanguageCodes []string `json:"languageCodes,omitempty"` // ["ru","en"]
	Model         string   `json:"model,omitempty"`         // e.g. "handwritten", "page"
}
type response struct {
	Result *struct {
		TextAnnotation *struct {
			FullText string `json:"fullText,omitempty"`
			Blocks   []struct {
				Lines []struct {
					Text string `json:"text,omitempty"`
				} `json:"lines,omitempty"`
			} `json:"blocks,omitempty"`
		} `json:"textAnnotation,omitempty"`
		Page string `json:"page,omitempty"`
	} `json:"result,omitempty"`
}

func (e *Engine) Recognize(ctx context.Context, image []byte, opt ocr.Options) (string, error) {
	iamToken, err := e.iamc.Token(ctx)
	if err != nil {
		return "", err
	}
	b64 := base64.StdEncoding.EncodeToString(image)
	reqBody := request{
		Content:       b64,
		MimeType:      util.SniffMimeForOCR(image),
		LanguageCodes: opt.Langs,
	}
	if opt.Model != "" {
		reqBody.Model = opt.Model
	} else {
		reqBody.Model = "handwritten"
	}
	payload, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://ocr.api.cloud.yandex.net/ocr/v1/recognizeText",
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+iamToken)
	req.Header.Set("x-folder-id", e.folderID)
	req.Header.Set("x-data-logging-enabled", "true")

	resp, err := e.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// один ретрай
		if iamToken, err = e.iamc.Token(ctx); err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+iamToken)
		resp, err = e.httpc.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
	}
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("yandex ocr %d: %s", resp.StatusCode, string(x))
	}

	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	ta := out.GetTextAnnotation()
	if ta == nil {
		return "", nil
	}
	if t := strings.TrimSpace(ta.FullText); t != "" {
		return t, nil
	}
	// fallback: lines
	var lines []string
	for _, b := range ta.Blocks {
		for _, l := range b.Lines {
			if s := strings.TrimSpace(l.Text); s != "" {
				lines = append(lines, s)
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (r *response) GetTextAnnotation() *struct {
	FullText string `json:"fullText,omitempty"`
	Blocks   []struct {
		Lines []struct {
			Text string `json:"text,omitempty"`
		} `json:"lines,omitempty"`
	} `json:"blocks,omitempty"`
} {
	if r == nil || r.Result == nil {
		return nil
	}
	return r.Result.TextAnnotation
}
