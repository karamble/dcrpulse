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

// Letting a browser reach a game, without letting it reach anything else.
//
// The gaming sandbox is on a network with no route off this host, which is what
// makes an untrusted game safe to run beside a wallet. A page served by that
// game therefore cannot be fetched directly by a browser, and this is the only
// door.
//
// It is a translating door, and the translation is the point. On the way in it
// takes a short-lived token that means "an open panel in this dashboard" and
// swaps it for the game's own token, which means "this game" and authorizes
// spending. The page never holds the second one. On the way out it strips
// anything the game says about cookies or framing, because the game is
// untrusted and those are this host's business.
//
// Two routes with two different kinds of authentication, which is exactly why
// they are not under /api and not under /gaming:
//
//	GET /gameui/{game}/          the document, authenticated by the dashboard
//	                             session cookie, which a frame navigation
//	                             carries because it is same-site
//	*   /gameui/{game}/api/...   the game's API, authenticated by the panel
//	                             token, because the frame has an opaque origin
//	                             and sends no cookies at all
//
// /api would 403 the second on Origin: null and would want a cookie that will
// never arrive; /gaming wants a game token and points the other way.

// gamingSandbox is the one transport for every proxied call, so connections are
// pooled rather than a new one per request.
//
// ResponseHeaderTimeout is deliberately unset. /table/fund and /bond/fund
// answer only when a person has decided whether to approve a payment, which is
// minutes away, and a header timeout would cancel the request they were in the
// middle of approving.
var gamingSandbox = &http.Transport{
	DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
	MaxIdleConnsPerHost: 8,
	IdleConnTimeout:     90 * time.Second,
	// So a stream arrives as it is produced rather than in whatever chunks
	// a compressor decides to emit.
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

// proxyDocumentTo is the fetch, the hashing and the policy, with the upstream
// already decided.
//
// Split out so it can be driven against a stand-in game. Which game runs on
// which port comes from the portal's own state file and is resolved above; what
// happens to the headers is this, and it is the part worth pinning.
func proxyDocumentTo(w http.ResponseWriter, r *http.Request, base, token string) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/ui/", nil)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	// The document route on the plugin is unauthenticated, but the token is
	// sent anyway so this one call does not become the exception that makes
	// somebody wonder whether the rest are authenticated.
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := gamingSandbox.RoundTrip(req)
	if err != nil {
		gamingUIUpstreamError(w, err)
		return
	}
	defer resp.Body.Close()

	// Buffered, unlike everything else here, for two reasons that both
	// require having the whole document: hashing its inline scripts into a
	// script-src, and refusing to serve one large enough to be a problem.
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
	// SecurityHeaders set the dashboard's own policy on the way in. This is
	// a different document with a different origin, so it gets a different
	// one - the same override the dcrtime worker already uses.
	h.Set("Content-Security-Policy", policy)
	// Never X-Frame-Options: it is the whole point that this is framed, and
	// in older browsers it would override frame-ancestors and break it.
	h.Del("X-Frame-Options")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// gamingUIFramePolicy is the policy a page with an opaque origin needs.
//
// 'self' matches nothing in an opaque origin, so a policy copied from the
// dashboard's would produce a page that can fetch nothing and cannot even be
// framed - and the failure would look exactly like this proxy being broken.
// Every source has to name the origin literally.
//
// The sandbox directive is stated here as well as on the frame element. The
// element is what actually applies it; repeating it means a document that
// somehow got loaded outside a frame is still confined.
func gamingUIFramePolicy(origin string, scriptHashes []string) string {
	script := "script-src"
	if len(scriptHashes) == 0 {
		// Nothing inline to allow. A page with no script is still a page.
		script += " 'none'"
	}
	for _, h := range scriptHashes {
		script += " '" + h + "'"
	}
	return strings.Join([]string{
		"default-src 'none'",
		script,
		// The bundle is one document with its styles inlined, and there is
		// no origin it could load a stylesheet from anyway.
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

// GameUIPreflightHandler answers the CORS preflight every call from the frame
// makes.
//
// It answers before any token is looked at, because a preflight carries no
// Authorization header - that is what it is asking permission to send. And it
// answers identically for any well-formed path, so it cannot be used to
// enumerate which games are installed or which routes exist.
func GameUIPreflightHandler(w http.ResponseWriter, r *http.Request) {
	gamingUICORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
	_ = r
}

// gamingUICORS is the one place these headers are set.
//
// Allow-Origin is '*' and Allow-Credentials is never set, and the pair is
// deliberate. A browser refuses '*' together with credentials, but accepts
// 'null' with them - and 'null' is an origin any page can produce by opening a
// sandboxed frame of its own, which is the textbook way this arrangement gets
// broken. Choosing '*' makes that mistake unrepresentable.
//
// The consequence, stated plainly: on this subtree the bearer token is the
// entire authority. That is why it is narrow, short-lived, revocable, and
// bounded by an allowlist.
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
		// Not found rather than unauthorized, matching how the gaming
		// tunnel answers: there is no reason to describe this host to
		// somebody holding nothing.
		http.NotFound(w, r)
		return
	}

	// The path the caller asked for is used to *look up* a route and never
	// to build one. What goes upstream is the allowlist entry's own string,
	// so traversal and encoded separators have nothing to act on.
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

	// A body cap, because this subtree is not under the API router's.
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
		// Unconditional, rather than trusting content-type sniffing to
		// notice a stream. A buffered stream is indistinguishable from a
		// table where nothing is happening.
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = target.Scheme
			pr.Out.URL.Host = target.Host
			pr.Out.URL.Path = route.Path
			// Cleared, not left over. RawPath is what actually goes on
			// the wire when it is a valid encoding of Path, and one
			// carried in from the request would be an encoding of a
			// path this proxy just decided not to use.
			pr.Out.URL.RawPath = ""
			pr.Out.URL.RawQuery = query
			pr.Out.Host = ""

			// Built from nothing rather than filtered. The header that
			// matters most is Cookie: a default ReverseProxy forwards it
			// verbatim, which would hand the sandbox a live dashboard
			// session and make the isolated network decorative. Building
			// up means a header can only be forwarded on purpose.
			out := make(http.Header, 4)
			if ct := pr.In.Header.Get("Content-Type"); ct != "" {
				out.Set("Content-Type", ct)
			}
			if ac := pr.In.Header.Get("Accept"); ac != "" {
				out.Set("Accept", ac)
			}
			// Replaced, never merged: the panel token goes no further
			// than this function.
			out.Set("Authorization", "Bearer "+gameToken)
			pr.Out.Header = out
		},
		ModifyResponse: func(resp *http.Response) error {
			// The game is untrusted and these are this host's to decide.
			// A Set-Cookie from it would land on the dashboard's own
			// origin, including one that shadows the session cookie.
			resp.Header.Del("Set-Cookie")
			resp.Header.Del("Content-Security-Policy")
			resp.Header.Del("Content-Security-Policy-Report-Only")
			resp.Header.Del("X-Frame-Options")
			resp.Header.Del("Access-Control-Allow-Origin")
			resp.Header.Del("Access-Control-Allow-Credentials")
			resp.Header.Set("Access-Control-Allow-Origin", "*")
			resp.Header.Set("Cache-Control", "no-store")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				// The panel closed, or the person navigated away. Not
				// worth a line, and there is nobody to answer.
				return
			}
			log.Printf("gaming: %s %s: %v", game, route.Path, err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r.WithContext(ctx))
}

// gamingUIUpstreamError says what went wrong without describing this host.
//
// Installed and running are different answers on purpose: a game that is added
// but not up is something the user can act on, and a game that is not installed
// is not something to confirm the existence of.
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
