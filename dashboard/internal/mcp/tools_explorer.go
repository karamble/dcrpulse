// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

type explorerAddressInput struct {
	Address string `json:"address" jsonschema:"Decred address to look up"`
}

type explorerTxInput struct {
	TxHash string `json:"txHash" jsonschema:"transaction hash to look up"`
}

type explorerHeightInput struct {
	Height int64 `json:"height" jsonschema:"block height to look up"`
}

type explorerHashInput struct {
	Hash string `json:"hash" jsonschema:"block hash to look up"`
}

// explorerRecentBlocksInput parameterizes explorer_recent_blocks. Both fields
// are optional (omitempty); defaults match the HTTP handler.
type explorerRecentBlocksInput struct {
	Page     int `json:"page,omitempty" jsonschema:"page number, 1-based (default 1)"`
	PageSize int `json:"pageSize,omitempty" jsonschema:"blocks per page (default 10, max 100)"`
}

// explorerSearchInput parameterizes explorer_search.
type explorerSearchInput struct {
	Query string `json:"query" jsonschema:"search query: block height, block hash, transaction hash, or address"`
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
	readTool("explorer", "explorer_recent_blocks",
		"List recent blocks, newest first, with pagination. Optional page (default 1) and pageSize (default 10, max 100).",
		func(ctx context.Context, in explorerRecentBlocksInput) (any, error) {
			page := in.Page
			if page <= 0 {
				page = 1
			}
			pageSize := in.PageSize
			if pageSize <= 0 {
				pageSize = 10
			}
			if pageSize > 100 {
				pageSize = 100
			}
			return services.FetchRecentBlocksPaginated(ctx, page, pageSize)
		}),
	readTool("explorer", "explorer_mempool",
		"List the current mempool transactions awaiting confirmation.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return services.FetchMempoolTransactions(ctx)
		}),
	readTool("explorer", "explorer_search",
		"Universal chain search: resolves a block height, block hash, transaction hash, or address to its details. Requires 'query'.",
		func(ctx context.Context, in explorerSearchInput) (any, error) {
			return services.UniversalSearch(ctx, in.Query)
		}),
}
