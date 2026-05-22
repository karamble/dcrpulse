// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"net/http"

	"dcrpulse/internal/rpc"
)

// BisonrelayVersionHandler proxies brclientd's VersionService.Version
// through to the dashboard's HTTP API. Returns the brclientd
// appName / appVersion / goRuntime triple as JSON, or 502 if brclientd
// is unreachable.
func BisonrelayVersionHandler(w http.ResponseWriter, r *http.Request) {
	ver, err := rpc.BrclientdVersion(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ver)
}

// BisonrelayStatusHandler proxies brclientd's /status endpoint, returning
// the current stage, server LN node, and the most recent CheckLNWalletUsable
// error verbatim so the UI can render it.
func BisonrelayStatusHandler(w http.ResponseWriter, r *http.Request) {
	status, err := rpc.BrclientdStatus(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
