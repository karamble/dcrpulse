// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC license.
package handlers

import (
	"context"
	"dcrpulse/internal/services"
	"encoding/json"
	"net/http"
	"time"
)

func JoinDecredPulseHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	var req struct {
		Restart bool `json:"restart"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid join request", http.StatusBadRequest)
			return
		}
	}
	joined, err := services.BeginCommunityJoin(ctx, req.Restart)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, joined)
}
func CommunityJoinStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	joined, err := services.CommunityJoinStatus(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, joined)
}
func CommunityJoinAcceptHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "join id is required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := services.AcceptCommunityJoin(ctx, req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
