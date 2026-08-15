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

	"dcrpulse/internal/gamingpb"
	"dcrpulse/internal/services"
)

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
	if !services.ValidGamingGCID(gcid) {
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

// BisonrelayGamingReclaimHandler takes back coin a game locked, into the
// account it is bound to.
//
// The destination is not a parameter: it is derived from the bound account, so
// neither the game nor this request can decide where the money lands.
func BisonrelayGamingReclaimHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game string `json:"game"`
		Kind string `json:"kind"`
		SID  string `json:"sid"`
		// Outpoint names coin other than the seat's current stake, which
		// is the way back for a deposit that was paid but never recorded.
		Outpoint string `json:"outpoint"`
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
	sid := strings.ToLower(strings.TrimSpace(req.SID))
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind != "bond" && sid == "" {
		// A stake and a table bond belong to one table; only the standing
		// bond exists without one.
		http.Error(w, "no table named", http.StatusBadRequest)
		return
	}

	txid, err := services.ReclaimGamingCoin(r.Context(), game, kind, sid, strings.TrimSpace(req.Outpoint))
	if err != nil {
		gamingTableError(w, err)
		return
	}
	gamingJSON(w, map[string]any{"txid": txid})
}

// BisonrelayGamingPayoutHandler tells a game where its winnings are to be paid.
//
// The address is not a parameter: it is derived from the bound account, so
// neither the game nor this request decides where the money lands. Normally
// this happens on its own when a game first reports itself; the route is here
// for the case where it did not.
func BisonrelayGamingPayoutHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game string `json:"game"`
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

	addr, err := services.PinGamingPayout(r.Context(), game)
	if err != nil {
		gamingTableError(w, err)
		return
	}
	gamingJSON(w, map[string]any{"address": addr})
}

// BisonrelayGamingStateHandler reports what a game last said about its own
// tables and locked coin.
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
	// Blocks remaining are derived here rather than reported: a count is true
	// for one block and then quietly wrong, so the game sends absolute
	// maturity heights and this measures them against the bridge's own tip.
	var tip int64
	if t, err := services.GamingChainTipNow(r.Context()); err == nil {
		tip = t.Height
	}
	gamingJSON(w, gamingStateView(state, tip))
}

// gamingLock is one piece of locked coin, with what the operator has to know to
// decide whether they can take it back yet.
type gamingLock struct {
	Kind       string `json:"kind"`
	SID        string `json:"sid,omitempty"`
	Seat       uint32 `json:"seat"`
	Outpoint   string `json:"outpoint"`
	Address    string `json:"address,omitempty"`
	Atoms      int64  `json:"atoms"`
	MaturesAt  int64  `json:"maturesAt"`
	BlocksLeft int64  `json:"blocksLeft"`
	Spendable  bool   `json:"spendable"`
	Spent      bool   `json:"spent"`
	// Spending is a spend of this output sitting in the mempool. The coin is
	// still this game's until it confirms, so the row stays; what it must not
	// do is invite a second attempt at coin already moving.
	Spending bool `json:"spending"`
}

// gamingLockAt fills in what is left to wait, given the bridge's tip. A tip of
// zero means the chain could not be read, so nothing is claimed to be ready.
func gamingLockAt(l gamingLock, tip int64) gamingLock {
	if l.Spent || l.Spending || l.MaturesAt <= 0 || tip <= 0 {
		return l
	}
	if left := l.MaturesAt - tip; left > 0 {
		l.BlocksLeft = left
	} else {
		l.Spendable = true
	}
	return l
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
			"settling":   t.GetSettling(),
		})
	}

	locks := make([]gamingLock, 0, 1+len(s.GetTableBonds())+len(s.GetStakes()))
	if b := s.GetBond(); b != nil && b.GetHasDeposit() {
		locks = append(locks, gamingLockAt(gamingLock{
			Kind:      "bond",
			Outpoint:  b.GetOutpoint(),
			Address:   b.GetAddress(),
			Atoms:     b.GetAtoms(),
			MaturesAt: b.GetMaturesAt(),
			Spent:     b.GetSpent(),
			Spending:  b.GetSpending(),
		}, tip))
	}
	for _, tb := range s.GetTableBonds() {
		locks = append(locks, gamingLockAt(gamingLock{
			Kind:      "tablebond",
			SID:       tb.GetSid(),
			Seat:      tb.GetSeat(),
			Outpoint:  tb.GetOutpoint(),
			Address:   tb.GetAddress(),
			Atoms:     tb.GetAtoms(),
			MaturesAt: tb.GetMaturesAt(),
			Spent:     tb.GetSpent(),
			Spending:  tb.GetSpending(),
		}, tip))
	}
	for _, st := range s.GetStakes() {
		locks = append(locks, gamingLockAt(gamingLock{
			Kind:      "stake",
			SID:       st.GetSid(),
			Seat:      st.GetSeat(),
			Outpoint:  st.GetOutpoint(),
			Address:   st.GetAddress(),
			Atoms:     st.GetAtoms(),
			MaturesAt: st.GetMaturesAt(),
			Spent:     st.GetSpent(),
			Spending:  st.GetSpending(),
		}, tip))
	}

	return map[string]any{
		"reported":         true,
		"reportedAt":       s.GetReportedAt(),
		"tipHeight":        tip,
		"tables":           tables,
		"locks":            locks,
		"payoutAddress":    s.GetPayoutAddress(),
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
