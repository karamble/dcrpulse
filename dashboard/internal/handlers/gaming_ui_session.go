// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"dcrpulse/internal/services"
)

// These sit under /api so same-origin and a dashboard session are what mint a
// panel token; the page that receives one could not have asked for it.

// BisonrelayGamingUISessionHandler opens a game panel. Everything that can fail
// happens before a token exists.
func BisonrelayGamingUISessionHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game    string `json:"game"`
		TableID string `json:"tableId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	game := strings.ToLower(strings.TrimSpace(req.Game))
	if !gamingUIGameRe.MatchString(game) || !services.GamingUIRoutesKnown(game) {
		http.Error(w, "no such game", http.StatusNotFound)
		return
	}

	if _, err := services.GamingGameURL(game); err != nil {
		switch {
		case errors.Is(err, services.ErrGamingGameNotInstalled):
			http.Error(w, "that game is not installed", http.StatusNotFound)
		case errors.Is(err, services.ErrGamingGameNotRunning):
			http.Error(w, "that game is not running yet", http.StatusServiceUnavailable)
		default:
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}
		return
	}

	// No token is issued unless the host has first pinned where winnings go.
	payout, err := services.PinGamingPayoutAddress(r.Context(), game)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	// Names for the chairs, best-effort and after the money question: a
	// panel that opens with numbered seats is fine, a panel that cannot say
	// where winnings go is not.
	if err := services.PushGamingNames(r.Context(), game); err != nil {
		log.Printf("gaming names: %v", err)
	}

	token, session, err := services.MintGamingUISession(game, strings.TrimSpace(req.TableID), payout)
	if err != nil {
		http.Error(w, "could not open a panel", http.StatusInternalServerError)
		return
	}

	gamingJSON(w, map[string]any{
		"token":     token,
		"expiresAt": session.Expires.UTC().Format("2006-01-02T15:04:05Z"),
		"uiUrl":     "/gameui/" + game + "/",
		"apiBase":   "/gameui/" + game + "/api",
		"tableId":   session.TableID,
		"payout":    session.PayoutAddress,
	})
}

// BisonrelayGamingUIRefreshHandler rotates a panel's token.
func BisonrelayGamingUIRefreshHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	token, session, ok := services.RotateGamingUISession(strings.TrimSpace(req.Token))
	if !ok {
		http.Error(w, "that panel is not open", http.StatusNotFound)
		return
	}
	gamingJSON(w, map[string]any{
		"token":     token,
		"expiresAt": session.Expires.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

// BisonrelayGamingUIEndHandler closes a panel.
func BisonrelayGamingUIEndHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	services.RevokeGamingUISession(strings.TrimSpace(req.Token))
	gamingJSON(w, map[string]any{"closed": true})
}
