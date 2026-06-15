// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/pkg/bisonw"
)

type dexOrdersInput struct {
	Host string `json:"host,omitempty" jsonschema:"optional DEX host filter; empty returns all"`
}

type dexNotificationsInput struct {
	Count int `json:"count,omitempty" jsonschema:"max notifications to return (default 50)"`
}

type dexPlaceOrderInput struct {
	Host    string `json:"host" jsonschema:"DEX server host"`
	Base    uint32 `json:"base" jsonschema:"base asset id"`
	Quote   uint32 `json:"quote" jsonschema:"quote asset id"`
	Sell    bool   `json:"sell" jsonschema:"true to sell the base asset, false to buy it"`
	IsLimit bool   `json:"isLimit" jsonschema:"true for a limit order, false for market"`
	Qty     uint64 `json:"qty" jsonschema:"quantity in atomic units of the base asset"`
	Rate    uint64 `json:"rate,omitempty" jsonschema:"rate in atomic message-rate units (limit orders)"`
	TifNow  bool   `json:"tifNow,omitempty" jsonschema:"immediate (taker) if true, standing (maker) if false"`
}

type dexCancelInput struct {
	OrderID string `json:"orderId" jsonschema:"hex order id to cancel"`
}

// dexTools are the dex domain tools: reads that need no app password, plus
// grant-gated order placement/cancel via the unlocked DEX.
var dexTools = []toolDef{
	readTool("dex", "dex_version",
		"Get the bisonw (DCRDEX) version and confirm the DEX client is reachable.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.Version(ctx)
		}),
	readTool("dex", "dex_exchanges",
		"List the configured DEX servers and their markets.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.Exchanges(ctx)
		}),
	readTool("dex", "dex_wallets",
		"List the DEX wallets and their balances and state.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.Wallets(ctx)
		}),
	readTool("dex", "dex_orders",
		"List the user's active and recent DEX orders. Optional 'host' to filter by DEX server.",
		func(ctx context.Context, in dexOrdersInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.MyOrders(ctx, in.Host)
		}),
	readTool("dex", "dex_notifications",
		"List recent DEX notifications. Optional count (default 50).",
		func(ctx context.Context, in dexNotificationsInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			n := in.Count
			if n <= 0 {
				n = 50
			}
			return c.Notifications(ctx, n)
		}),
	agentTool("dex", "dex_place_order",
		"Place a DCRDEX trade order. Requires a spend grant with DEX trading enabled and the DEX unlocked. Quantity and rate are in atomic units.",
		func(ctx context.Context, a *agent, in dexPlaceOrderInput) (any, error) {
			detail := fmt.Sprintf("base=%d quote=%d qty=%d", in.Base, in.Quote, in.Qty)
			if err := grants.authorizeDex(a.id, time.Now()); err != nil {
				recordSpend(a, "dex_place_order", 0, 0, in.Host, "denied", err.Error())
				return nil, err
			}
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				err := fmt.Errorf("DEX is locked; ask the user to unlock it in the dashboard")
				recordSpend(a, "dex_place_order", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			raw, err := client.Trade(ctx, bisonw.TradeParams{
				AppPass: appPass,
				Host:    in.Host,
				IsLimit: in.IsLimit,
				Sell:    in.Sell,
				Base:    in.Base,
				Quote:   in.Quote,
				Qty:     in.Qty,
				Rate:    in.Rate,
				TifNow:  in.TifNow,
			})
			if err != nil {
				recordSpend(a, "dex_place_order", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_place_order", 0, 0, in.Host, "ok", detail)
			return raw, nil
		}),
	agentTool("dex", "dex_cancel_order",
		"Cancel a standing DCRDEX order. Requires a spend grant with DEX trading enabled.",
		func(ctx context.Context, a *agent, in dexCancelInput) (any, error) {
			if err := grants.authorizeDex(a.id, time.Now()); err != nil {
				recordSpend(a, "dex_cancel_order", 0, 0, in.OrderID, "denied", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			if err := client.Cancel(ctx, in.OrderID); err != nil {
				recordSpend(a, "dex_cancel_order", 0, 0, in.OrderID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_cancel_order", 0, 0, in.OrderID, "ok", "")
			return map[string]any{"orderId": in.OrderID, "ok": true}, nil
		}),
}
