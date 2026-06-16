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

type lnDecodeInvoiceInput struct {
	PayReq string `json:"payReq" jsonschema:"bolt11 invoice to decode"`
}

type lnGraphNodeInput struct {
	PubKey string `json:"pubKey" jsonschema:"node identity pubkey (hex)"`
}

type lnGraphSearchInput struct {
	Query string `json:"query,omitempty" jsonschema:"substring to match against node alias or pubkey"`
}

type lnGraphRoutesInput struct {
	PubKey    string  `json:"pubKey" jsonschema:"destination node identity pubkey (hex)"`
	AmountDCR float64 `json:"amountDcr" jsonschema:"amount to route in DCR"`
}

type lnLiquidityEstimateInput struct {
	ChanSizeDCR float64 `json:"chanSizeDcr" jsonschema:"inbound channel size in DCR"`
	Server      string  `json:"server,omitempty" jsonschema:"optional liquidity provider server; blank uses the network default"`
	CertPEM     string  `json:"certPem,omitempty" jsonschema:"optional provider TLS cert (PEM)"`
}

type lnLiquidityRequestInput struct {
	ChanSizeDCR    float64 `json:"chanSizeDcr" jsonschema:"inbound channel size in DCR"`
	ApprovedFeeDCR float64 `json:"approvedFeeDcr" jsonschema:"maximum provider fee in DCR the request may pay; aborts if exceeded"`
	Server         string  `json:"server,omitempty" jsonschema:"optional liquidity provider server; blank uses the network default"`
	CertPEM        string  `json:"certPem,omitempty" jsonschema:"optional provider TLS cert (PEM)"`
}

type lnCloseChannelInput struct {
	ChannelPoint string `json:"channelPoint" jsonschema:"channel point in txid:index form"`
	Force        bool   `json:"force,omitempty" jsonschema:"force close without cooperation from the remote"`
}

type lnCancelInvoiceInput struct {
	PaymentHash string `json:"paymentHash" jsonschema:"payment hash (hex) of the open invoice to cancel"`
}

type lnWatchtowerAddInput struct {
	PubKey  string `json:"pubKey" jsonschema:"watchtower identity pubkey (hex)"`
	Address string `json:"address" jsonschema:"watchtower address (host:port)"`
}

type lnWatchtowerRemoveInput struct {
	PubKey string `json:"pubKey" jsonschema:"watchtower identity pubkey (hex)"`
}

type lnAutopilotSetInput struct {
	Active bool `json:"active" jsonschema:"true to enable autopilot, false to disable"`
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
			if err := grants.authorizeAction(a.id, scopeLightning, time.Now()); err != nil {
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
	readTool("lightning", "ln_decode_invoice",
		"Decode a Lightning (bolt11) invoice into its fields (destination, amount, expiry, description) without paying it.",
		func(ctx context.Context, in lnDecodeInvoiceInput) (any, error) {
			return services.DecodeLightningInvoice(ctx, in.PayReq)
		}),
	readTool("lightning", "ln_peer_presets",
		"List recommended Lightning peer presets (hub nodes) for opening channels.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return services.LightningPeerPresets(ctx), nil
		}),
	readTool("lightning", "ln_liquidity_defaults",
		"Get the built-in liquidity provider defaults (server and cert) for the active network.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetLiquidityDefaults(ctx) }),
	readTool("lightning", "ln_liquidity_estimate",
		"Estimate the provider fee and policy for an inbound liquidity channel of a given size, without paying anything.",
		func(ctx context.Context, in lnLiquidityEstimateInput) (any, error) {
			size, err := dcrutil.NewAmount(in.ChanSizeDCR)
			if err != nil || int64(size) <= 0 {
				return nil, fmt.Errorf("chanSizeDcr must be positive")
			}
			req := &types.RequestLiquidityEstimateRequest{
				ChanSizeAtoms: int64(size),
				Server:        in.Server,
				CertPEM:       in.CertPEM,
			}
			return services.EstimateLiquidityChannel(ctx, req)
		}),
	readTool("lightning", "ln_autopilot_status",
		"Get the Lightning autopilot status (whether automatic channel management is active).",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetLightningAutopilotStatus(ctx) }),
	readTool("lightning", "ln_graph_node",
		"Get details for one node in the Lightning channel graph by its identity pubkey.",
		func(ctx context.Context, in lnGraphNodeInput) (any, error) {
			return services.QueryLightningNodeInfo(ctx, in.PubKey)
		}),
	readTool("lightning", "ln_graph_search",
		"Search the Lightning channel graph for nodes matching a substring (alias or pubkey).",
		func(ctx context.Context, in lnGraphSearchInput) (any, error) {
			return services.SearchLightningNodes(ctx, in.Query)
		}),
	readTool("lightning", "ln_graph_routes",
		"Query candidate payment routes to a destination node for a given amount.",
		func(ctx context.Context, in lnGraphRoutesInput) (any, error) {
			amt, err := dcrutil.NewAmount(in.AmountDCR)
			if err != nil || int64(amt) <= 0 {
				return nil, fmt.Errorf("amountDcr must be positive")
			}
			return services.QueryLightningRoutes(ctx, in.PubKey, int64(amt))
		}),
	readTool("lightning", "ln_network",
		"Get global Lightning network statistics from the channel graph.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.GetLightningNetworkInfo(ctx) }),
	readTool("lightning", "ln_watchtowers",
		"List the watchtowers registered with this Lightning node.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListLightningWatchtowers(ctx) }),
	agentTool("lightning", "ln_close_channel",
		"Close a Lightning channel. Requires a spend grant with Lightning enabled. dcrlnd must be unlocked.",
		func(ctx context.Context, a *agent, in lnCloseChannelInput) (any, error) {
			if in.ChannelPoint == "" {
				return nil, fmt.Errorf("channelPoint required")
			}
			if err := grants.authorizeAction(a.id, scopeLightning, time.Now()); err != nil {
				recordSpend(a, "ln_close_channel", 0, 0, in.ChannelPoint, "denied", err.Error())
				return nil, err
			}
			resp, err := services.CloseLightningChannel(ctx, in.ChannelPoint, in.Force)
			if err != nil {
				recordSpend(a, "ln_close_channel", 0, 0, in.ChannelPoint, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "ln_close_channel", 0, 0, in.ChannelPoint, "ok", resp.ClosingTxid)
			return resp, nil
		}),
	agentTool("lightning", "ln_cancel_invoice",
		"Cancel an open Lightning invoice by its payment hash. Requires a spend grant with Lightning enabled.",
		func(ctx context.Context, a *agent, in lnCancelInvoiceInput) (any, error) {
			if in.PaymentHash == "" {
				return nil, fmt.Errorf("paymentHash required")
			}
			if err := grants.authorizeAction(a.id, scopeLightning, time.Now()); err != nil {
				recordSpend(a, "ln_cancel_invoice", 0, 0, in.PaymentHash, "denied", err.Error())
				return nil, err
			}
			if err := services.CancelLightningInvoice(ctx, in.PaymentHash); err != nil {
				recordSpend(a, "ln_cancel_invoice", 0, 0, in.PaymentHash, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "ln_cancel_invoice", 0, 0, in.PaymentHash, "ok", "")
			return map[string]any{"canceled": true}, nil
		}),
	agentTool("lightning", "ln_watchtower_add",
		"Register a watchtower with this Lightning node. Requires a spend grant with Lightning enabled.",
		func(ctx context.Context, a *agent, in lnWatchtowerAddInput) (any, error) {
			if in.PubKey == "" || in.Address == "" {
				return nil, fmt.Errorf("pubKey and address required")
			}
			if err := grants.authorizeAction(a.id, scopeLightning, time.Now()); err != nil {
				recordSpend(a, "ln_watchtower_add", 0, 0, in.PubKey, "denied", err.Error())
				return nil, err
			}
			if err := services.AddLightningWatchtower(ctx, in.PubKey, in.Address); err != nil {
				recordSpend(a, "ln_watchtower_add", 0, 0, in.PubKey, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "ln_watchtower_add", 0, 0, in.PubKey, "ok", in.Address)
			return map[string]any{"added": true}, nil
		}),
	agentTool("lightning", "ln_watchtower_remove",
		"Deregister a watchtower from this Lightning node. Requires a spend grant with Lightning enabled.",
		func(ctx context.Context, a *agent, in lnWatchtowerRemoveInput) (any, error) {
			if in.PubKey == "" {
				return nil, fmt.Errorf("pubKey required")
			}
			if err := grants.authorizeAction(a.id, scopeLightning, time.Now()); err != nil {
				recordSpend(a, "ln_watchtower_remove", 0, 0, in.PubKey, "denied", err.Error())
				return nil, err
			}
			if err := services.RemoveLightningWatchtower(ctx, in.PubKey); err != nil {
				recordSpend(a, "ln_watchtower_remove", 0, 0, in.PubKey, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "ln_watchtower_remove", 0, 0, in.PubKey, "ok", "")
			return map[string]any{"removed": true}, nil
		}),
	agentTool("lightning", "ln_autopilot_set",
		"Enable or disable Lightning autopilot (automatic channel management). Requires a spend grant with Lightning enabled.",
		func(ctx context.Context, a *agent, in lnAutopilotSetInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeLightning, time.Now()); err != nil {
				recordSpend(a, "ln_autopilot_set", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			if err := services.SetLightningAutopilotStatus(ctx, in.Active); err != nil {
				recordSpend(a, "ln_autopilot_set", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "ln_autopilot_set", 0, 0, "", "ok", fmt.Sprintf("active=%v", in.Active))
			return map[string]any{"active": in.Active}, nil
		}),
	agentTool("lightning", "ln_liquidity_request",
		"Request an inbound liquidity channel from a provider. This pays the provider a fee, which counts against the spend grant's daily cap. Requires a spend grant with Lightning enabled.",
		func(ctx context.Context, a *agent, in lnLiquidityRequestInput) (any, error) {
			size, err := dcrutil.NewAmount(in.ChanSizeDCR)
			if err != nil || int64(size) <= 0 {
				return nil, fmt.Errorf("chanSizeDcr must be positive")
			}
			fee, err := dcrutil.NewAmount(in.ApprovedFeeDCR)
			if err != nil || int64(fee) <= 0 {
				return nil, fmt.Errorf("approvedFeeDcr must be positive")
			}
			feeAtoms := int64(fee)
			feeDCR := dcrutil.Amount(feeAtoms).ToCoin()
			if err := grants.authorizeLightning(a.id, feeAtoms, time.Now()); err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "ln_liquidity_request", 0, feeDCR, in.Server, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "ln_liquidity_request", 0, feeDCR, in.Server, "denied", err.Error())
				}
				return nil, err
			}
			req := &types.RequestLiquidityRequest{
				ChanSizeAtoms:    int64(size),
				ApprovedFeeAtoms: feeAtoms,
				Server:           in.Server,
				CertPEM:          in.CertPEM,
			}
			resp, err := services.RequestLiquidityChannel(ctx, req)
			if err != nil {
				grants.refund(a.id, feeAtoms)
				recordSpend(a, "ln_liquidity_request", 0, feeDCR, in.Server, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "ln_liquidity_request", 0, feeDCR, in.Server, "ok", resp.ChannelPoint)
			return resp, nil
		}),
}
