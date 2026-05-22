// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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

// BisonrelayIdentityHandler returns brclientd's local BR identity payload
// (nick + zkidentity public keys) by proxying ChatService.UserPublicIdentity.
// 502 if brclientd is unreachable or has not yet reached the ready stage.
func BisonrelayIdentityHandler(w http.ResponseWriter, r *http.Request) {
	id, err := rpc.BrclientdUserPublicIdentity(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(id)
}

// BisonrelayMessagesHandler proxies brclientd's /history/pm endpoint. Query
// params: contact (hex peer UID, required), page (default 0), page_size
// (default 50, max 500). Returns the raw JSON envelope brclientd produces so
// the dashboard stays stateless w.r.t. chat history.
func BisonrelayMessagesHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	contact := strings.TrimSpace(q.Get("contact"))
	if contact == "" {
		http.Error(w, "contact query param is required", http.StatusBadRequest)
		return
	}
	page := 0
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			page = n
		}
	}
	pageSize := 50
	if v := q.Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			pageSize = n
		}
	}
	body, err := rpc.BrclientdHistoryPM(r.Context(), contact, page, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelaySetupHandler proxies a nick/name pair to brclientd's pre-setup
// /create-identity endpoint. The frontend wizard only calls this when
// /api/br/status reports stage=needs-identity; outside that window
// brclientd's port is owned by clientrpc and the call 404s.
func BisonrelaySetupHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nick string `json:"nick"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Nick = strings.TrimSpace(req.Nick)
	req.Name = strings.TrimSpace(req.Name)
	if req.Nick == "" {
		http.Error(w, "nick is required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdCreateIdentity(r.Context(), req.Nick, req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
