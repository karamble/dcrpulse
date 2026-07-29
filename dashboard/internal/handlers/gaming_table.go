// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/services"
)

// BisonrelayGamingCreateHandler proposes a table and puts it in a group chat.
func BisonrelayGamingCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
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
		switch {
		case errors.Is(err, services.ErrGamingGameNotInstalled):
			http.Error(w, err.Error(), http.StatusForbidden)
		case errors.Is(err, services.ErrGamingGameNotRunning):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	gamingJSON(w, table)
}

// BisonrelayGamingBondHandler reports a game's standing bond.
func BisonrelayGamingBondHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	game := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("game")))
	if game == "" {
		http.Error(w, "no game named", http.StatusBadRequest)
		return
	}
	bond, err := services.GamingBondStatus(r.Context(), game)
	if err != nil {
		gamingReclaimError(w, err)
		return
	}
	gamingJSON(w, bond)
}

// BisonrelayGamingTableBondsHandler lists what a game still holds locked at
// tables.
//
// Its own route rather than a field on the bond: these outlive the tables they
// belong to by a week, so by the time one matures the table is long finished and
// is not where anybody would look for it.
func BisonrelayGamingTableBondsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	game := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("game")))
	if game == "" {
		http.Error(w, "no game named", http.StatusBadRequest)
		return
	}
	bonds, err := services.GamingTableBonds(r.Context(), game)
	if err != nil {
		gamingReclaimError(w, err)
		return
	}
	gamingJSON(w, map[string]any{"bonds": bonds})
}

// BisonrelayGamingReclaimHandler takes back coin a game locked, into the bound
// account.
func BisonrelayGamingReclaimHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Game string `json:"game"`
		Kind string `json:"kind"`
		SID  string `json:"sid"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	game := strings.ToLower(strings.TrimSpace(req.Game))
	if game == "" {
		http.Error(w, "no game named", http.StatusBadRequest)
		return
	}

	var txid string
	var err error
	switch strings.ToLower(strings.TrimSpace(req.Kind)) {
	case "bond":
		txid, err = services.ReclaimGamingBond(r.Context(), game)
	case "stake":
		sid := strings.ToLower(strings.TrimSpace(req.SID))
		if sid == "" {
			http.Error(w, "no table named", http.StatusBadRequest)
			return
		}
		txid, err = services.ReclaimGamingStake(r.Context(), game, sid)
	case "tablebond":
		// A table's forfeitable bond, which is not the standing one above:
		// that buys the right to join and is never forfeitable, this is
		// what a seat loses for walking out of a hand. Different key,
		// different lock, so it cannot share a branch with either.
		sid := strings.ToLower(strings.TrimSpace(req.SID))
		if sid == "" {
			http.Error(w, "no table named", http.StatusBadRequest)
			return
		}
		txid, err = services.ReclaimGamingTableBond(r.Context(), game, sid)
	default:
		http.Error(w, "kind must be bond, stake or tablebond", http.StatusBadRequest)
		return
	}
	if err != nil {
		gamingReclaimError(w, err)
		return
	}
	gamingJSON(w, map[string]any{"txid": txid})
}

// gamingReclaimError keeps the game's own refusal, which says how many blocks
// are left on a lock.
func gamingReclaimError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrGamingGameNotInstalled):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, services.ErrGamingGameNotRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}
