// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"dcrpulse/internal/rpc"
)

// DcrdexStatus reports reachability and versions of the backend-only bisonw
// daemon. Richer onboarding state (initialized/logged-in/registered) is added
// alongside the init/login routes.
type DcrdexStatus struct {
	Reachable     bool   `json:"reachable"`
	Unlocked      bool   `json:"unlocked"`
	BisonwVersion string `json:"bisonwVersion,omitempty"`
	RPCServerVer  string `json:"rpcServerVersion,omitempty"`
	Error         string `json:"error,omitempty"`
}

// GetDcrdexStatusHandler reports whether the bisonw RPC server is reachable and
// its versions, via the version route (no app initialization required).
func GetDcrdexStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	client, err := rpc.DcrdexClient()
	if err != nil {
		json.NewEncoder(w).Encode(DcrdexStatus{Reachable: false, Error: err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	v, err := client.Version(ctx)
	if err != nil {
		json.NewEncoder(w).Encode(DcrdexStatus{Reachable: false, Error: err.Error()})
		return
	}

	status := DcrdexStatus{Reachable: true}
	_, status.Unlocked = rpc.DcrdexAppPass()
	if v.Bisonw != nil {
		status.BisonwVersion = v.Bisonw.VersionString
	}
	if v.RPCServerVersion != nil {
		status.RPCServerVer = formatSemver(v.RPCServerVersion.Major, v.RPCServerVersion.Minor, v.RPCServerVersion.Patch)
	}
	json.NewEncoder(w).Encode(status)
}

func formatSemver(major, minor, patch uint32) string {
	return itoa(major) + "." + itoa(minor) + "." + itoa(patch)
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

type dcrdexAuthRequest struct {
	AppPass string `json:"appPass"`
	Seed    string `json:"seed,omitempty"`
}

// InitDcrdexHandler initializes the bisonw client with a user-supplied app
// password (optionally restoring from a seed), logs in, and holds the password
// in memory for the session. The password is never persisted.
func InitDcrdexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req dcrdexAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AppPass == "" {
		http.Error(w, "appPass is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.Init(ctx, req.AppPass, req.Seed); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if _, err := client.Login(ctx, req.AppPass); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	rpc.SetDcrdexAppPass(req.AppPass)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// UnlockDcrdexHandler logs the bisonw client in with the supplied app password
// and holds it in memory for the session (used after a restart re-locks it).
func UnlockDcrdexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req dcrdexAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AppPass == "" {
		http.Error(w, "appPass is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := client.Login(ctx, req.AppPass); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	rpc.SetDcrdexAppPass(req.AppPass)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// LockDcrdexHandler logs the bisonw client out and forgets the in-memory app
// password.
func LockDcrdexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if client, err := rpc.DcrdexClient(); err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		client.Logout(ctx)
	}
	rpc.ClearDcrdexAppPass()
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
