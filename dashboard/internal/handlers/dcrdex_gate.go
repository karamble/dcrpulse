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

// dexAuthClient gates an action that needs the unlocked session: 409 while
// DCRDEX is locked, then 503 when the client is not up, in that order.
func dexAuthClient(w http.ResponseWriter) (string, *bisonw.Client, bool) {
	appPass, ok := rpc.DcrdexAppPass()
	if !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return "", nil, false
	}
	client, ok := dexClient(w)
	return appPass, client, ok
}
