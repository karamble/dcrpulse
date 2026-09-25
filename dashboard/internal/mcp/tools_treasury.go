// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/services"
)

// treasuryActivationHeight is the block at which the treasury activated; it is
// the default and minimum start for a historical TSpend scan.
const treasuryActivationHeight int64 = 552448

// scanStartInput parameterizes treasury_scan_start.
type scanStartInput struct {
	StartHeight int64 `json:"startHeight,omitempty" jsonschema:"block height to start the scan from; defaults to and is clamped to the treasury activation height; must not exceed the current chain tip"`
}

// treasuryTools are the read-only "treasury" domain tools plus the local
// blockchain TSpend scan (no funds, no scope).
var treasuryTools = []toolDef{
	readTool("treasury", "treasury_info",
		"Get the Decred treasury overview (balance and recent treasury activity).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.FetchTreasuryInfo(ctx) }),
	readTool("treasury", "treasury_balance_history",
		"Get the treasury balance-over-time series (the first block of every UTC month plus the tip, cached).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.TreasuryBalanceHistory(ctx) }),
	readTool("treasury", "treasury_scan_progress",
		"Get the current treasury scan progress.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetScanProgress() }),
	readTool("treasury", "treasury_scan_results",
		"Get what the last treasury scan recorded over the blocks it read: treasury spends, contributions, and the block reward per UTC month, in atoms.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetScanResults(), nil }),
	readTool("treasury", "treasury_scan_start",
		"Start a local blockchain scan of every block for treasury spends, contributions and block reward. This reads the chain only; it spends no funds. One start per minute, shared with the dashboard.",
		func(ctx context.Context, in scanStartInput) (any, error) {
			if err := allow(middleware.TreasuryScan); err != nil {
				return nil, err
			}
			startHeight := in.StartHeight
			if startHeight < treasuryActivationHeight {
				startHeight = treasuryActivationHeight
			}
			if err := services.TriggerHistoricalScan(ctx, startHeight); err != nil {
				return nil, err
			}
			return map[string]any{"started": true, "startHeight": startHeight}, nil
		}),
}
