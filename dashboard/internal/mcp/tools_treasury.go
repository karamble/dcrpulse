// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

// treasuryTools are the read-only "treasury" domain tools.
var treasuryTools = []toolDef{
	readTool("treasury", "treasury_info",
		"Get the Decred treasury overview (balance and recent treasury activity).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.FetchTreasuryInfo(ctx) }),
	readTool("treasury", "treasury_mempool_tspends",
		"List treasury-spend (TSpend) transactions currently in the mempool.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetMempoolTSpends(ctx) }),
}
