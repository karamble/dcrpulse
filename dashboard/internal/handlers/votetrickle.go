// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/services"
)

// StartVoteTrickleHandler signs a proposal's eligible votes up front (the wallet
// is re-locked before this returns) and launches the background vote-trickle
// worker that submits them spread over the given duration.
func StartVoteTrickleHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token           string `json:"token"`
		VoteOption      string `json:"voteOption"`
		DurationSeconds int64  `json:"durationSeconds"`
		Bunches         int    `json:"bunches"`
		Passphrase      string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Token == "" || req.VoteOption == "" {
		http.Error(w, "token and voteOption required", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	if req.DurationSeconds <= 0 {
		http.Error(w, "durationSeconds must be > 0", http.StatusBadRequest)
		return
	}

	passphrase := []byte(req.Passphrase)
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	// Signing every eligible ticket up front can take a while; allow for it.
	// (The worker itself then runs detached in the background.)
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	if err := services.StartVoteTrickle(ctx, req.Token, req.VoteOption,
		time.Duration(req.DurationSeconds)*time.Second, req.Bunches, passphrase); err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
			http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		case strings.Contains(lower, "already running"):
			http.Error(w, msg, http.StatusConflict)
		default:
			log.Printf("StartVoteTrickle failed: %v", err)
			http.Error(w, msg, http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// StopVoteTrickleHandler stops a running proposal's trickle or dismisses a
// finished one (idempotent). The proposal token is the ?token= query parameter.
func StopVoteTrickleHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}
	services.StopVoteTrickle(token)
	w.WriteHeader(http.StatusNoContent)
}

// VoteTrickleStatusHandler returns the live status of every trickle run (one per
// proposal currently or recently trickling).
func VoteTrickleStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(services.VoteTrickleWorkersSnapshot())
}

// StreamVoteTrickleEventsHandler upgrades to WebSocket and streams events
// (replays the last 200, then live).
func StreamVoteTrickleEventsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: middleware.SameOriginWS,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade votetrickle-events WebSocket: %v", err)
		return
	}
	defer conn.Close()

	for _, ev := range services.LastVoteTrickleEvents(200) {
		if err := conn.WriteJSON(ev); err != nil {
			return
		}
	}

	ch, unsubscribe := services.SubscribeVoteTrickleEvents()
	defer unsubscribe()

	notify := make(chan struct{})
	go func() {
		defer close(notify)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		case <-notify:
			return
		}
	}
}
