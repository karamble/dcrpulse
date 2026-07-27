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
	"regexp"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/gorilla/websocket"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// The Bison Relay gaming section's confinement policy (Bison Relay > Gaming).
// Games are untrusted plugins running beside a wallet, dcrlnd and a BR
// identity, so they never hold wallet credentials: they reach one account,
// under caps, through this policy. Storage speaks atoms; the frontend speaks
// DCR, converted here the same way the BR-MCP handlers do.

// gamingSettingsView is the DCR-denominated frontend shape.
type gamingSettingsView struct {
	Enabled             bool     `json:"enabled"`
	Account             string   `json:"account"`
	Mode                string   `json:"mode"`
	PerTableCapDcr      float64  `json:"perTableCapDcr"`
	PerDayCapDcr        float64  `json:"perDayCapDcr"`
	MaxOpenTables       int      `json:"maxOpenTables"`
	ApprovalTimeoutSecs int      `json:"approvalTimeoutSecs"`
	InstalledGames      []string `json:"installedGames"`
}

func gamingToView(s types.GamingSettings) gamingSettingsView {
	if s.InstalledGames == nil {
		s.InstalledGames = []string{}
	}
	return gamingSettingsView{
		Enabled:             s.Enabled,
		Account:             s.Account,
		Mode:                s.Mode,
		PerTableCapDcr:      dcrutil.Amount(s.PerTableCapAtoms).ToCoin(),
		PerDayCapDcr:        dcrutil.Amount(s.PerDayCapAtoms).ToCoin(),
		MaxOpenTables:       s.MaxOpenTables,
		ApprovalTimeoutSecs: s.ApprovalTimeoutSecs,
		InstalledGames:      s.InstalledGames,
	}
}

func gamingFromView(v gamingSettingsView) (types.GamingSettings, error) {
	perTable, err := dcrutil.NewAmount(v.PerTableCapDcr)
	if err != nil {
		return types.GamingSettings{}, err
	}
	perDay, err := dcrutil.NewAmount(v.PerDayCapDcr)
	if err != nil {
		return types.GamingSettings{}, err
	}
	return types.GamingSettings{
		Enabled:             v.Enabled,
		Account:             v.Account,
		Mode:                v.Mode,
		PerTableCapAtoms:    int64(perTable),
		PerDayCapAtoms:      int64(perDay),
		MaxOpenTables:       v.MaxOpenTables,
		ApprovalTimeoutSecs: v.ApprovalTimeoutSecs,
		InstalledGames:      v.InstalledGames,
	}, nil
}

func gamingJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// BisonrelayGamingSettingsHandler round-trips the gaming confinement policy.
func BisonrelayGamingSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		gamingJSON(w, gamingToView(services.ReadGamingSettings()))
	case http.MethodPost:
		var in gamingSettingsView
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		next, err := gamingFromView(in)
		if err != nil {
			http.Error(w, "bad amount: "+err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := services.WriteGamingSettings(next)
		if err != nil {
			http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		gamingJSON(w, gamingToView(saved))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayGamingGamesHandler lists the games this build can route, marked
// with whether the user installed them.
func BisonrelayGamingGamesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gamingJSON(w, map[string]any{"games": services.GamingCatalogue()})
}

// ---- The tunnel ----
//
// A game sends and receives its own protocol frames through these two routes.
// The host carries them and reads only the routing key: what a frame means is
// between the players, who sign their own traffic and check each other's.
//
// Both sit on the same authenticated API as everything else. A game running as
// a separate process in the stack therefore needs a credential of its own,
// which does not exist yet - plugin authentication is unresolved, and until it
// is, only something already holding a dashboard session can use the tunnel.

var gamingGCIDRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// BisonrelayGamingSendHandler sends one frame to a table's group chat.
func BisonrelayGamingSendHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game  string `json:"game"`
		GCID  string `json:"gcid"`
		Frame string `json:"frame"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !gamingGCIDRe.MatchString(req.GCID) {
		http.Error(w, "gcid must be 64 hex characters", http.StatusBadRequest)
		return
	}
	if req.Game == "" || req.Frame == "" {
		http.Error(w, "game and frame are required", http.StatusBadRequest)
		return
	}

	switch err := services.SendGamingFrame(r.Context(), req.Game, req.GCID, req.Frame); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, services.ErrGamingGameNotInstalled),
		errors.Is(err, services.ErrGamingNotAFrame),
		errors.Is(err, services.ErrGamingWrongGame):
		// A game is told it was refused, and nothing about what else the
		// host is carrying.
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		http.Error(w, "send frame: "+err.Error(), http.StatusBadGateway)
	}
}

// BisonrelayGamingEventsHandler streams one game's inbound frames.
//
// The subscription is per game, so a game never sees another's traffic. That is
// containment rather than secrecy - frames cross a group chat any member can
// read - but nothing is served to a game that was not addressed to it.
func BisonrelayGamingEventsHandler(w http.ResponseWriter, r *http.Request) {
	game := r.URL.Query().Get("game")
	if game == "" {
		http.Error(w, "game is required", http.StatusBadRequest)
		return
	}

	upgrader := websocket.Upgrader{CheckOrigin: middleware.SameOriginWS}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("BisonrelayGamingEventsHandler upgrade: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	frames, unsubscribe := services.Gaming().Subscribe(game, 64)
	defer unsubscribe()

	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	pinger := time.NewTicker(30 * time.Second)
	defer pinger.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-pinger.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case ev, ok := <-frames:
			if !ok {
				return
			}
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		}
	}
}
