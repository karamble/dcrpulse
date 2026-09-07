// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"errors"
	"time"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/services"
)

// treasuryActivationHeight is the block at which the treasury activated; it is
// the default and minimum start for a historical TSpend scan.
const treasuryActivationHeight int64 = 552448

// scanStartInput parameterizes treasury_scan_start.
type scanStartInput struct {
	StartHeight int64 `json:"startHeight,omitempty" jsonschema:"block height to start the scan from; defaults to and is clamped to the treasury activation height"`
}

// treasuryTools are the read-only "treasury" domain tools plus the local
// blockchain TSpend scan (no funds, no scope).
var treasuryTools = []toolDef{
	readTool("treasury", "treasury_info",
		"Get the Decred treasury overview (balance and recent treasury activity).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.FetchTreasuryInfo(ctx) }),
	readTool("treasury", "treasury_balance_history",
		"Get the treasury balance-over-time series (sampled at ~monthly cadence, cached).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.TreasuryBalanceHistory(ctx) }),
	readTool("treasury", "treasury_scan_progress",
		"Get the current historical TSpend scan progress.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetScanProgress() }),
	readTool("treasury", "treasury_scan_results",
		"Get the results from the last completed historical TSpend scan.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetScanResults(), nil }),
	readTool("treasury", "treasury_scan_start",
		"Start a local historical blockchain scan for TSpends. This reads the chain only; it spends no funds. One start per minute, shared with the dashboard.",
		func(ctx context.Context, in scanStartInput) (any, error) {
			// The same bucket as the dashboard's /treasury/scan-history
			// route, so browser and agent share one allowance.
			if !middleware.Limiter("treasury-scan", 60*time.Second, 1).Allow() {
				return nil, errors.New("rate limit exceeded, retry later")
			}
			startHeight := in.StartHeight
			if startHeight < treasuryActivationHeight {
				startHeight = treasuryActivationHeight
			}
			if err := services.TriggerHistoricalScan(startHeight); err != nil {
				return nil, err
			}
			return map[string]any{"started": true, "startHeight": startHeight}, nil
		}),
}
