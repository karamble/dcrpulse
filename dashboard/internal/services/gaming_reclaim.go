// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Taking back coin a game locked on this user's behalf.
//
// It stays here rather than in a game's own page: it signs and broadcasts a
// transaction, and the page runs in a sandboxed frame reaching an allowlist.
// The destination is derived from the bound account so a game cannot name it.

// GamingBond is what a game says about the standing deposit that buys its
// seats.
type GamingBond struct {
	Game          string `json:"game"`
	Address       string `json:"address"`
	Outpoint      string `json:"outpoint"`
	HasDeposit    bool   `json:"hasDeposit"`
	MinAtoms      int64  `json:"minAtoms"`
	MinBlocks     int64  `json:"minBlocks"`
	Atoms         int64  `json:"atoms,omitempty"`
	Confirmations int64  `json:"confirmations,omitempty"`
	Height        int64  `json:"height,omitempty"`
	MaturesAt     int64  `json:"maturesAt,omitempty"`
	BlocksLeft    int64  `json:"blocksLeft,omitempty"`
	Spendable     bool   `json:"spendable,omitempty"`
	Spent         bool   `json:"spent,omitempty"`
	ChainErr      string `json:"chainErr,omitempty"`
}

// GamingBondStatus asks a game about its standing bond.
func GamingBondStatus(ctx context.Context, game string) (GamingBond, error) {
	body, err := gamingCall(ctx, game, http.MethodGet, "/bond", nil, 20*time.Second)
	if err != nil {
		return GamingBond{}, err
	}
	var out GamingBond
	if err := json.Unmarshal(body, &out); err != nil {
		return GamingBond{}, fmt.Errorf("%s answered with something unreadable: %w", game, err)
	}
	out.Game = game
	return out, nil
}

// ReclaimGamingBond takes a matured standing bond back into the bound account.
func ReclaimGamingBond(ctx context.Context, game string) (string, error) {
	addr, err := gamingReceiveAddress(ctx)
	if err != nil {
		return "", err
	}
	return gamingReclaim(ctx, game, "/bond/sweep", map[string]string{"destAddr": addr})
}

// ReclaimGamingStake takes a matured stake back out of one table's escrow.
func ReclaimGamingStake(ctx context.Context, game, sid string) (string, error) {
	addr, err := gamingReceiveAddress(ctx)
	if err != nil {
		return "", err
	}
	return gamingReclaim(ctx, game, "/table/refund", map[string]string{"sid": sid, "destAddr": addr})
}

// ReclaimGamingTableBond takes a matured forfeitable bond back from one table.
//
// The third lock and the slowest. A stake waits out that table's CSV, the
// standing bond waits out the minimum, and this waits out a week - long enough
// that nobody can sit out their own claim window. It is the branch that needs
// nobody else's agreement, which is what makes it the way out of a table that
// dissolved rather than ended.
func ReclaimGamingTableBond(ctx context.Context, game, sid string) (string, error) {
	addr, err := gamingReceiveAddress(ctx)
	if err != nil {
		return "", err
	}
	return gamingReclaim(ctx, game, "/table/bond/sweep", map[string]string{"sid": sid, "destAddr": addr})
}

// gamingReceiveAddress is where reclaimed coin lands: a fresh address in the
// account games are confined to, never one a game supplied.
func gamingReceiveAddress(ctx context.Context) (string, error) {
	account, err := gamingAccountNumber(ctx)
	if err != nil {
		return "", fmt.Errorf("no gaming account to pay into: %w", err)
	}
	addr, err := GetNextAddress(ctx, account)
	if err != nil {
		return "", fmt.Errorf("could not derive an address to pay into: %w", err)
	}
	return addr, nil
}

func gamingReclaim(ctx context.Context, game, path string, req map[string]string) (string, error) {
	blob, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	// Long, because it builds a transaction and waits for the network to
	// take it.
	body, err := gamingCall(ctx, game, http.MethodPost, path, blob, 2*time.Minute)
	if err != nil {
		return "", err
	}
	var out struct {
		TxID string `json:"txid"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.TxID == "" {
		return "", fmt.Errorf("%s did not say what it sent", game)
	}
	return out.TxID, nil
}

// gamingCall speaks to a game as the host, with the game's own token.
func gamingCall(ctx context.Context, game, method, path string, body []byte, wait time.Duration) ([]byte, error) {
	base, err := GamingGameURL(game)
	if err != nil {
		return nil, err
	}
	token, ok := GamingGameToken(game)
	if !ok {
		return nil, ErrGamingGameNotInstalled
	}

	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ask %s: %w", game, err)
	}
	defer resp.Body.Close()

	out, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// The game's own words. A refusal here says exactly how many
		// blocks are left on a lock, which is the useful part.
		var said struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(out, &said) == nil && said.Error != "" {
			return nil, fmt.Errorf("%s", said.Error)
		}
		return nil, fmt.Errorf("%s answered %d", game, resp.StatusCode)
	}
	return out, nil
}

// GamingTableBond is one forfeitable bond a game holds at one table.
//
// Separate from GamingBond because it is separate coin under a separate key on a
// separate clock. Conflating them in the interface would invite the mistake the
// protocol is careful to avoid: the standing bond buys the right to join and can
// never be forfeited, this one is what a seat loses for walking out of a hand.
type GamingTableBond struct {
	Game      string `json:"game"`
	SID       string `json:"sid"`
	Seat      uint32 `json:"seat"`
	Outpoint  string `json:"outpoint"`
	Address   string `json:"address,omitempty"`
	Atoms     int64  `json:"atoms,omitempty"`
	MinBlocks uint32 `json:"minBlocks"`

	Confirmations int64  `json:"confirmations,omitempty"`
	Height        int64  `json:"height,omitempty"`
	MaturesAt     int64  `json:"maturesAt,omitempty"`
	BlocksLeft    int64  `json:"blocksLeft,omitempty"`
	Spendable     bool   `json:"spendable,omitempty"`
	Spent         bool   `json:"spent,omitempty"`
	ChainErr      string `json:"chainErr,omitempty"`
}

// GamingTableBonds asks a game what it still holds locked at tables.
func GamingTableBonds(ctx context.Context, game string) ([]GamingTableBond, error) {
	body, err := gamingCall(ctx, game, http.MethodGet, "/table/bonds", nil, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var out struct {
		Bonds []GamingTableBond `json:"bonds"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("%s answered with something unreadable: %w", game, err)
	}
	for i := range out.Bonds {
		out.Bonds[i].Game = game
	}
	return out.Bonds, nil
}
