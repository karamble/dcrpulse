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

// newAddressInput parameterizes wallet_new_address.
type newAddressInput struct {
	Account uint32 `json:"account" jsonschema:"wallet account number to derive the address from"`
}

// walletValidateInput parameterizes wallet_validate_address.
type walletValidateInput struct {
	Address string `json:"address" jsonschema:"Decred address to validate and check ownership of"`
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
		"List the active wallet's receiving addresses that have received funds.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.FetchAddressesWithContext(ctx) }),
	readTool("wallet", "wallet_new_address",
		"Generate a fresh receiving address for a wallet account. Not a spend (no passphrase needed); use it to get a destination address.",
		func(ctx context.Context, in newAddressInput) (any, error) {
			addr, err := services.GetNextAddress(ctx, in.Account)
			if err != nil {
				return nil, err
			}
			return map[string]any{"account": in.Account, "address": addr}, nil
		}),
	readTool("wallet", "wallet_transactions",
		"List recent wallet transactions. Optional count (default 20) and from (offset).",
		func(ctx context.Context, in txListInput) (any, error) {
			count := in.Count
			if count <= 0 {
				count = 20
			}
			return services.ListTransactions(ctx, count, in.From)
		}),
	readTool("wallet", "wallet_validate_address",
		"Validate a Decred address and report whether it belongs to the active wallet (and which account). Requires 'address'.",
		func(ctx context.Context, in walletValidateInput) (any, error) {
			return services.ValidateAddress(ctx, in.Address)
		}),
	readTool("wallet", "wallet_sync_progress",
		"Get the active wallet's sync progress: phase, peer count, header/rescan progress.",
		func(_ context.Context, _ emptyInput) (any, error) { return services.GetSyncSnapshot(), nil }),
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
				if tripwire(a.id, err) {
					recordSpend(a, "wallet_send", in.Account, in.AmountDCR, in.Address, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "wallet_send", in.Account, in.AmountDCR, in.Address, "denied", err.Error())
				}
				return nil, err
			}
			unsigned, err := services.ConstructTransaction(ctx, in.Account, in.Address, atoms, false)
			if err != nil {
				grants.refund(a.id, atoms)
				zero(pass)
				recordSpend(a, "wallet_send", in.Account, in.AmountDCR, in.Address, "error", err.Error())
				return nil, err
			}
			// SignAndPublishTransaction zeroes pass after use.
			txid, err := services.SignAndPublishTransaction(ctx, in.Account, unsigned.UnsignedTransaction, pass)
			if err != nil {
				grants.refund(a.id, atoms)
				recordSpend(a, "wallet_send", in.Account, in.AmountDCR, in.Address, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "wallet_send", in.Account, in.AmountDCR, in.Address, "ok", txid)
			return map[string]any{
				"txid":      txid,
				"account":   in.Account,
				"address":   in.Address,
				"amountDcr": in.AmountDCR,
			}, nil
		}),
}
