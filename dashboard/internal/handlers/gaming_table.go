// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"dcrpulse/internal/gamingpb"
	"dcrpulse/internal/services"
)

// BisonrelayGamingCreateHandler proposes a table and puts it in a group chat.
func BisonrelayGamingCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game       string                    `json:"game"`
		GCID       string                    `json:"gcid"`
		BuyInAtoms int64                     `json:"buyinAtoms"`
		Funds      services.GamingTableFunds `json:"funds"`
		Seats      uint32                    `json:"seats"`
		// OpenBlocks is optional; zero takes the default.
		OpenBlocks uint32 `json:"openBlocks"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	game := strings.ToLower(strings.TrimSpace(req.Game))
	gcid := strings.ToLower(strings.TrimSpace(req.GCID))
	if game == "" {
		http.Error(w, "no game named", http.StatusBadRequest)
		return
	}
	if !services.ValidGamingGCID(gcid) {
		http.Error(w, "gcid must be 64 hex characters", http.StatusBadRequest)
		return
	}
	buyin := req.BuyInAtoms
	if buyin <= 0 {
		http.Error(w, "the buy-in is not an amount", http.StatusBadRequest)
		return
	}

	table, err := services.CreateGamingTable(r.Context(), game, gcid, uint64(buyin), req.Seats, req.OpenBlocks, req.Funds)
	if err != nil {
		gamingTableError(w, err)
		return
	}
	gamingJSON(w, table)
}

// BisonrelayGamingInviteHandler hands an accepted invitation to the game that
// can act on it.
//
// A browser route rather than one a game can reach: accepting is a person's
// decision, taken in their own session.
func BisonrelayGamingInviteHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game   string `json:"game"`
		Invite string `json:"invite"`
		GCID   string `json:"gcid"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	game := strings.ToLower(strings.TrimSpace(req.Game))
	gcid := strings.ToLower(strings.TrimSpace(req.GCID))
	if game == "" || strings.TrimSpace(req.Invite) == "" {
		http.Error(w, "game and invite are required", http.StatusBadRequest)
		return
	}
	if !services.ValidGamingGCID(gcid) {
		http.Error(w, "gcid must be 64 hex characters", http.StatusBadRequest)
		return
	}

	sid, err := services.AcceptGamingInvite(r.Context(), game, req.Invite, gcid)
	if err != nil {
		gamingTableError(w, err)
		return
	}
	gamingJSON(w, map[string]any{"accepted": true, "sid": sid})
}

// BisonrelayGamingStateHandler reports nonfinancial game state. Deposits and
// recovery are read exclusively from the bridge authority ledger.
func BisonrelayGamingStateHandler(w http.ResponseWriter, r *http.Request) {
	game := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("game")))
	if game == "" {
		http.Error(w, "no game named", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("refresh") == "1" {
		if err := services.RefreshGamingState(r.Context(), game); err != nil {
			gamingTableError(w, err)
			return
		}
	}
	state := services.GamingReportedState(game)
	if state == nil {
		// Never reported is not an error: a game that has not connected
		// since this bridge started has nothing to say yet.
		gamingJSON(w, map[string]any{"reported": false})
		return
	}
	var tip int64
	if chain, err := services.GamingChainTipNow(r.Context()); err == nil {
		tip = chain.Height
	}
	gamingJSON(w, gamingStateView(state, tip))
}

func gamingStateView(s *gamingpb.GameState, tip int64) map[string]any {
	tables := make([]map[string]any, 0, len(s.GetTables()))
	for _, t := range s.GetTables() {
		tables = append(tables, map[string]any{
			"sid":        t.GetSid(),
			"gcid":       t.GetGcid(),
			"state":      t.GetState(),
			"seats":      t.GetSeats(),
			"buyinAtoms": t.GetBuyinAtoms(),
			"until":      t.GetUntil(),
			"over":       t.GetOver(),
		})
	}
	return map[string]any{
		"reported":         true,
		"reportedAt":       s.GetReportedAt(),
		"tipHeight":        tip,
		"tables":           tables,
		"chainErr":         s.GetChainErr(),
		"seedAcknowledged": s.GetSeedBackupAcknowledged(),
	}
}

// gamingTableError separates what the operator can act on from what they cannot:
// a game they never registered, one that is simply not running, and a refusal
// the game itself issued.
func gamingTableError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrGamingGameNotRegistered):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, services.ErrGamingGameNotConnected):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}
