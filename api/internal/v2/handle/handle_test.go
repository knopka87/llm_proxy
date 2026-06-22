package handle

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseDeadline(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		header   string
		expected time.Duration
	}{
		{
			name:     "no header returns default 180s",
			url:      "/",
			header:   "",
			expected: 180 * time.Second,
		},
		{
			name:     "X-Request-Timeout header returns custom value",
			url:      "/",
			header:   "60",
			expected: 60 * time.Second,
		},
		{
			name:     "timeoutSec query param returns custom value",
			url:      "/?timeoutSec=90",
			header:   "",
			expected: 90 * time.Second,
		},
		{
			name:     "X-Request-Timeout takes precedence over timeoutSec",
			url:      "/?timeoutSec=90",
			header:   "60",
			expected: 60 * time.Second,
		},
		{
			name:     "very large value capped at 5 minutes",
			url:      "/",
			header:   "99999",
			expected: 5 * time.Minute,
		},
		{
			name:     "exactly 5 minutes is not capped",
			url:      "/",
			header:   "300",
			expected: 5 * time.Minute,
		},
		{
			name:     "large timeoutSec capped at 5 minutes",
			url:      "/?timeoutSec=600",
			header:   "",
			expected: 5 * time.Minute,
		},
		{
			name:     "zero header returns default",
			url:      "/",
			header:   "0",
			expected: 180 * time.Second,
		},
		{
			name:     "negative header returns default",
			url:      "/",
			header:   "-10",
			expected: 180 * time.Second,
		},
		{
			name:     "non-numeric header returns default",
			url:      "/",
			header:   "abc",
			expected: 180 * time.Second,
		},
		{
			name:     "non-numeric timeoutSec returns default",
			url:      "/?timeoutSec=abc",
			header:   "",
			expected: 180 * time.Second,
		},
		{
			name:     "empty timeoutSec returns default",
			url:      "/?timeoutSec=",
			header:   "",
			expected: 180 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			if tt.header != "" {
				req.Header.Set("X-Request-Timeout", tt.header)
			}
			got := parseDeadline(req)
			if got != tt.expected {
				t.Errorf("parseDeadline() = %v, want %v", got, tt.expected)
			}
		})
	}
}
