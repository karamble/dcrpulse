// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package middleware

import "testing"

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
