// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

// nodeTools are the read-only "node" domain tools. This is the only domain an
// agent can use before the user grants it anything else.
var nodeTools = []toolDef{
	readTool("node", "node_status",
		"Get the dcrd node sync status: synced state, block height, peer count, and version.",
		func(_ context.Context, _ emptyInput) (any, error) { return services.FetchNodeStatus() }),
	readTool("node", "node_dashboard",
		"Get the node dashboard summary: chain, circulating supply, staking and treasury overview.",
		func(_ context.Context, _ emptyInput) (any, error) { return services.FetchDashboardData() }),
	readTool("node", "node_blockchain_info",
		"Get detailed blockchain info from dcrd (best block, difficulty, chainwork, etc.).",
		func(_ context.Context, _ emptyInput) (any, error) { return services.FetchBlockchainInfo() }),
	readTool("node", "node_network",
		"Get network info: peer count and network hashrate.",
		func(_ context.Context, _ emptyInput) (any, error) { return services.FetchNetworkInfo() }),
	readTool("node", "node_peers",
		"List the dcrd node's currently connected peers.",
		func(_ context.Context, _ emptyInput) (any, error) { return services.FetchPeers() }),
	readTool("node", "node_supply",
		"Get the Decred coin supply overview (circulating, staked, treasury, mixed percentage).",
		func(_ context.Context, _ emptyInput) (any, error) { return services.FetchSupplyInfo() }),
	readTool("node", "node_staking_overview",
		"Get network-wide staking info: ticket price, pool size, participation rate, and vote stats.",
		func(_ context.Context, _ emptyInput) (any, error) { return services.FetchStakingInfo() }),
	readTool("node", "node_mempool",
		"Get current mempool info (transaction count, size in bytes).",
		func(_ context.Context, _ emptyInput) (any, error) { return services.FetchMempoolInfo() }),
}
