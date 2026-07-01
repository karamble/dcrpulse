// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
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

// txOutputInput is one recipient in a constructed transaction.
type txOutputInput struct {
	Address     string `json:"address" jsonschema:"destination Decred address"`
	AmountAtoms int64  `json:"amountAtoms" jsonschema:"amount to send to this address, in atoms"`
}

// walletConstructInput parameterizes wallet_construct_transaction. Provide either a
// single address+amountAtoms or an outputs list; set sendAll to sweep the whole
// account balance to the single address.
type walletConstructInput struct {
	Account     uint32          `json:"account" jsonschema:"source wallet account number"`
	Address     string          `json:"address,omitempty" jsonschema:"destination address for a single-recipient send"`
	AmountAtoms int64           `json:"amountAtoms,omitempty" jsonschema:"amount for a single-recipient send, in atoms"`
	Outputs     []txOutputInput `json:"outputs,omitempty" jsonschema:"multiple recipients; alternative to address+amountAtoms"`
	SendAll     bool            `json:"sendAll,omitempty" jsonschema:"sweep the whole account balance to the single address"`
}

// signedTxInput carries a signed transaction for decode or broadcast, either as
// base64 of the raw signed bytes (e.g. a hardware wallet .dcrtx file) or as a
// hex/text export.
type signedTxInput struct {
	SignedTxB64 string `json:"signedTxB64,omitempty" jsonschema:"base64 of the raw signed transaction bytes (e.g. a hardware wallet .dcrtx file)"`
	SignedTxHex string `json:"signedTxHex,omitempty" jsonschema:"signed transaction as hex or a text export; used when signedTxB64 is empty"`
}

// signedTxBytes resolves a signed transaction from base64 (raw bytes, e.g. a
// hardware wallet .dcrtx file) or a hex/text export, mirroring the offline-signing
// HTTP handlers.
func signedTxBytes(b64, text string) ([]byte, error) {
	if s := strings.TrimSpace(b64); s != "" {
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 signed transaction")
		}
		return data, nil
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("signedTxB64 or signedTxHex required")
	}
	return []byte(text), nil
}

// txAlreadyKnown reports whether a broadcast error means the transaction was
// already accepted (already broadcast or in the mempool), a benign "already done"
// outcome rather than a failure.
func txAlreadyKnown(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "already have") || strings.Contains(s, "already exists") ||
		strings.Contains(s, "duplicate") || strings.Contains(s, "in mempool") ||
		strings.Contains(s, "transaction already")
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
			pass, err := grants.authorize(ctx, a.id, in.Account, atoms, in.Address, time.Now())
			if err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "wallet_send", in.Account, in.AmountDCR, in.Address, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "wallet_send", in.Account, in.AmountDCR, in.Address, "denied", err.Error())
				}
				return nil, err
			}
			unsigned, err := services.ConstructTransaction(ctx, in.Account, []types.TxRecipient{{Address: in.Address, AmountAtoms: atoms}}, false)
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
	readTool("wallet", "wallet_construct_transaction",
		"Build an UNSIGNED transaction and summarize its inputs, outputs, change, fee, and net debit. Uses no private keys and works on watch-only wallets. Returns unsignedTxHex to sign offline on a hardware wallet, then broadcast the signed result with wallet_broadcast_signed_transaction. Provide either address+amountAtoms or an outputs list; set sendAll to sweep the account.",
		func(ctx context.Context, in walletConstructInput) (any, error) {
			var recipients []types.TxRecipient
			if len(in.Outputs) > 0 {
				for _, o := range in.Outputs {
					recipients = append(recipients, types.TxRecipient{Address: o.Address, AmountAtoms: o.AmountAtoms})
				}
			} else {
				recipients = []types.TxRecipient{{Address: in.Address, AmountAtoms: in.AmountAtoms}}
			}
			return services.ConstructUnsignedTx(ctx, in.Account, recipients, in.SendAll)
		}),
	readTool("wallet", "wallet_decode_signed_transaction",
		"Decode a signed transaction (base64 of the raw bytes, or a hex/text export) into a preview: txid, size, per-output address/amount/isMine, and fee. Uses no private keys; use it to verify a hardware-signed transaction before broadcasting.",
		func(ctx context.Context, in signedTxInput) (any, error) {
			data, err := signedTxBytes(in.SignedTxB64, in.SignedTxHex)
			if err != nil {
				return nil, err
			}
			return services.PreviewSignedTransaction(ctx, data)
		}),
	agentTool("wallet", "wallet_broadcast_signed_transaction",
		"Broadcast an already-signed transaction (base64 raw bytes or hex/text export) to the network. The transaction must have been signed by a human on a hardware wallet; the agent only relays it. Requires a grant with the wallet.broadcast scope. Returns the txid; an already-broadcast transaction is reported as success.",
		func(ctx context.Context, a *agent, in signedTxInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeWalletBroadcast, time.Now()); err != nil {
				recordSpend(a, "wallet_broadcast_signed_transaction", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			data, err := signedTxBytes(in.SignedTxB64, in.SignedTxHex)
			if err != nil {
				recordSpend(a, "wallet_broadcast_signed_transaction", 0, 0, "", "error", err.Error())
				return nil, err
			}
			txBytes, tx, err := services.ParseSignedTransaction(data)
			if err != nil {
				recordSpend(a, "wallet_broadcast_signed_transaction", 0, 0, "", "error", err.Error())
				return nil, err
			}
			txid := tx.TxHash().String()
			txHash, err := services.BroadcastSignedTransaction(ctx, txBytes)
			if err != nil {
				if txAlreadyKnown(err) {
					recordSpend(a, "wallet_broadcast_signed_transaction", 0, 0, txid, "ok", "already broadcast")
					return map[string]any{"txHash": txid, "alreadyBroadcast": true}, nil
				}
				recordSpend(a, "wallet_broadcast_signed_transaction", 0, 0, txid, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "wallet_broadcast_signed_transaction", 0, 0, txHash, "ok", txHash)
			return map[string]any{"txHash": txHash, "alreadyBroadcast": false}, nil
		}),
}
