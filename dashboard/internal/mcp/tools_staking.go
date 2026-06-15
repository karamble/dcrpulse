// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

// stakingTools are the read-only "staking" domain tools.
var stakingTools = []toolDef{
	readTool("staking", "staking_tickets",
		"List the active wallet's staking tickets with their lifecycle status (live, voted, etc.).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListTickets(ctx) }),
	readTool("staking", "staking_info",
		"Get the active wallet's staking summary (ticket counts, locked value, rewards).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.FetchWalletStakingInfo(ctx) }),
	readTool("staking", "staking_vsps",
		"List known Voting Service Providers from the public registry.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListVSPs(ctx) }),
	readTool("staking", "staking_used_vsps",
		"List the Voting Service Providers this wallet has used.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetUsedVSPs(ctx) }),
	readTool("staking", "staking_autobuyer_settings",
		"Get the saved automatic ticket-buyer settings.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.LoadAutobuyerSettings(ctx) }),
}
