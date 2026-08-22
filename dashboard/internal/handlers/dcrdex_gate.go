// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"

	"dcrpulse/internal/rpc"
	"dcrpulse/pkg/bisonw"
)

// dexClient answers 503 and reports false when bisonw's RPC client is not up.
func dexClient(w http.ResponseWriter) (*bisonw.Client, bool) {
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return nil, false
	}
	return client, true
}

// dexUnlockedClient gates an RPC action that needs bisonw's core logged in:
// 409 while no session is up, then 503 when the RPC client is not up. The
// dashboard holds no app password, so the webserver session is the unlock
// state; bisonw's core is shared by both servers, so that login satisfies the
// RPC routes too.
func dexUnlockedClient(w http.ResponseWriter) (*bisonw.Client, bool) {
	if !rpc.DcrdexUnlocked() {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return nil, false
	}
	return dexClient(w)
}

// dexWebSession gates an action that needs the unlocked session: 409 while no
// webserver session is up, then 503 when the web client is not up, in that
// order.
func dexWebSession(w http.ResponseWriter) (*bisonw.WebClient, bool) {
	if !rpc.DcrdexUnlocked() {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return nil, false
	}
	web, err := rpc.DcrdexWebClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return nil, false
	}
	return web, true
}
