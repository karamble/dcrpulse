// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"net/http"
	"time"
)

// A game plugin authenticates every one of its routes with the same bearer
// token and cannot tell a proxied browser from this host, so this list is the
// access control rather than defence in depth. Anything absent must fail closed.

// GamingUIRoute is one route a game's page may reach.
type GamingUIRoute struct {
	Method string
	Path   string
	// Deadline bounds the call; the funding routes block while a person
	// approves a payment in the dashboard.
	Deadline time.Duration
	// Stream marks an open-ended response that must not be buffered.
	Stream bool
}

// gamingUIRoutes is the allowlist, per game. A game with no entry reaches
// nothing.
var gamingUIRoutes = map[string][]GamingUIRoute{
	"poker": {
		{Method: http.MethodGet, Path: "/health", Deadline: 15 * time.Second},
		{Method: http.MethodGet, Path: "/tables", Deadline: 15 * time.Second},
		{Method: http.MethodGet, Path: "/table/hand", Deadline: 30 * time.Second},
		{Method: http.MethodGet, Path: "/table/ledger", Deadline: 30 * time.Second},
		{Method: http.MethodGet, Path: "/table/log", Deadline: 30 * time.Second},
		{Method: http.MethodGet, Path: "/bond", Deadline: 15 * time.Second},
		{Method: http.MethodGet, Path: "/payout", Deadline: 15 * time.Second},
		{Method: http.MethodGet, Path: "/spend", Deadline: 15 * time.Second},

		{Method: http.MethodGet, Path: "/events", Stream: true},

		{Method: http.MethodPost, Path: "/table/act", Deadline: 30 * time.Second},
		{Method: http.MethodPost, Path: "/table/leave", Deadline: 30 * time.Second},
		{Method: http.MethodPost, Path: "/table/challenge", Deadline: 30 * time.Second},

		{Method: http.MethodPost, Path: "/table/fund", Deadline: 10 * time.Minute},
		{Method: http.MethodPost, Path: "/table/bond", Deadline: 10 * time.Minute},
		{Method: http.MethodPost, Path: "/bond/fund", Deadline: 45 * time.Minute},
	},
}

// Absent on purpose, host-only: /identity/backup, /identity/restore,
// /payout/set, /table/deposit/set, /bond/set, /table/refund, /bond/sweep,
// /table/bond/sweep, /table/join, /cmd.
//
// The three reclaim routes are host-only for one reason: each builds and
// broadcasts a transaction the game signed itself, and a page that can cause a
// broadcast is a larger thing than a page that can read. Where to send the coin
// is named by the host, never by the game.

// GamingUIRouteFor resolves a request against one game's allowlist. The
// returned Path, not the request's, is what the proxy must send upstream.
func GamingUIRouteFor(game, method, path string) (GamingUIRoute, bool) {
	for _, r := range gamingUIRoutes[game] {
		if r.Method == method && r.Path == path {
			return r, true
		}
	}
	return GamingUIRoute{}, false
}

// GamingUIRoutesKnown reports whether a game has any page-reachable routes.
func GamingUIRoutesKnown(game string) bool {
	return len(gamingUIRoutes[game]) > 0
}
