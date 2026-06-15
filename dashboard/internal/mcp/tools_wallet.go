// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

// txListInput parameterizes wallet_transactions. Both fields are optional
// (omitempty), so the schema does not require them.
type txListInput struct {
	Count int `json:"count,omitempty" jsonschema:"max transactions to return (default 20)"`
	From  int `json:"from,omitempty" jsonschema:"offset into the transaction list"`
}

// walletTools are the read-only "wallet" domain tools. They report on the
// active wallet only; spend tools (gated on a user grant) come in a later phase.
var walletTools = []toolDef{
	readTool("wallet", "wallet_dashboard",
		"Get the active wallet overview: balances and wallet status.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return services.FetchWalletDashboardDataWithContext(ctx)
		}),
	readTool("wallet", "wallet_accounts",
		"List the accounts in the active wallet with their balances.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.FetchAllAccounts(ctx) }),
	readTool("wallet", "wallet_status",
		"Get the active wallet's status (loaded, locked/unlocked, sync state).",
		func(_ context.Context, _ emptyInput) (any, error) { return services.FetchWalletStatus() }),
	readTool("wallet", "wallet_addresses",
		"List the active wallet's receiving addresses.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.FetchAddressesWithContext(ctx) }),
	readTool("wallet", "wallet_transactions",
		"List recent wallet transactions. Optional count (default 20) and from (offset).",
		func(ctx context.Context, in txListInput) (any, error) {
			count := in.Count
			if count <= 0 {
				count = 20
			}
			return services.ListTransactions(ctx, count, in.From)
		}),
}
