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
	"dcrpulse/internal/types"
)

// purchaseInput parameterizes staking_purchase.
type purchaseInput struct {
	Account       uint32 `json:"account" jsonschema:"source account number"`
	NumTickets    uint32 `json:"numTickets" jsonschema:"number of tickets to buy"`
	VSPHost       string `json:"vspHost" jsonschema:"VSP host URL (from staking_vsps)"`
	VSPPubkey     string `json:"vspPubkey" jsonschema:"VSP public key (from staking_vsps)"`
	ChangeAccount uint32 `json:"changeAccount,omitempty" jsonschema:"change account number; defaults to the source account"`
}

// vspInfoInput parameterizes staking_vsp_info.
type vspInfoInput struct {
	Host string `json:"host" jsonschema:"VSP host URL to probe (from staking_vsps)"`
}

// autobuyerSaveSettingsInput parameterizes staking_autobuyer_save_settings.
type autobuyerSaveSettingsInput struct {
	Account           uint32  `json:"account" jsonschema:"account number tickets are bought from"`
	VSPHost           string  `json:"vspHost" jsonschema:"VSP host URL (from staking_vsps)"`
	VSPPubkey         string  `json:"vspPubkey" jsonschema:"VSP public key (from staking_vsps)"`
	BalanceToMaintain float64 `json:"balanceToMaintain" jsonschema:"DCR balance to keep unspent; the autobuyer only buys above this"`
}

// vspTicketMaintenanceInput parameterizes the VSP ticket-maintenance writes.
type vspTicketMaintenanceInput struct {
	VSPHost       string `json:"vspHost" jsonschema:"VSP host URL (from staking_vsps)"`
	VSPPubkey     string `json:"vspPubkey" jsonschema:"VSP public key (from staking_vsps)"`
	Account       uint32 `json:"account" jsonschema:"account holding the tickets"`
	ChangeAccount uint32 `json:"changeAccount,omitempty" jsonschema:"change account number; defaults to the source account"`
}

// stakingTools are the staking domain tools (reads plus grant-gated purchase).
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
	readTool("staking", "staking_vsp_info",
		"Probe one Voting Service Provider host and return its advertised vspinfo (fee, pubkey, network).",
		func(ctx context.Context, in vspInfoInput) (any, error) {
			if in.Host == "" {
				return nil, fmt.Errorf("host is required (see staking_vsps)")
			}
			return services.GetVSPInfo(ctx, in.Host)
		}),
	readTool("staking", "staking_purchase_status",
		"Report whether a background (mixed) ticket purchase is running plus the most recent terminal result.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.PurchaseStatusSnapshot(), nil }),
	readTool("staking", "staking_autobuyer_status",
		"Get the automatic ticket-buyer status (running flag, last error, persisted settings).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.AutobuyerStatusSnapshot(ctx), nil }),
	agentTool("staking", "staking_purchase",
		"Buy staking tickets through a VSP. Requires a spend grant covering the account; the cost (ticket price x count) is checked against the grant caps. The agent never supplies a passphrase.",
		func(ctx context.Context, a *agent, in purchaseInput) (any, error) {
			if in.NumTickets == 0 {
				return nil, fmt.Errorf("numTickets must be at least 1")
			}
			if in.VSPHost == "" || in.VSPPubkey == "" {
				return nil, fmt.Errorf("vspHost and vspPubkey are required (see staking_vsps)")
			}
			// Reject before pricing the ticket when the agent has no grant for
			// this account, so an ungranted call does no chain work.
			if err := grants.precheckAccount(a.id, in.Account, time.Now()); err != nil {
				recordSpend(a, "staking_purchase", in.Account, 0, in.VSPHost, "denied", err.Error())
				return nil, err
			}
			info, err := services.FetchStakingInfo()
			if err != nil {
				return nil, fmt.Errorf("ticket price unavailable: %w", err)
			}
			perTicket, err := dcrutil.NewAmount(info.TicketPrice)
			if err != nil {
				return nil, err
			}
			costDCR := info.TicketPrice * float64(in.NumTickets)
			totalAtoms := int64(perTicket) * int64(in.NumTickets)
			changeAccount := in.ChangeAccount
			if changeAccount == 0 {
				changeAccount = in.Account
			}
			// Ticket purchases have no recipient address; the allowlist is skipped.
			pass, err := grants.authorize(a.id, in.Account, totalAtoms, "", time.Now())
			if err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "staking_purchase", in.Account, costDCR, in.VSPHost, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "staking_purchase", in.Account, costDCR, in.VSPHost, "denied", err.Error())
				}
				return nil, err
			}
			defer zero(pass)
			resp, err := services.PurchaseTickets(ctx, in.Account, in.NumTickets, in.VSPHost, in.VSPPubkey, changeAccount, pass)
			if err != nil {
				grants.refund(a.id, totalAtoms)
				recordSpend(a, "staking_purchase", in.Account, costDCR, in.VSPHost, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "staking_purchase", in.Account, costDCR, in.VSPHost, "ok",
				fmt.Sprintf("%d ticket(s)", len(resp.TicketHashes)))
			return resp, nil
		}),
	agentTool("staking", "staking_autobuyer_save_settings",
		"Persist the automatic ticket-buyer settings (does not start it). Requires a staking grant.",
		func(ctx context.Context, a *agent, in autobuyerSaveSettingsInput) (any, error) {
			if in.VSPHost == "" || in.VSPPubkey == "" {
				return nil, fmt.Errorf("vspHost and vspPubkey are required (see staking_vsps)")
			}
			if in.BalanceToMaintain < 0 {
				return nil, fmt.Errorf("balanceToMaintain must be >= 0")
			}
			if err := grants.authorizeAction(a.id, scopeStaking, time.Now()); err != nil {
				recordSpend(a, "staking_autobuyer_save_settings", in.Account, 0, in.VSPHost, "denied", err.Error())
				return nil, err
			}
			s := types.AutobuyerSettings{
				Account:           in.Account,
				VspHost:           in.VSPHost,
				VspPubkey:         in.VSPPubkey,
				BalanceToMaintain: in.BalanceToMaintain,
			}
			if err := services.SaveAutobuyerSettings(ctx, &s); err != nil {
				recordSpend(a, "staking_autobuyer_save_settings", in.Account, 0, in.VSPHost, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "staking_autobuyer_save_settings", in.Account, 0, in.VSPHost, "ok", "")
			return map[string]any{"ok": true}, nil
		}),
	agentTool("staking", "staking_autobuyer_stop",
		"Stop the running automatic ticket-buyer (idempotent). Requires a staking grant.",
		func(ctx context.Context, a *agent, _ emptyInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeStaking, time.Now()); err != nil {
				recordSpend(a, "staking_autobuyer_stop", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			services.StopAutobuyer()
			recordSpend(a, "staking_autobuyer_stop", 0, 0, "", "ok", "")
			return map[string]any{"ok": true}, nil
		}),
	agentTool("staking", "staking_sync_failed_vsp_tickets",
		"Retry VSP fee payments for the wallet's failed tickets. Requires a staking grant; signs with the held passphrase.",
		func(ctx context.Context, a *agent, in vspTicketMaintenanceInput) (any, error) {
			if in.VSPHost == "" || in.VSPPubkey == "" {
				return nil, fmt.Errorf("vspHost and vspPubkey are required (see staking_vsps)")
			}
			changeAccount := in.ChangeAccount
			if changeAccount == 0 {
				changeAccount = in.Account
			}
			pass, err := grants.authorizeActionPass(a.id, scopeStaking, time.Now())
			if err != nil {
				recordSpend(a, "staking_sync_failed_vsp_tickets", in.Account, 0, in.VSPHost, "denied", err.Error())
				return nil, err
			}
			defer zero(pass)
			summary, err := services.SyncFailedVSPTickets(ctx, in.VSPHost, in.VSPPubkey, in.Account, changeAccount, pass)
			if err != nil {
				recordSpend(a, "staking_sync_failed_vsp_tickets", in.Account, 0, in.VSPHost, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "staking_sync_failed_vsp_tickets", in.Account, 0, in.VSPHost, "ok", "")
			return summary, nil
		}),
	agentTool("staking", "staking_process_unmanaged_vsp_tickets",
		"Re-associate the wallet's untracked tickets with a VSP. Requires a staking grant; signs with the held passphrase.",
		func(ctx context.Context, a *agent, in vspTicketMaintenanceInput) (any, error) {
			if in.VSPHost == "" || in.VSPPubkey == "" {
				return nil, fmt.Errorf("vspHost and vspPubkey are required (see staking_vsps)")
			}
			changeAccount := in.ChangeAccount
			if changeAccount == 0 {
				changeAccount = in.Account
			}
			pass, err := grants.authorizeActionPass(a.id, scopeStaking, time.Now())
			if err != nil {
				recordSpend(a, "staking_process_unmanaged_vsp_tickets", in.Account, 0, in.VSPHost, "denied", err.Error())
				return nil, err
			}
			defer zero(pass)
			summary, err := services.ProcessUnmanagedVSPTickets(ctx, in.VSPHost, in.VSPPubkey, in.Account, changeAccount, pass)
			if err != nil {
				recordSpend(a, "staking_process_unmanaged_vsp_tickets", in.Account, 0, in.VSPHost, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "staking_process_unmanaged_vsp_tickets", in.Account, 0, in.VSPHost, "ok", "")
			return summary, nil
		}),
}
