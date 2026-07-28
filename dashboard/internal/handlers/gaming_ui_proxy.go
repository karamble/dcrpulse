// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/services"
)

// The gaming sandbox has no route off this host, so this is the only way a
// browser reaches a game. The panel token arriving here is swapped for the
// game's own token, which the page never holds.
//
// The document is authenticated by the dashboard session cookie, which a frame
// navigation carries; the API calls under it are authenticated by the panel
// token, because the frame has an opaque origin and sends no cookies.

// ResponseHeaderTimeout is unset: the funding routes answer only once a person
// has approved a payment, minutes later.
var gamingSandbox = &http.Transport{
	DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
	MaxIdleConnsPerHost: 8,
	IdleConnTimeout:     90 * time.Second,
	// Uncompressed so a stream arrives as it is produced.
	DisableCompression: true,
}

var gamingUIGameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// GameUIDocumentHandler serves a game's own page into a sandboxed frame.
func GameUIDocumentHandler(w http.ResponseWriter, r *http.Request) {
	game := mux.Vars(r)["game"]
	if !gamingUIGameRe.MatchString(game) || !services.GamingUIRoutesKnown(game) {
		http.NotFound(w, r)
		return
	}
	base, err := services.GamingGameURL(game)
	if err != nil {
		gamingUIUpstreamError(w, err)
		return
	}
	token, ok := services.GamingGameToken(game)
	if !ok {
		http.NotFound(w, r)
		return
	}
	proxyDocumentTo(w, r, base, token)
}

// proxyDocumentTo fetches a game's page and returns it under a synthesized
// policy, with the upstream already resolved.
func proxyDocumentTo(w http.ResponseWriter, r *http.Request, base, token string) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/ui/", nil)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := gamingSandbox.RoundTrip(req)
	if err != nil {
		gamingUIUpstreamError(w, err)
		return
	}
	defer resp.Body.Close()

	// Buffered so the inline scripts can be hashed into a script-src.
	const maxDocument = 8 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDocument+1))
	if err != nil {
		gamingUIUpstreamError(w, err)
		return
	}
	if len(body) > maxDocument {
		log.Printf("gaming: a game served an interface larger than %d bytes", maxDocument)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	origin := middleware.ExternalOrigin(r)
	policy := gamingUIFramePolicy(origin, middleware.InlineScriptHashesFor(body))

	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", policy)
	// X-Frame-Options would override frame-ancestors in older browsers and
	// stop this being framed at all.
	h.Del("X-Frame-Options")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// gamingUIFramePolicy builds the policy for a page served into a sandboxed
// frame. Every source names the origin literally: 'self' matches nothing in an
// opaque origin.
func gamingUIFramePolicy(origin string, scriptHashes []string) string {
	script := "script-src"
	if len(scriptHashes) == 0 {
		script += " 'none'"
	}
	for _, h := range scriptHashes {
		script += " '" + h + "'"
	}
	return strings.Join([]string{
		"default-src 'none'",
		script,
		"style-src 'unsafe-inline'",
		"img-src data: blob: " + origin,
		"font-src data:",
		"connect-src " + origin,
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors " + origin,
		"sandbox allow-scripts",
	}, "; ")
}

// GameUIPreflightHandler answers the CORS preflight. It answers before any
// token check, because a preflight carries none, and identically for every
// path so it cannot be used to enumerate games or routes.
func GameUIPreflightHandler(w http.ResponseWriter, r *http.Request) {
	gamingUICORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
	_ = r
}

// gamingUICORS sets Allow-Origin '*' and never Allow-Credentials: a browser
// refuses '*' with credentials but accepts 'null' with them, and any page can
// produce a null origin. The bearer token is the whole authority here.
func gamingUICORS(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Vary", "Origin")
}

// GameUIAPIHandler proxies one allowlisted call to a game.
func GameUIAPIHandler(w http.ResponseWriter, r *http.Request) {
	gamingUICORS(w)

	game := mux.Vars(r)["game"]
	if !gamingUIGameRe.MatchString(game) {
		http.NotFound(w, r)
		return
	}

	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	session, ok := services.GamingUISessionFor(token)
	if !ok || session.Game != game {
		http.NotFound(w, r)
		return
	}

	// The asked path only looks a route up; the entry's own Path is what
	// goes upstream, so traversal has nothing to act on.
	asked := "/" + strings.TrimPrefix(mux.Vars(r)["rest"], "/")
	route, ok := services.GamingUIRouteFor(game, r.Method, asked)
	if !ok {
		http.NotFound(w, r)
		return
	}

	base, err := services.GamingGameURL(game)
	if err != nil {
		gamingUIUpstreamError(w, err)
		return
	}
	target, err := url.Parse(base)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	gameToken, ok := services.GamingGameToken(game)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// This subtree is not under the API router's body limit.
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	}

	ctx := r.Context()
	if route.Deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, route.Deadline)
		defer cancel()
	}
	query := r.URL.RawQuery

	proxy := &httputil.ReverseProxy{
		Transport: gamingSandbox,
		// Unconditional: a buffered stream looks like a quiet one.
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = target.Scheme
			pr.Out.URL.Host = target.Host
			pr.Out.URL.Path = route.Path
			pr.Out.URL.RawPath = ""
			pr.Out.URL.RawQuery = query
			pr.Out.Host = ""

			// Built up rather than filtered, so a header can only be
			// forwarded on purpose. Cookie above all: a default
			// ReverseProxy would hand the sandbox a live session.
			out := make(http.Header, 4)
			if ct := pr.In.Header.Get("Content-Type"); ct != "" {
				out.Set("Content-Type", ct)
			}
			if ac := pr.In.Header.Get("Accept"); ac != "" {
				out.Set("Accept", ac)
			}
			// Replaced, never merged: the panel token stops here.
			out.Set("Authorization", "Bearer "+gameToken)
			pr.Out.Header = out
		},
		ModifyResponse: func(resp *http.Response) error {
			// A Set-Cookie from the game would land on this origin.
			resp.Header.Del("Set-Cookie")
			resp.Header.Del("Content-Security-Policy")
			resp.Header.Del("Content-Security-Policy-Report-Only")
			resp.Header.Del("X-Frame-Options")
			// Deleted, not replaced: ReverseProxy adds these to the
			// writer, which already carries the one set below, and a
			// duplicated Allow-Origin fails CORS outright.
			resp.Header.Del("Access-Control-Allow-Origin")
			resp.Header.Del("Access-Control-Allow-Credentials")
			resp.Header.Set("Cache-Control", "no-store")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("gaming: %s %s: %v", game, route.Path, err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r.WithContext(ctx))
}

// gamingUIUpstreamError answers without describing this host.
func gamingUIUpstreamError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrGamingGameNotInstalled):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, services.ErrGamingGameNotRunning):
		http.Error(w, "the game is not running", http.StatusServiceUnavailable)
	default:
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
}
