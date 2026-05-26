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
