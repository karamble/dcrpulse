// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"os"
	"path/filepath"
)

// GamingTableStatus is what this bridge's own records say about one table: that
// the operator accepted it, where its seat bond and stake stand, and what
// became of its payout.
type GamingTableStatus struct {
	Accepted bool               `json:"accepted"`
	Closed   bool               `json:"closed"`
	SeatBond *GamingTableSpend  `json:"seatBond,omitempty"`
	Stake    *GamingTableSpend  `json:"stake,omitempty"`
	Payout   *GamingTablePayout `json:"payout,omitempty"`
}

// GamingTableSpend is the latest request the table's game made for one deposit.
type GamingTableSpend struct {
	State GamingSpendState `json:"state"`
	TxID  string           `json:"txid,omitempty"`
}

// GamingTablePayout is the table's payout and its transaction on the chain.
type GamingTablePayout struct {
	ID    string             `json:"id"`
	State string             `json:"state"`
	Chain *GamingPayoutChain `json:"chain,omitempty"`
}

// ValidGamingTableID reports whether sid has the form of a table identifier.
func ValidGamingTableID(sid string) bool {
	return financialTableID.MatchString(sid)
}

// gamingPayoutRank prefers a live payout over one that expired or was refused.
func gamingPayoutRank(state string) int {
	switch state {
	case "confirmed":
		return 4
	case "publishing":
		return 3
	case "awaiting_signatures", "awaiting_approval":
		return 2
	default:
		return 1
	}
}

// ReadGamingTableStatus reports one table of one game. A table this bridge
// never accepted comes back with Accepted false and nothing else.
func (br *Bridge) ReadGamingTableStatus(game, sid string) (GamingTableStatus, error) {
	var out GamingTableStatus
	// Only an existing ledger is read; opening one would create it.
	if _, err := os.Stat(filepath.Join(br.dataDir, "financial-authority", "authority.json")); err != nil {
		return out, nil
	}
	store, err := br.gamingFundsStore()
	if err != nil {
		return out, err
	}
	tables, err := store.Tables()
	if err != nil {
		return out, err
	}
	for _, t := range tables {
		if t.Scope.Game == game && t.Table == sid {
			out.Accepted = true
			out.Closed = out.Closed || t.Closed
		}
	}
	if !out.Accepted {
		return out, nil
	}

	spends, _, err := br.GamingSpendLedger()
	if err != nil {
		return out, err
	}
	for _, s := range spends {
		if s.Game != game || s.TableID != sid {
			continue
		}
		var slot **GamingTableSpend
		switch s.DepositKind {
		case "seatbond":
			slot = &out.SeatBond
		case "stake":
			slot = &out.Stake
		default:
			continue
		}
		// The ledger is newest first, so the first one is the latest.
		if *slot == nil {
			*slot = &GamingTableSpend{State: s.State, TxID: s.TxID}
		}
	}

	settlements, err := store.AllSettlements()
	if err != nil {
		return out, err
	}
	chains := gamingPayoutChains(store)
	for _, p := range settlements {
		if p.Scope.Game != game || p.Table != sid {
			continue
		}
		if out.Payout == nil || gamingPayoutRank(p.State) > gamingPayoutRank(out.Payout.State) {
			out.Payout = &GamingTablePayout{ID: p.ID, State: p.State, Chain: chains[p.ID]}
		}
	}
	return out, nil
}
