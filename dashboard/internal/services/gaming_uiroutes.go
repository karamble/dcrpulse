package services

import (
	"net/http"
	"time"
)

// What a game's own page is allowed to reach, and nothing else.
//
// This list is not defence in depth. It is the access control.
//
// A game plugin's HTTP guard accepts one bearer token for every route it has,
// including the one that hands out the seed it derives all its keys from. It
// cannot do better: the proxy authenticates as the game, so a request from a
// browser and a request from this host arrive identical. So whether a page can
// reach a route is decided here and only here, and anything not on this list
// must fail closed.
//
// Matching is exact path and exact method, and the URL sent upstream is built
// from the entry's own Path rather than from the request's - so traversal,
// encoded slashes and case tricks are not filtered, they are unrepresentable.

// GamingUIRoute is one route a page may reach.
type GamingUIRoute struct {
	Method string
	Path   string
	// Deadline bounds the call. Most are quick; the funding ones block
	// while a person decides whether to approve a payment in the dashboard,
	// and cutting those off would cancel the thing they were approving.
	Deadline time.Duration
	// Stream says the response is open-ended and must not be buffered.
	Stream bool
}

// gamingUIRoutes is the allowlist, by game.
//
// Per game rather than shared, because "what a page may do" is a statement
// about a particular game's API and not a general policy. A game added later
// reaches nothing until somebody writes its routes down here, which is the
// right default.
var gamingUIRoutes = map[string][]GamingUIRoute{
	"poker": {
		// Reading.
		{Method: http.MethodGet, Path: "/health", Deadline: 15 * time.Second},
		{Method: http.MethodGet, Path: "/tables", Deadline: 15 * time.Second},
		{Method: http.MethodGet, Path: "/table/hand", Deadline: 30 * time.Second},
		{Method: http.MethodGet, Path: "/table/ledger", Deadline: 30 * time.Second},
		{Method: http.MethodGet, Path: "/bond", Deadline: 15 * time.Second},
		{Method: http.MethodGet, Path: "/payout", Deadline: 15 * time.Second},
		{Method: http.MethodGet, Path: "/spend", Deadline: 15 * time.Second},

		// The stream. No deadline at all: it is supposed to stay open, and
		// a timeout here would look to a reader exactly like a table where
		// nothing is happening.
		{Method: http.MethodGet, Path: "/events", Stream: true},

		// Playing. A person's decision is the one input that comes from
		// outside the protocol, and this is where it enters.
		{Method: http.MethodPost, Path: "/table/act", Deadline: 30 * time.Second},
		{Method: http.MethodPost, Path: "/table/leave", Deadline: 30 * time.Second},

		// Asking the host for money. These do not move any: they ask, and
		// a person approves it in the dashboard behind the panel. The long
		// deadlines are how long the plugin is willing to wait for that
		// answer, and shortening them here would abandon a payment the
		// person was in the middle of approving.
		{Method: http.MethodPost, Path: "/table/fund", Deadline: 10 * time.Minute},
		{Method: http.MethodPost, Path: "/table/bond", Deadline: 10 * time.Minute},
		{Method: http.MethodPost, Path: "/bond/fund", Deadline: 45 * time.Minute},
	},
}

// Deliberately absent, and why. Each of these is reachable by this host with
// the game's own token; none is reachable by a page.
//
//	/identity/backup, /identity/restore
//	    Hands out, or replaces, the seed every key is derived from. Losing it
//	    strands a bond forever; leaking it is total. There is already a
//	    deliberate route to it in the dashboard, behind a session.
//
//	/payout/set, /table/deposit/set, /bond/set
//	    All three name where coin goes or where it already is. A page that
//	    could set a payout address is a page that could redirect winnings,
//	    which is exactly what injecting the address from the bound wallet
//	    account exists to prevent.
//
//	/table/refund, /bond/sweep
//	    Build and broadcast a self-signed reclaim. Deliberate host actions
//	    with real consequences, not a click in a proxied page.
//
//	/table/join
//	    Joining is an invitation decision, and the dashboard already has a
//	    route for it that reads the invitation itself.
//
//	/cmd
//	    One entry point for a whole command vocabulary. Allowlisting it
//	    would allowlist everything reachable through it, now and later.

// GamingUIRouteFor resolves a request against one game's allowlist.
//
// The returned route's Path is what the proxy must use to build the upstream
// URL. Callers must not pass the request's own path through.
func GamingUIRouteFor(game, method, path string) (GamingUIRoute, bool) {
	for _, r := range gamingUIRoutes[game] {
		if r.Method == method && r.Path == path {
			return r, true
		}
	}
	return GamingUIRoute{}, false
}

// GamingUIRoutesKnown reports whether a game has any page-reachable routes at
// all, which is how the proxy tells "no such game" from "that game's page may
// not do that".
func GamingUIRoutesKnown(game string) bool {
	return len(gamingUIRoutes[game]) > 0
}
