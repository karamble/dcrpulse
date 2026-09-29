// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
	"dcrpulse/internal/utils"
)

// normVSPHost canonicalizes a VSP host for comparison: the public registry and
// the wallet's used-VSP history both store it without a scheme.
func normVSPHost(h string) string {
	h = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(h), "https://"), "http://")
	return strings.ToLower(strings.TrimSuffix(h, "/"))
}

// resolveKnownVSP requires an agent-named VSP to be one this wallet has already
// used or one the public registry lists, with a matching pubkey. The VSP names
// the address its fee is paid to, so an unconstrained host lets an agent choose
// where wallet funds land. The dashboard's own UI paths stay unrestricted.
// findKnownVSP returns the used-list or registry entry for host, or an error
// naming what is acceptable. Callers that pay a VSP go through resolveKnownVSP,
// which additionally pins the pubkey; a read-only probe only needs membership.
func findKnownVSP(ctx context.Context, host string) (*types.VSPInfo, error) {
	want := normVSPHost(host)
	if want == "" {
		return nil, fmt.Errorf("vspHost is required (see staking_vsps)")
	}
	match := func(list []types.VSPInfo) *types.VSPInfo {
		for i := range list {
			if normVSPHost(list[i].Host) == want {
				return &list[i]
			}
		}
		return nil
	}
	// The used list first: it needs no outbound request, and a VSP this wallet
	// already pays is the common case. A lookup failure (or a disabled registry
	// listing, which returns no entries) just leaves that set empty.
	used, _ := services.GetUsedVSPs(ctx)
	found := match(used)
	if found == nil {
		registry, _ := services.ListVSPs(ctx)
		found = match(registry)
	}
	if found == nil {
		return nil, fmt.Errorf("vspHost %q is not a VSP this wallet has used or a public registry entry: pick one from staking_used_vsps or staking_vsps", host)
	}
	return found, nil
}

func resolveKnownVSP(ctx context.Context, host, pubkey string) (*types.VSPInfo, error) {
	found, err := findKnownVSP(ctx, host)
	if err != nil {
		return nil, err
	}
	if found.PubKey != "" && found.PubKey != pubkey {
		return nil, fmt.Errorf("vspPubkey does not match the known key for %q", host)
	}
	return found, nil
}

// vspMaintenanceRun gates and audits one VSP ticket-maintenance write. The
// fees it pays are an operating cost, so they are not counted against the
// grant caps, as for a ticket purchase's VSP fee.
func vspMaintenanceRun(ctx context.Context, a *agent, tool string, allowance middleware.Allowance, in vspTicketMaintenanceInput,
	work func(context.Context, uint32, []byte) (*types.SyncFailedVSPTicketsResponse, error)) (any, error) {
	if in.VSPHost == "" || in.VSPPubkey == "" {
		return nil, fmt.Errorf("vspHost and vspPubkey are required (see staking_vsps)")
	}
	changeAccount := in.ChangeAccount
	if changeAccount == 0 {
		changeAccount = in.Account
	}
	// Reject an ungranted agent before any wallet or network work, so the gate
	// stays first and a call without a grant cannot probe the VSP lists.
	if err := grants.precheckScope(a.id, scopeStaking, time.Now()); err != nil {
		recordSpend(a, tool, in.Account, 0, in.VSPHost, "denied", err.Error())
		return nil, err
	}
	// The fee leaves one account and the change lands in the other.
	for _, acct := range []uint32{in.Account, changeAccount} {
		if err := grants.precheckAccount(a.id, acct, time.Now()); err != nil {
			recordSpend(a, tool, acct, 0, in.VSPHost, "denied", err.Error())
			return nil, err
		}
	}
	// Granted, so the run may spend the shared allowance: one per interval
	// across agent and dashboard, ahead of any VSP or wallet traffic.
	if err := allow(allowance); err != nil {
		return nil, err
	}
	if _, err := resolveKnownVSP(ctx, in.VSPHost, in.VSPPubkey); err != nil {
		recordSpend(a, tool, in.Account, 0, in.VSPHost, "denied", err.Error())
		return nil, err
	}
	pass, err := grants.authorizeVSPRun(a.id, in.Account, changeAccount, time.Now())
	if err != nil {
		recordSpend(a, tool, in.Account, 0, in.VSPHost, "denied", err.Error())
		return nil, err
	}
	defer utils.Zero(pass)

	summary, err := work(ctx, changeAccount, pass)
	if err != nil {
		recordSpend(a, tool, in.Account, 0, in.VSPHost, "error", err.Error())
		return nil, err
	}
	detail := "run complete"
	if summary != nil {
		detail = fmt.Sprintf("paid %d, errored %d, unpaid %d", summary.After.Paid, summary.After.Errored, summary.After.Unpaid)
	}
	recordSpend(a, tool, in.Account, 0, in.VSPHost, "ok", detail)
	return summary, nil
}

// purchaseInput parameterizes staking_purchase.
type purchaseInput struct {
	Account       uint32 `json:"account" jsonschema:"source account number; overridden by the mixed account when privacy is configured"`
	NumTickets    uint32 `json:"numTickets" jsonschema:"number of tickets to buy"`
	VSPHost       string `json:"vspHost" jsonschema:"VSP host URL (from staking_vsps)"`
	VSPPubkey     string `json:"vspPubkey" jsonschema:"VSP public key (from staking_vsps)"`
	ChangeAccount uint32 `json:"changeAccount,omitempty" jsonschema:"change account number; defaults to the source account, and is overridden by the mixed change account when privacy is configured"`
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
			// The probe is an outbound request to whatever host is named, so it
			// is limited to the same VSPs the paying tools accept rather than
			// letting a caller point the dashboard at an arbitrary server.
			if _, err := findKnownVSP(ctx, in.Host); err != nil {
				return nil, err
			}
			return services.GetVSPInfo(ctx, in.Host)
		}),
	readTool("staking", "staking_purchase_status",
		"Report whether a background (mixed) ticket purchase is running plus the most recent terminal result.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.PurchaseStatusSnapshot(), nil }),
	readTool("staking", "staking_autobuyer_status",
		"Get the automatic ticket-buyer status (running flag, last error, persisted settings).",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return services.AutobuyerStatusSnapshot(ctx), nil
		}),
	agentToolDesc("staking", "staking_purchase",
		func(a *agent) string {
			return "Buy staking tickets through a VSP. Requires a spend grant covering both the funding account and the change account; the cost (ticket price x count) is checked against the grant caps. The agent never supplies a passphrase." + stakingHint(a)
		},
		func(ctx context.Context, a *agent, in purchaseInput) (any, error) {
			if in.NumTickets == 0 {
				return nil, fmt.Errorf("numTickets must be at least 1")
			}
			if in.VSPHost == "" || in.VSPPubkey == "" {
				return nil, fmt.Errorf("vspHost and vspPubkey are required (see staking_vsps)")
			}
			// Reject an ungranted agent before any wallet work, so the gate stays
			// first and a call without a grant costs nothing.
			if err := grants.precheckGrant(a.id, time.Now()); err != nil {
				recordSpend(a, "staking_purchase", in.Account, 0, in.VSPHost, "denied", err.Error())
				return nil, err
			}
			// On a privacy wallet the ticket is funded from the mixed account
			// with change to the unmixed one, whatever the agent named. Resolve
			// that once, here: the grant is checked against these accounts and
			// the service spends exactly these, so the two cannot disagree.
			changeAccount := in.ChangeAccount
			if changeAccount == 0 {
				changeAccount = in.Account
			}
			accts, err := services.ResolveTicketAccounts(ctx, in.Account, changeAccount)
			if err != nil {
				recordSpend(a, "staking_purchase", in.Account, 0, in.VSPHost, "error", err.Error())
				return nil, err
			}
			// The ticket is funded from one account and the split's change
			// lands in the other, so both must be covered by the grant.
			for _, acct := range []uint32{accts.Source, accts.Change} {
				if err := grants.precheckAccount(a.id, acct, time.Now()); err != nil {
					recordSpend(a, "staking_purchase", acct, 0, in.VSPHost, "denied", err.Error())
					return nil, err
				}
			}
			// A completed purchase records the VSP in the wallet's used list, so
			// an unconstrained host here would let an agent seed that list and
			// launder its own host into the maintenance tools' allowlist.
			if _, err := resolveKnownVSP(ctx, in.VSPHost, in.VSPPubkey); err != nil {
				recordSpend(a, "staking_purchase", accts.Source, 0, in.VSPHost, "denied", err.Error())
				return nil, err
			}
			info, err := services.FetchStakingInfo(ctx)
			if err != nil {
				return nil, fmt.Errorf("ticket price unavailable: %w", err)
			}
			perTicket, err := dcrutil.NewAmount(info.TicketPrice)
			if err != nil {
				return nil, err
			}
			perTicketAtoms := int64(perTicket)
			// Bound before multiplying: a large numTickets overflows int64 and
			// wraps to a small positive total that clears the caps, while the
			// full count still reaches the wallet, which buys as many tickets as
			// the balance affords rather than failing.
			if perTicketAtoms > 0 && int64(in.NumTickets) > math.MaxInt64/perTicketAtoms {
				return nil, fmt.Errorf("numTickets is too large for the current ticket price")
			}
			costDCR := info.TicketPrice * float64(in.NumTickets)
			totalAtoms := perTicketAtoms * int64(in.NumTickets)
			// Ticket purchases have no recipient address; the allowlist is skipped.
			pass, h, err := grants.authorize(ctx, a.id, accts.Source, totalAtoms, "",
				fmt.Sprintf("buy %d ticket(s) for %s from account %d via %s", in.NumTickets, dcrAmountStr(totalAtoms), accts.Source, in.VSPHost), time.Now())
			if err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "staking_purchase", accts.Source, costDCR, in.VSPHost, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "staking_purchase", accts.Source, costDCR, in.VSPHost, "denied", err.Error())
				}
				return nil, err
			}
			defer utils.Zero(pass)
			resp, err := services.PurchaseTickets(ctx, accts, in.NumTickets, in.VSPHost, in.VSPPubkey, pass)
			if err != nil {
				// Only a failure that provably precedes the spend may release
				// the reservation; see services.ErrSpendStarted.
				detail := err.Error()
				if !errors.Is(err, services.ErrSpendStarted) {
					h.refund(totalAtoms)
				} else {
					detail = "reservation kept, the purchase may still complete: " + detail
				}
				recordSpend(a, "staking_purchase", accts.Source, costDCR, in.VSPHost, "error", detail)
				return nil, err
			}
			recordSpend(a, "staking_purchase", accts.Source, costDCR, in.VSPHost, "ok",
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
			// The autobuyer pays this host's fee on every ticket it buys, so it
			// is constrained the same way an explicit purchase is.
			if _, err := resolveKnownVSP(ctx, in.VSPHost, in.VSPPubkey); err != nil {
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
			if !services.IsAutobuyerRunning() {
				recordSpend(a, "staking_autobuyer_stop", 0, 0, "", "unchanged", "the ticket buyer was not running")
				return map[string]any{"ok": true}, nil
			}
			services.StopAutobuyer()
			recordSpend(a, "staking_autobuyer_stop", 0, 0, "", "ok", "stop requested")
			return map[string]any{"ok": true}, nil
		}),
	agentToolDesc("staking", "staking_sync_failed_vsp_tickets",
		func(a *agent) string {
			return "Retry VSP fee payments for the wallet's failed tickets. Pays real fees: requires a staking grant covering both the fee and change accounts, the VSP must be one this wallet has used or a registry entry, and VSP fees are an operating cost and are not counted against the grant caps. Signs with the held passphrase. One run per 30 seconds, shared with the dashboard." + stakingHint(a)
		},
		func(ctx context.Context, a *agent, in vspTicketMaintenanceInput) (any, error) {
			return vspMaintenanceRun(ctx, a, "staking_sync_failed_vsp_tickets", middleware.VSPSync, in,
				func(ctx context.Context, changeAccount uint32, pass []byte) (*types.SyncFailedVSPTicketsResponse, error) {
					return services.SyncFailedVSPTickets(ctx, in.VSPHost, in.VSPPubkey, in.Account, changeAccount, pass)
				})
		}),
	agentToolDesc("staking", "staking_process_unmanaged_vsp_tickets",
		func(a *agent) string {
			return "Re-associate the wallet's untracked tickets with a VSP. Pays real fees: requires a staking grant covering both the fee and change accounts, the VSP must be one this wallet has used or a registry entry, and VSP fees are an operating cost and are not counted against the grant caps. Signs with the held passphrase. One run per 30 seconds, shared with the dashboard." + stakingHint(a)
		},
		func(ctx context.Context, a *agent, in vspTicketMaintenanceInput) (any, error) {
			return vspMaintenanceRun(ctx, a, "staking_process_unmanaged_vsp_tickets", middleware.VSPUnmanaged, in,
				func(ctx context.Context, changeAccount uint32, pass []byte) (*types.SyncFailedVSPTicketsResponse, error) {
					return services.ProcessUnmanagedVSPTickets(ctx, in.VSPHost, in.VSPPubkey, in.Account, changeAccount, pass)
				})
		}),
}
