// Package middleware provides HTTP middleware used by the dashboard.
package middleware

import (
	"crypto/sha256"
	"encoding/base64"
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

// demoReadOnlyPosts are the few non-GET routes that are actually read-only
// (estimates, invoice decoding, Politeia polling, the login/logout handshake)
// and so must stay reachable while demo mode blocks every other write.
var demoReadOnlyPosts = map[string]bool{
	"/api/auth/login":                          true,
	"/api/auth/logout":                         true,
	"/api/dcrdex/preorder":                     true,
	"/api/dcrdex/maxbuy":                       true,
	"/api/dcrdex/maxsell":                      true,
	"/api/wallet/ln/send/decode":               true,
	"/api/wallet/governance/proposals/refresh": true,
}

// demoBlockedGets are GET routes that nonetheless perform a fund-moving or
// otherwise unsafe action and must be blocked in demo mode even though a
// method-based filter would let them through. /api/wallet/ln/send is a
// WebSocket upgrade that sends a Lightning payment.
var demoBlockedGets = map[string]bool{
	"/api/wallet/ln/send": true,
}

// demoAllowed reports whether a request may proceed while demo mode is on.
func demoAllowed(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return !demoBlockedGets[r.URL.Path]
	default:
		if demoReadOnlyPosts[r.URL.Path] {
			return true
		}
		// Politeia per-proposal refresh carries a {token} path variable.
		return strings.HasPrefix(r.URL.Path, "/api/wallet/governance/proposals/") &&
			strings.HasSuffix(r.URL.Path, "/refresh")
	}
}

// DemoBlocker makes the API read-only when enabled: it rejects every
// state-changing request except an allowlist of read-only POSTs, plus a small
// set of fund-moving GET endpoints. Blocked requests get 403 with a stable
// {"error":"demo_disabled"} body the frontend keys on to show its demo modal.
// When disabled it adds zero overhead (returns the handler unwrapped).
func DemoBlocker(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if demoAllowed(r) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"demo_disabled","message":"This action is disabled in the demo."}`))
		})
	}
}
