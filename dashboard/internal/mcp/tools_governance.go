// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

// governanceTools are the read-only "governance" domain tools: consensus
// agendas, treasury voting policies, and Politeia proposals.
var governanceTools = []toolDef{
	readTool("governance", "governance_agendas",
		"List consensus voting agendas and this wallet's current vote choices.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListAgendas(ctx) }),
	readTool("governance", "governance_treasury_policies",
		"List the wallet's treasury key (Pi key) voting policies.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListTreasuryKeyPolicies(ctx) }),
	readTool("governance", "governance_tspend_policies",
		"List the wallet's treasury-spend (TSpend) voting policies.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListTSpendPolicies(ctx) }),
	readTool("governance", "governance_proposals",
		"List Politeia governance proposals.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			proposals, _, err := services.ListProposals(ctx)
			return proposals, err
		}),
}
