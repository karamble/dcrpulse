// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"net/http"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

func torWriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// GetTorHandler returns the current Tor toggle settings.
func GetTorHandler(w http.ResponseWriter, r *http.Request) {
	torWriteJSON(w, services.ReadTorSettings())
}

// SetTorHandler persists Tor settings, bumping the rev so the supervisors
// relaunch their daemons with or without the proxy flags.
func SetTorHandler(w http.ResponseWriter, r *http.Request) {
	var req types.TorSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	out, err := services.WriteTorSettings(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	torWriteJSON(w, out)
}

// GetTorStatusHandler returns proxy reachability, per-daemon routing state, and
// the dcrd onion address.
func GetTorStatusHandler(w http.ResponseWriter, r *http.Request) {
	torWriteJSON(w, services.TorStatusSnapshot())
}

// GetTorControlHandler returns live data from the Tor control port.
func GetTorControlHandler(w http.ResponseWriter, r *http.Request) {
	torWriteJSON(w, services.TorControlSnapshot())
}

// TorNewIdentityHandler signals Tor to build fresh circuits (NEWNYM).
func TorNewIdentityHandler(w http.ResponseWriter, r *http.Request) {
	if err := services.TorNewIdentity(); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
