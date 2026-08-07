package main

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type clientIPFilter struct {
	allowedClients []netip.Prefix
	trustedProxies []netip.Prefix
}

func newClientIPFilter(allowedClientsRaw, trustedProxiesRaw string) (*clientIPFilter, error) {
	allowedClients, err := parsePrefixes(allowedClientsRaw)
	if err != nil {
		return nil, fmt.Errorf("parse allowed client CIDRs: %w", err)
	}
	if len(allowedClients) == 0 {
		return nil, fmt.Errorf("allowed client CIDRs are empty")
	}
	trustedProxies, err := parsePrefixes(trustedProxiesRaw)
	if err != nil {
		return nil, fmt.Errorf("parse trusted proxy CIDRs: %w", err)
	}
	return &clientIPFilter{allowedClients: allowedClients, trustedProxies: trustedProxies}, nil
}

func parsePrefixes(raw string) ([]netip.Prefix, error) {
	parts := strings.Split(raw, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addressErr := netip.ParseAddr(value)
			if addressErr != nil {
				return nil, fmt.Errorf("invalid CIDR or IP %q: %w", value, err)
			}
			bits := 128
			if address.Is4() {
				bits = 32
			}
			prefix = netip.PrefixFrom(address, bits)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func (f *clientIPFilter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP, ok := f.clientIP(r)
		if !ok || !containsIP(f.allowedClients, clientIP) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (f *clientIPFilter) clientIP(r *http.Request) (netip.Addr, bool) {
	peerIP, ok := parseRemoteIP(r.RemoteAddr)
	if !ok {
		return netip.Addr{}, false
	}
	if !containsIP(f.trustedProxies, peerIP) {
		return peerIP, true
	}

	// Reverse proxy must overwrite X-Forwarded-For with $remote_addr.
	// We intentionally use only the first value and reject malformed input.
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]
	clientIP, err := netip.ParseAddr(strings.TrimSpace(forwarded))
	if err != nil {
		return netip.Addr{}, false
	}
	return clientIP.Unmap(), true
}

func parseRemoteIP(remoteAddress string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		host = remoteAddress
	}
	address, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

func containsIP(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
