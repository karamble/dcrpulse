// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostAllowedDefaults(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"", true},
		{"localhost", true},
		{"localhost:8080", true},
		{"app.localhost:8080", true},
		{"127.0.0.1", true},
		{"127.0.0.1:8080", true},
		{"192.168.1.50:8080", true},
		{"95.216.110.66:8080", true},
		{"::1", true},
		{"[::1]", true},
		{"[::1]:8080", true},
		{"dashboard:8080", true},
		{"dcrpulse-dashboard:8080", true},
		{"dcrpulse_dashboard_1:8735", true},
		{"umbrel.local", true},
		{"umbrel.local:8735", true},
		{"umbrel-dev.local:8735", true},
		{"casaos.local:8080", true},
		{"UMBREL.LOCAL:8735", true},
		{"umbrel.local.", true},
		{"pqrstuvwxyz234567.onion", true},
		{"evil.com", false},
		{"evil.com:8080", false},
		{"dcrpulse.example.com", false},
		{"dcrpulse.95.216.110.66.sslip.io", false},
		{"umbrel.local.evil.com", false},
		{"onion.evil.com", false},
	}
	for _, tt := range tests {
		if got := hostAllowed(tt.host); got != tt.want {
			t.Errorf("hostAllowed(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestHostAllowedFromEnv(t *testing.T) {
	t.Setenv("DASHBOARD_ALLOWED_HOSTS", "dcrpulse.example.com, dcrpulse.95.216.110.66.sslip.io")
	allowed := []string{
		"dcrpulse.example.com",
		"dcrpulse.example.com:8080",
		"DCRPULSE.EXAMPLE.COM",
		"dcrpulse.95.216.110.66.sslip.io",
	}
	for _, h := range allowed {
		if !hostAllowed(h) {
			t.Errorf("hostAllowed(%q) = false, want true", h)
		}
	}
	if hostAllowed("evil.com") {
		t.Error(`hostAllowed("evil.com") = true, want false`)
	}
}

func TestHostAllowedWildcard(t *testing.T) {
	t.Setenv("DASHBOARD_ALLOWED_HOSTS", "*")
	if !hostAllowed("evil.com") {
		t.Error(`hostAllowed("evil.com") = false, want true with the wildcard set`)
	}
}

// The cap used to be skipped whenever the caller said multipart, which let a
// JSON body arrive unbounded on any route just by setting a header. A caller
// picks its own Content-Type; it does not pick the route it reached, so the
// exemption keys on the path instead.
func TestLimitJSONBodyCapsWhateverTheCallerClaims(t *testing.T) {
	const cap, body = 100, 500
	const upload = "/api/br/files/send"
	multipart := "multipart/form-data; boundary=x"

	h := LimitJSONBody(cap, map[string]bool{upload: true})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n, err := io.Copy(io.Discard, r.Body)
			if err != nil {
				http.Error(w, "too large", http.StatusRequestEntityTooLarge)
				return
			}
			fmt.Fprintf(w, "%d", n)
		}))

	tests := []struct {
		name       string
		method     string
		path       string
		ctype      string
		wantCapped bool
	}{
		{"json on a session-less route", http.MethodPost, "/api/auth/login", "application/json", true},
		// The row this change exists for: the same body, one header different.
		{"the same body called multipart", http.MethodPost, "/api/auth/login", multipart, true},
		{"put", http.MethodPut, "/api/wallet/settings", multipart, true},
		{"patch", http.MethodPatch, "/api/wallet/settings", multipart, true},
		{"delete", http.MethodDelete, "/api/wallet/settings", multipart, true},
		// GET carries no body worth capping and the streaming routes are all GET.
		{"get", http.MethodGet, "/api/auth/status", "application/json", false},
		// The upload routes stream under their own larger cap, and that holds
		// whatever they claim to be carrying.
		{"an upload route", http.MethodPost, upload, multipart, false},
		{"an upload route called json", http.MethodPost, upload, "application/json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader(bytes.Repeat([]byte("x"), body)))
			req.Header.Set("Content-Type", tt.ctype)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if tt.wantCapped {
				if w.Code != http.StatusRequestEntityTooLarge {
					t.Fatalf("answered %d %q, want the body capped at %d", w.Code, strings.TrimSpace(w.Body.String()), cap)
				}
				return
			}
			if got := strings.TrimSpace(w.Body.String()); got != fmt.Sprint(body) {
				t.Fatalf("handler read %q of %d bytes, want the whole body uncapped", got, body)
			}
		})
	}
}
