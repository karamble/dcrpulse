// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
)

// The running-bot routes are RPC-only: bisonw's webserver can start, stop and
// persist a bot, but only its RPC server can reconfigure or refund one that is
// already running. Two of them name the market-maker config file by a path on
// the daemon's own filesystem (see config.DcrdexMMConfigPath), and bisonw reads
// whatever that file holds rather than taking a config in the request.
//
// The argument order is not the same across the two update routes: host is the
// first argument to updaterunningbotinv and the second to updaterunningbotcfg.
// The server checks the argument count but not what any slot means.

// MMBalances is the funding a bot may still draw on, in atoms, keyed by asset
// id.
type MMBalances struct {
	DEX map[uint32]uint64 `json:"dexBalances"`
	CEX map[uint32]uint64 `json:"cexBalances"`
}

// encodeBotDiffs renders signed per-asset atom deltas as the [[assetID,delta]]
// JSON array the diff arguments carry. A nil or empty map encodes as "[]",
// which the routes accept as no change.
func encodeBotDiffs(diffs map[uint32]int64) (string, error) {
	pairs := make([][2]int64, 0, len(diffs))
	for _, assetID := range slices.Sorted(maps.Keys(diffs)) {
		pairs = append(pairs, [2]int64{int64(assetID), diffs[assetID]})
	}
	b, err := json.Marshal(pairs)
	if err != nil {
		return "", fmt.Errorf("bisonw: encode balance diffs: %w", err)
	}
	return string(b), nil
}

// MMAvailableBalances reports what the daemon would let a bot on this market
// allocate, reading the bot's settings from the config file at cfgPath. It is
// read-only, so it doubles as the check that cfgPath is one bisonw can read.
func (c *Client) MMAvailableBalances(ctx context.Context, cfgPath, host string, baseID, quoteID uint32) (*MMBalances, error) {
	args := []string{
		cfgPath,
		host,
		strconv.FormatUint(uint64(baseID), 10),
		strconv.FormatUint(uint64(quoteID), 10),
	}
	var res MMBalances
	if err := c.Call(ctx, "mmavailablebalances", nil, args, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// UpdateRunningBotCfg applies the config file at cfgPath to the bot already
// running on this market, optionally moving funds by the same signed deltas
// UpdateRunningBotInventory takes. The daemon does not write the file, so the
// caller persists the config first (the webserver's updatebotconfig route) and
// then points this at it.
//
// A config bisonw rejects does not fail the call: it stops the bot and cancels
// every order it had booked.
func (c *Client) UpdateRunningBotCfg(ctx context.Context, cfgPath, host string, baseID, quoteID uint32, dexDiffs, cexDiffs map[uint32]int64) error {
	dex, err := encodeBotDiffs(dexDiffs)
	if err != nil {
		return err
	}
	cex, err := encodeBotDiffs(cexDiffs)
	if err != nil {
		return err
	}
	args := []string{
		cfgPath,
		host,
		strconv.FormatUint(uint64(baseID), 10),
		strconv.FormatUint(uint64(quoteID), 10),
		dex,
		cex,
	}
	return c.Call(ctx, "updaterunningbotcfg", nil, args, nil)
}

// UpdateRunningBotInventory moves funds between a running bot's allocation and
// the wallet by signed atom deltas, leaving its config alone. A withdrawal
// larger than the bot holds is clamped to what is available and still reports
// success, so re-read the balances rather than assuming the request applied.
func (c *Client) UpdateRunningBotInventory(ctx context.Context, host string, baseID, quoteID uint32, dexDiffs, cexDiffs map[uint32]int64) error {
	dex, err := encodeBotDiffs(dexDiffs)
	if err != nil {
		return err
	}
	cex, err := encodeBotDiffs(cexDiffs)
	if err != nil {
		return err
	}
	args := []string{
		host,
		strconv.FormatUint(uint64(baseID), 10),
		strconv.FormatUint(uint64(quoteID), 10),
		dex,
		cex,
	}
	return c.Call(ctx, "updaterunningbotinv", nil, args, nil)
}
