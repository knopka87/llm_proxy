package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPFilter(t *testing.T) {
	t.Parallel()

	filter, err := newClientIPFilter("77.222.60.149/32", "127.0.0.1/32")
	if err != nil {
		t.Fatalf("newClientIPFilter() error = %v", err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := filter.Middleware(next)

	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		wantStatus int
	}{
		{name: "direct allowed", remoteAddr: "77.222.60.149:1234", wantStatus: http.StatusNoContent},
		{name: "direct denied", remoteAddr: "203.0.113.10:1234", wantStatus: http.StatusForbidden},
		{name: "untrusted cannot spoof", remoteAddr: "203.0.113.10:1234", forwarded: "77.222.60.149", wantStatus: http.StatusForbidden},
		{name: "trusted proxy allowed", remoteAddr: "127.0.0.1:1234", forwarded: "77.222.60.149", wantStatus: http.StatusNoContent},
		{name: "trusted proxy denied client", remoteAddr: "127.0.0.1:1234", forwarded: "203.0.113.10", wantStatus: http.StatusForbidden},
		{name: "trusted proxy missing header", remoteAddr: "127.0.0.1:1234", wantStatus: http.StatusForbidden},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-For", test.forwarded)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestNewClientIPFilterRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := newClientIPFilter("", ""); err == nil {
		t.Fatal("empty allowlist was accepted")
	}
	if _, err := newClientIPFilter("not-an-ip", ""); err == nil {
		t.Fatal("invalid allowlist was accepted")
	}
}
