// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/rpc"
)

type dexOrdersInput struct {
	Host string `json:"host,omitempty" jsonschema:"optional DEX host filter; empty returns all"`
}

type dexNotificationsInput struct {
	Count int `json:"count,omitempty" jsonschema:"max notifications to return (default 50)"`
}

// dexTools are the read-only "dex" domain tools. They use the bisonw RPC client
// reads that do not require the DEX app password. Placing/canceling orders and
// running the market-maker bot (state-changing) come in a later phase.
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
}
