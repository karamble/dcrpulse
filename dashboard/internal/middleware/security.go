// Package middleware provides HTTP middleware used by the dashboard.
package middleware

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const csp = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"font-src 'self'; " +
	"connect-src 'self' ws: wss:; " +
	"frame-ancestors 'self'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

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
func LimitJSONBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
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
