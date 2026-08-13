// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/services"
)

var gamingGCIDRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// BisonrelayGamingCreateHandler proposes a table and puts it in a group chat.
func BisonrelayGamingCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game     string  `json:"game"`
		GCID     string  `json:"gcid"`
		BuyInDcr float64 `json:"buyinDcr"`
		Seats    uint32  `json:"seats"`
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
	if !gamingGCIDRe.MatchString(gcid) {
		http.Error(w, "gcid must be 64 hex characters", http.StatusBadRequest)
		return
	}
	buyin, err := dcrutil.NewAmount(req.BuyInDcr)
	if err != nil || buyin <= 0 {
		http.Error(w, "the buy-in is not an amount", http.StatusBadRequest)
		return
	}

	table, err := services.CreateGamingTable(r.Context(), game, gcid, uint64(buyin), req.Seats, req.OpenBlocks)
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
	if !gamingGCIDRe.MatchString(gcid) {
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
