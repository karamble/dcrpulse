// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

type explorerAddressInput struct {
	Address string `json:"address"`
}

type explorerTxInput struct {
	TxHash string `json:"txHash"`
}

type explorerHeightInput struct {
	Height int64 `json:"height"`
}

type explorerHashInput struct {
	Hash string `json:"hash"`
}

// explorerTools are the read-only "explorer" domain tools: parameterized
// lookups against the chain via dcrd.
var explorerTools = []toolDef{
	readTool("explorer", "explorer_address",
		"Look up a Decred address: balance and transaction summary. Requires 'address'.",
		func(ctx context.Context, in explorerAddressInput) (any, error) {
			return services.FetchAddressInfo(ctx, in.Address)
		}),
	readTool("explorer", "explorer_transaction",
		"Look up a transaction by its hash. Requires 'txHash'.",
		func(ctx context.Context, in explorerTxInput) (any, error) {
			return services.FetchTransaction(ctx, in.TxHash)
		}),
	readTool("explorer", "explorer_block_by_height",
		"Look up a block by its height. Requires 'height'.",
		func(ctx context.Context, in explorerHeightInput) (any, error) {
			return services.FetchBlockByHeight(ctx, in.Height)
		}),
	readTool("explorer", "explorer_block_by_hash",
		"Look up a block by its hash. Requires 'hash'.",
		func(ctx context.Context, in explorerHashInput) (any, error) {
			return services.FetchBlockByHash(ctx, in.Hash)
		}),
}
