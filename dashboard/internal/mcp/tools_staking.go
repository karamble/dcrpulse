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

// purchaseInput parameterizes staking_purchase.
type purchaseInput struct {
	Account       uint32 `json:"account" jsonschema:"source account number"`
	NumTickets    uint32 `json:"numTickets" jsonschema:"number of tickets to buy"`
	VSPHost       string `json:"vspHost" jsonschema:"VSP host URL (from staking_vsps)"`
	VSPPubkey     string `json:"vspPubkey" jsonschema:"VSP public key (from staking_vsps)"`
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
	agentTool("staking", "staking_purchase",
		"Buy staking tickets through a VSP. Requires a spend grant covering the account; the cost (ticket price x count) is checked against the grant caps. The agent never supplies a passphrase.",
		func(ctx context.Context, a *agent, in purchaseInput) (any, error) {
			if in.NumTickets == 0 {
				return nil, fmt.Errorf("numTickets must be at least 1")
			}
			if in.VSPHost == "" || in.VSPPubkey == "" {
				return nil, fmt.Errorf("vspHost and vspPubkey are required (see staking_vsps)")
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
				recordSpend(a, "staking_purchase", in.Account, costDCR, in.VSPHost, "denied", err.Error())
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
}
