package yandex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type IamClient struct {
	httpc  *http.Client
	oauth  string
	mu     sync.Mutex
	token  string
	expiry time.Time
}

func NewIamClient(oauth string) *IamClient {
	return &IamClient{
		httpc: &http.Client{Timeout: 20 * time.Second},
		oauth: oauth,
	}
}

func (c *IamClient) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expiry.Add(-time.Minute)) {
		return c.token, nil
	}

	body := map[string]string{"yandexPassportOauthToken": c.oauth}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://iam.api.cloud.yandex.net/iam/v1/tokens", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("iam %d", resp.StatusCode)
	}

	var out struct {
		IamToken string `json:"iamToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	c.token = out.IamToken
	c.expiry = time.Now().Add(11 * time.Hour)
	return c.token, nil
}
