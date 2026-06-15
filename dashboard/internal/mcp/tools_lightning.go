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

type lnPayInput struct {
	PayReq      string  `json:"payReq" jsonschema:"bolt11 invoice to pay"`
	AmountDCR   float64 `json:"amountDcr,omitempty" jsonschema:"amount in DCR, required only for zero-amount invoices"`
	FeeLimitDCR float64 `json:"feeLimitDcr,omitempty" jsonschema:"optional maximum routing fee in DCR"`
}

type lnInvoiceInput struct {
	AmountDCR float64 `json:"amountDcr,omitempty" jsonschema:"invoice amount in DCR (0 = any)"`
	Memo      string  `json:"memo,omitempty" jsonschema:"optional memo"`
}

type lnOpenChannelInput struct {
	PeerURI  string  `json:"peerUri" jsonschema:"pubkey@host:port or bare node pubkey"`
	LocalDCR float64 `json:"localDcr" jsonschema:"local funding amount in DCR"`
	PushDCR  float64 `json:"pushDcr,omitempty" jsonschema:"optional amount to push to the remote, in DCR"`
	Private  bool    `json:"private,omitempty" jsonschema:"open as a private channel"`
}

// lightningTools are the lightning domain tools: reads plus grant-gated spend
// actions (pay, open channel) and invoice creation.
var lightningTools = []toolDef{
	readTool("lightning", "lightning_info",
		"Get the Lightning node info (identity pubkey, sync state, channel and peer counts).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetLightningInfo(ctx) }),
	readTool("lightning", "lightning_balance",
		"Get Lightning on-chain and channel balances.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetLightningBalance(ctx) }),
	readTool("lightning", "lightning_channels",
		"List Lightning channels and their state.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListLightningChannels(ctx) }),
	readTool("lightning", "lightning_activity",
		"Get a summary of recent Lightning activity (payments and invoices).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetLightningActivity(ctx) }),
	readTool("lightning", "lightning_payments",
		"List Lightning payments sent from this node.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListLightningPayments(ctx) }),
	readTool("lightning", "lightning_invoices",
		"List Lightning invoices created on this node.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListLightningInvoices(ctx) }),
	agentTool("lightning", "ln_pay",
		"Pay a Lightning (bolt11) invoice. Requires a spend grant with Lightning enabled; the amount counts against the grant's daily cap. dcrlnd must be unlocked.",
		func(ctx context.Context, a *agent, in lnPayInput) (any, error) {
			dec, err := services.DecodeLightningInvoice(ctx, in.PayReq)
			if err != nil {
				return nil, fmt.Errorf("decode invoice: %w", err)
			}
			amtAtoms := dec.NumAtoms
			if amtAtoms <= 0 {
				amt, err := dcrutil.NewAmount(in.AmountDCR)
				if err != nil || int64(amt) <= 0 {
					return nil, fmt.Errorf("this invoice has no amount; provide amountDcr")
				}
				amtAtoms = int64(amt)
			}
			amtDCR := dcrutil.Amount(amtAtoms).ToCoin()
			if err := grants.authorizeLightning(a.id, amtAtoms, time.Now()); err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "ln_pay", 0, amtDCR, dec.Destination, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "ln_pay", 0, amtDCR, dec.Destination, "denied", err.Error())
				}
				return nil, err
			}
			req := &types.LightningSendPaymentRequest{PayReq: in.PayReq}
			if dec.NumAtoms <= 0 {
				req.Amt = amtAtoms
			}
			if f, err := dcrutil.NewAmount(in.FeeLimitDCR); err == nil && int64(f) > 0 {
				req.FeeLimitAtoms = int64(f)
			}
			ch, err := services.StreamLightningPayment(ctx, req)
			if err != nil {
				grants.refund(a.id, amtAtoms)
				recordSpend(a, "ln_pay", 0, amtDCR, dec.Destination, "error", err.Error())
				return nil, err
			}
			var last types.LightningPayment
			for p := range ch {
				last = p
				if p.Status == "confirmed" || p.Status == "failed" {
					break
				}
			}
			if last.Status != "confirmed" {
				grants.refund(a.id, amtAtoms)
				reason := last.Status
				if reason == "" {
					reason = "incomplete"
				}
				recordSpend(a, "ln_pay", 0, amtDCR, dec.Destination, "error", "payment "+reason)
				return nil, fmt.Errorf("payment %s", reason)
			}
			recordSpend(a, "ln_pay", 0, amtDCR, dec.Destination, "ok", last.PaymentHash)
			return last, nil
		}),
	agentTool("lightning", "ln_add_invoice",
		"Create a Lightning invoice to receive payment. Requires a spend grant with Lightning enabled.",
		func(ctx context.Context, a *agent, in lnInvoiceInput) (any, error) {
			if err := grants.authorizeLightningAction(a.id, time.Now()); err != nil {
				recordSpend(a, "ln_add_invoice", 0, in.AmountDCR, "", "denied", err.Error())
				return nil, err
			}
			var atoms int64
			if in.AmountDCR > 0 {
				amt, err := dcrutil.NewAmount(in.AmountDCR)
				if err != nil {
					return nil, fmt.Errorf("invalid amount: %w", err)
				}
				atoms = int64(amt)
			}
			inv, err := services.AddLightningInvoice(ctx, &types.LightningAddInvoiceRequest{Memo: in.Memo, ValueAtoms: atoms})
			if err != nil {
				recordSpend(a, "ln_add_invoice", 0, in.AmountDCR, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "ln_add_invoice", 0, in.AmountDCR, "", "ok", inv.RHashHex)
			return inv, nil
		}),
	agentTool("lightning", "ln_open_channel",
		"Open a Lightning channel, funding it from the Lightning wallet. Requires a spend grant with Lightning enabled; the funding amount counts against the daily cap.",
		func(ctx context.Context, a *agent, in lnOpenChannelInput) (any, error) {
			local, err := dcrutil.NewAmount(in.LocalDCR)
			if err != nil || int64(local) <= 0 {
				return nil, fmt.Errorf("localDcr must be positive")
			}
			localAtoms := int64(local)
			if err := grants.authorizeLightning(a.id, localAtoms, time.Now()); err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "ln_open_channel", 0, in.LocalDCR, in.PeerURI, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "ln_open_channel", 0, in.LocalDCR, in.PeerURI, "denied", err.Error())
				}
				return nil, err
			}
			req := &types.OpenChannelRequest{PeerURI: in.PeerURI, LocalAtoms: localAtoms, Private: in.Private}
			if in.PushDCR > 0 {
				if p, err := dcrutil.NewAmount(in.PushDCR); err == nil {
					req.PushAtoms = int64(p)
				}
			}
			resp, err := services.OpenLightningChannel(ctx, req)
			if err != nil {
				grants.refund(a.id, localAtoms)
				recordSpend(a, "ln_open_channel", 0, in.LocalDCR, in.PeerURI, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "ln_open_channel", 0, in.LocalDCR, in.PeerURI, "ok", resp.FundingTxid)
			return resp, nil
		}),
}
