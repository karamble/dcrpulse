package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"dcrpulse/internal/services"
)

// Opening and closing a panel.
//
// These three sit under /api, unlike the proxy they issue credentials for, and
// that is the whole design: /api requires same-origin and a dashboard session,
// so only the dashboard's own application - running at the dashboard's origin,
// with the user logged in - can mint a panel token. The page that receives one
// could never have asked for it itself.
//
// The order inside the mint matters. Everything that could fail happens before
// a token exists, and the payout address is pinned with the *game's* token
// while the page still has none. A panel is never opened for a game this host
// could not first tell where to pay the user.

// BisonrelayGamingUISessionHandler opens a panel.
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

	// Running, before anything else. A panel opened onto a game that is not
	// up would fail on every call with nothing to act on.
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

	payout, err := services.PinGamingPayoutAddress(r.Context(), game)
	if err != nil {
		// No token is issued. A page that could play without this host
		// having pinned where the winnings go is the one arrangement this
		// whole boundary exists to prevent.
		http.Error(w, err.Error(), http.StatusConflict)
		return
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
//
// Rotation rather than extension, so a token that leaked has a bounded life
// even while the panel stays open. The parent asks for this, because the parent
// is the thing holding the dashboard session; the page only ever receives the
// result over its message port.
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
