// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
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
func resolveKnownVSP(ctx context.Context, host, pubkey string) (*types.VSPInfo, error) {
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
	if found.PubKey != "" && found.PubKey != pubkey {
		return nil, fmt.Errorf("vspPubkey does not match the known key for %q", host)
	}
	return found, nil
}

// vspFeeCandidates reports the tickets a maintenance run may pay a fee for and
// the sum of their ticket prices, which is what a VSP fee is a percentage of.
// Each ticket carries its own price, which matters when older tickets were
// bought at a very different price than today's.
func vspFeeCandidates(tickets []types.TicketRecord, unmanaged bool) (int, float64) {
	var count int
	var priceDCR float64
	for _, t := range tickets {
		var candidate bool
		if unmanaged {
			// Tickets no VSP is tracking at all.
			candidate = t.FeeStatus == "" &&
				(t.Status == "UNMINED" || t.Status == "IMMATURE" || t.Status == "LIVE")
		} else {
			// Errored fees are what the sync retries; unpaid ones are counted
			// too so the ceiling errs high and the refund gives the rest back.
			candidate = t.FeeStatus == "ERRORED" || t.FeeStatus == "UNPAID"
		}
		if !candidate {
			continue
		}
		count++
		priceDCR += t.TicketPrice
	}
	return count, priceDCR
}

// vspMaintenanceRun gates, meters and settles one VSP ticket-maintenance write.
// The wallet RPCs behind these tools report no amounts, so the worst-case fee is
// reserved up front and the share belonging to tickets the run did not resolve
// is refunded afterwards, the same shape ln_pay uses for its routing fee.
func vspMaintenanceRun(ctx context.Context, a *agent, tool string, in vspTicketMaintenanceInput, unmanaged bool,
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
	vsp, err := resolveKnownVSP(ctx, in.VSPHost, in.VSPPubkey)
	if err != nil {
		recordSpend(a, tool, in.Account, 0, in.VSPHost, "denied", err.Error())
		return nil, err
	}
	// Take the higher of the listed and the live-advertised fee, so a host
	// cannot shrink the ceiling it is measured against by advertising low.
	feePct := vsp.FeePercentage
	if live, lerr := services.GetVSPInfo(ctx, in.VSPHost); lerr == nil && live != nil && live.FeePercentage > feePct {
		feePct = live.FeePercentage
	}
	tickets, err := services.ListTickets(ctx)
	if err != nil {
		return nil, fmt.Errorf("ticket list unavailable: %w", err)
	}
	count, priceDCR := vspFeeCandidates(tickets, unmanaged)
	noun := "failed"
	if unmanaged {
		noun = "untracked"
	}
	// No local candidates does not mean nothing will be paid: the run settles
	// whatever the VSP still asks for, and a wallet that cannot report ticket fee
	// status looks exactly like a wallet with nothing to pay. Refuse rather than
	// authorize a run whose cost has no ceiling.
	if count == 0 {
		err := fmt.Errorf("the wallet reports no %s tickets needing a VSP fee, so this run's cost cannot be bounded; check that it is synced and reporting ticket fee status", noun)
		recordSpend(a, tool, in.Account, 0, in.VSPHost, "denied", err.Error())
		return nil, err
	}
	if feePct <= 0 {
		err := fmt.Errorf("could not determine the fee %q charges, so the cost cannot be bounded", in.VSPHost)
		recordSpend(a, tool, in.Account, 0, in.VSPHost, "denied", err.Error())
		return nil, err
	}
	ceiling, err := dcrutil.NewAmount(priceDCR * feePct / 100)
	if err != nil {
		return nil, fmt.Errorf("fee ceiling is out of range: %w", err)
	}
	ceilingAtoms := int64(ceiling)
	action := fmt.Sprintf("pay up to %s in VSP fees for %d %s ticket(s) at %s",
		dcrAmountStr(ceilingAtoms), count, noun, in.VSPHost)
	pass, err := grants.authorizeVSPFees(ctx, a.id, in.Account, changeAccount, ceilingAtoms, action, time.Now())
	if err != nil {
		if tripwire(a.id, err) {
			recordSpend(a, tool, in.Account, ceiling.ToCoin(), in.VSPHost, "blocked", "spend-limit violation: grant revoked and token blocked")
		} else {
			recordSpend(a, tool, in.Account, ceiling.ToCoin(), in.VSPHost, "denied", err.Error())
		}
		return nil, err
	}
	defer zero(pass)

	summary, workErr := work(ctx, changeAccount, pass)

	// The run pays per ticket as it goes, so even a failure part-way through may
	// have spent. Settle against what is still outstanding instead of assuming
	// either extreme: refund the ceiling's share of the tickets it did not
	// resolve and leave the rest counted.
	charged, resolved, settled := ceilingAtoms, 0, false
	if after, lerr := services.ListTickets(ctx); lerr == nil {
		remaining, remainingPrice := vspFeeCandidates(after, unmanaged)
		if unused, uerr := dcrutil.NewAmount(remainingPrice * feePct / 100); uerr == nil {
			refund := int64(unused)
			if refund > ceilingAtoms {
				refund = ceilingAtoms
			}
			grants.refund(a.id, refund)
			charged = ceilingAtoms - refund
			if resolved = count - remaining; resolved < 0 {
				resolved = 0
			}
			settled = true
		}
	}
	chargedDCR := dcrutil.Amount(charged).ToCoin()
	if workErr != nil {
		detail := workErr.Error()
		if !settled {
			// Nothing to reconcile against and fees may already have moved, so
			// the reservation stands rather than handing back headroom.
			detail += " (fees could not be reconciled; the full ceiling stays counted against the daily cap)"
		}
		recordSpend(a, tool, in.Account, chargedDCR, in.VSPHost, "error", detail)
		return nil, workErr
	}
	detail := fmt.Sprintf("%d ticket(s) processed", resolved)
	if !settled {
		detail = "ticket count unavailable; the full ceiling stays counted against the daily cap"
	}
	recordSpend(a, tool, in.Account, chargedDCR, in.VSPHost, "ok", detail)
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
			// The service overrides the caller's account when privacy is
			// configured, funding the ticket from the mixed account. Resolve that
			// here so the grant is checked against the account the purchase
			// actually spends from, not the one the agent named.
			srcAccount := in.Account
			changeAccount := in.ChangeAccount
			if changeAccount == 0 {
				changeAccount = in.Account
			}
			if mixing, mixed := services.TicketMixingParams(ctx); mixed {
				srcAccount = mixing.Mixed
				changeAccount = mixing.Change
			}
			// The ticket is funded from one account and the split's change
			// lands in the other, so both must be covered by the grant.
			for _, acct := range []uint32{srcAccount, changeAccount} {
				if err := grants.precheckAccount(a.id, acct, time.Now()); err != nil {
					recordSpend(a, "staking_purchase", acct, 0, in.VSPHost, "denied", err.Error())
					return nil, err
				}
			}
			// A completed purchase records the VSP in the wallet's used list, so
			// an unconstrained host here would let an agent seed that list and
			// launder its own host into the maintenance tools' allowlist.
			if _, err := resolveKnownVSP(ctx, in.VSPHost, in.VSPPubkey); err != nil {
				recordSpend(a, "staking_purchase", srcAccount, 0, in.VSPHost, "denied", err.Error())
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
			pass, err := grants.authorize(ctx, a.id, srcAccount, totalAtoms, "", time.Now())
			if err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "staking_purchase", srcAccount, costDCR, in.VSPHost, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "staking_purchase", srcAccount, costDCR, in.VSPHost, "denied", err.Error())
				}
				return nil, err
			}
			defer zero(pass)
			resp, err := services.PurchaseTickets(ctx, in.Account, in.NumTickets, in.VSPHost, in.VSPPubkey, changeAccount, pass)
			if err != nil {
				grants.refund(a.id, totalAtoms)
				recordSpend(a, "staking_purchase", srcAccount, costDCR, in.VSPHost, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "staking_purchase", srcAccount, costDCR, in.VSPHost, "ok",
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
			services.StopAutobuyer()
			recordSpend(a, "staking_autobuyer_stop", 0, 0, "", "ok", "")
			return map[string]any{"ok": true}, nil
		}),
	agentToolDesc("staking", "staking_sync_failed_vsp_tickets",
		func(a *agent) string {
			return "Retry VSP fee payments for the wallet's failed tickets. Pays real fees: requires a staking grant covering both the fee and change accounts, the VSP must be one this wallet has used or a registry entry, and the worst-case fee is reserved against the grant caps (the unused part is returned once the run settles). Signs with the held passphrase." + stakingHint(a)
		},
		func(ctx context.Context, a *agent, in vspTicketMaintenanceInput) (any, error) {
			return vspMaintenanceRun(ctx, a, "staking_sync_failed_vsp_tickets", in, false,
				func(ctx context.Context, changeAccount uint32, pass []byte) (*types.SyncFailedVSPTicketsResponse, error) {
					return services.SyncFailedVSPTickets(ctx, in.VSPHost, in.VSPPubkey, in.Account, changeAccount, pass)
				})
		}),
	agentToolDesc("staking", "staking_process_unmanaged_vsp_tickets",
		func(a *agent) string {
			return "Re-associate the wallet's untracked tickets with a VSP. Pays real fees: requires a staking grant covering both the fee and change accounts, the VSP must be one this wallet has used or a registry entry, and the worst-case fee is reserved against the grant caps (the unused part is returned once the run settles). Signs with the held passphrase." + stakingHint(a)
		},
		func(ctx context.Context, a *agent, in vspTicketMaintenanceInput) (any, error) {
			return vspMaintenanceRun(ctx, a, "staking_process_unmanaged_vsp_tickets", in, true,
				func(ctx context.Context, changeAccount uint32, pass []byte) (*types.SyncFailedVSPTicketsResponse, error) {
					return services.ProcessUnmanagedVSPTickets(ctx, in.VSPHost, in.VSPPubkey, in.Account, changeAccount, pass)
				})
		}),
}
