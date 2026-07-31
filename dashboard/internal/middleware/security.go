// Package middleware provides HTTP middleware used by the dashboard.
package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// buildCSP assembles the document Content-Security-Policy. Any sha256 hashes in
// scriptHashes are added to script-src so intentionally-shipped inline scripts
// (e.g. the pre-mount theme loader in index.html) are allowed without weakening
// the policy with 'unsafe-inline'.
func buildCSP(scriptHashes []string) string {
	scriptSrc := "script-src 'self'"
	for _, h := range scriptHashes {
		scriptSrc += " '" + h + "'"
	}
	return "default-src 'self'; " +
		scriptSrc + "; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; " +
		"font-src 'self'; " +
		"connect-src 'self' ws: wss:; " +
		"frame-ancestors 'self'; " +
		"base-uri 'self'; " +
		"form-action 'self'"
}

var csp = buildCSP(nil)

// InlineScriptHash returns the CSP script-src token for an inline script body.
func InlineScriptHash(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

// ConfigureInlineScriptHashes rebuilds the document CSP to allow the given
// inline script hashes. Call once at startup before serving requests.
func ConfigureInlineScriptHashes(hashes ...string) {
	csp = buildCSP(hashes)
}

// trustProxyHeaders reports whether X-Forwarded-Host should be honored when
// resolving the request's external host. Set TRUSTED_PROXY=true in compose
// files that run the dashboard behind a reverse proxy (Umbrel, CasaOS).
func trustProxyHeaders() bool {
	return strings.EqualFold(os.Getenv("TRUSTED_PROXY"), "true")
}

// expectedHost returns the host the browser sees us as, which is what an
// Origin header must match. Falls back to r.Host when no trusted proxy.
func expectedHost(r *http.Request) string {
	if trustProxyHeaders() {
		if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
			return fwd
		}
	}
	return r.Host
}

// extraAllowedHosts returns the host names accepted in addition to the
// built-in ones. Set DASHBOARD_ALLOWED_HOSTS to a comma-separated list when
// the dashboard is reached through a reverse proxy under a domain name; a
// single "*" accepts anything.
func extraAllowedHosts() []string {
	raw := os.Getenv("DASHBOARD_ALLOWED_HOSTS")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

// hostOnly strips the port and any trailing dot from a Host header value.
func hostOnly(host string) string {
	if strings.HasPrefix(host, "[") {
		// [::1] with no port: SplitHostPort would reject it.
		if i := strings.LastIndex(host, "]"); i > 0 && !strings.Contains(host[i:], ":") {
			return host[1:i]
		}
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(host, ".")
}

// hostAllowed reports whether a Host header value may address this dashboard.
// Treating whatever host arrives as our own would make the Origin comparison
// self-referential, so the names accepted by default are the ones that cannot
// be aimed here through public DNS: address literals, single-label names, and
// the mDNS and onion suffixes. A public domain has to be named in
// DASHBOARD_ALLOWED_HOSTS.
func hostAllowed(host string) bool {
	h := strings.ToLower(hostOnly(host))
	// Requests that carry no Host reach us by address alone.
	if h == "" {
		return true
	}
	if net.ParseIP(h) != nil {
		return true
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	// A name with no dot cannot come from public DNS. Container and compose
	// service names land here, including the Umbrel app proxy's target.
	if !strings.Contains(h, ".") {
		return true
	}
	// mDNS names resolve on the local network only (umbrel.local and a
	// renamed device's own .local name); an onion address is itself a key.
	if strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".onion") {
		return true
	}
	for _, a := range extraAllowedHosts() {
		a = strings.TrimSpace(a)
		if a == "*" || (a != "" && strings.EqualFold(hostOnly(a), h)) {
			return true
		}
	}
	return false
}

// RequireAllowedHost rejects requests carrying a Host this dashboard does not
// serve. Unlike RequireSameOrigin it applies to every method, since reads and
// WebSocket upgrades are GETs.
func RequireAllowedHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(expectedHost(r)) {
			http.Error(w, "host not served by this dashboard; set DASHBOARD_ALLOWED_HOSTS to accept it", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireSameOrigin rejects state-changing requests whose Origin header does
// not match the dashboard's own host. GET/HEAD/OPTIONS pass through.
func RequireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host != expectedHost(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SameOriginWS is the gorilla/websocket Upgrader.CheckOrigin function. It
// shares the host-resolution logic with RequireSameOrigin so behaviour is
// identical between HTTP POST and WebSocket upgrade.
func SameOriginWS(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == expectedHost(r)
}

// SecurityHeaders sets browser hardening headers on every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=()")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// LimitJSONBody caps request bodies on state-changing methods. Oversized
// bodies surface as a read error inside handlers, which already return 4xx.
// Skipped for multipart uploads so file-attachment handlers can apply their
// own (larger) limit.
func LimitJSONBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
					r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

var (
	rateLimitersMu sync.Mutex
	rateLimiters   = map[string]*rate.Limiter{}
)

// RateLimit returns a middleware enforcing a token-bucket allowance keyed by
// name. The dashboard is single-user so a global limiter per route is
// sufficient; no per-IP slicing.
func RateLimit(name string, every time.Duration, burst int) func(http.Handler) http.Handler {
	rateLimitersMu.Lock()
	lim, ok := rateLimiters[name]
	if !ok {
		lim = rate.NewLimiter(rate.Every(every), burst)
		rateLimiters[name] = lim
	}
	rateLimitersMu.Unlock()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !lim.Allow() {
				http.Error(w, "rate limit exceeded, retry later", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
