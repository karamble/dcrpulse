// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

// lightningTools are the read-only "lightning" domain tools. Channel opens,
// payments, and invoice creation (state-changing) come in a later phase.
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
}
