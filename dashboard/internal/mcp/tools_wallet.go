// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/services"
)

// txListInput parameterizes wallet_transactions. Both fields are optional
// (omitempty), so the schema does not require them.
type txListInput struct {
	Count int `json:"count,omitempty" jsonschema:"max transactions to return (default 20)"`
	From  int `json:"from,omitempty" jsonschema:"offset into the transaction list"`
}

// sendInput parameterizes wallet_send.
type sendInput struct {
	Account   uint32  `json:"account" jsonschema:"source wallet account number"`
	Address   string  `json:"address" jsonschema:"destination Decred address"`
	AmountDCR float64 `json:"amountDcr" jsonschema:"amount to send, in DCR"`
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
	agentTool("wallet", "wallet_send",
		"Send DCR on-chain from a wallet account. Requires a user-granted spend grant covering the account and amount; the agent never supplies a passphrase. Amount is in DCR. Returns the transaction id.",
		func(ctx context.Context, a *agent, in sendInput) (any, error) {
			amt, err := dcrutil.NewAmount(in.AmountDCR)
			if err != nil {
				return nil, fmt.Errorf("invalid amount: %w", err)
			}
			atoms := int64(amt)
			// Check the agent's grant (scope, caps, allowlist, expiry) and obtain a
			// private copy of the passphrase. Denials are returned to the agent.
			pass, err := grants.authorize(a.id, in.Account, atoms, in.Address, time.Now())
			if err != nil {
				return nil, err
			}
			unsigned, err := services.ConstructTransaction(ctx, in.Account, in.Address, atoms, false)
			if err != nil {
				grants.refund(a.id, atoms)
				zero(pass)
				return nil, err
			}
			// SignAndPublishTransaction zeroes pass after use.
			txid, err := services.SignAndPublishTransaction(ctx, in.Account, unsigned.UnsignedTransaction, pass)
			if err != nil {
				grants.refund(a.id, atoms)
				return nil, err
			}
			return map[string]any{
				"txid":      txid,
				"account":   in.Account,
				"address":   in.Address,
				"amountDcr": in.AmountDCR,
			}, nil
		}),
}
