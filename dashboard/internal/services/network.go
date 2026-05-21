// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"dcrpulse/internal/rpc"
)

var (
	networkOnce sync.Once
	networkVal  string
	networkErr  error
)

// CurrentNetwork returns "mainnet" or "testnet", lazily resolved via
// dcrd's getblockchaininfo. The result is cached for the process
// lifetime since the chain identity doesn't change at runtime.
func CurrentNetwork(ctx context.Context) (string, error) {
	networkOnce.Do(func() {
		if rpc.DcrdClient == nil {
			networkErr = fmt.Errorf("dcrd client not initialized")
			return
		}
		info, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
		if err != nil {
			networkErr = fmt.Errorf("get blockchain info: %w", err)
			return
		}
		chain := strings.ToLower(strings.TrimSpace(info.Chain))
		switch {
		case strings.Contains(chain, "main"):
			networkVal = "mainnet"
		case strings.Contains(chain, "test"):
			networkVal = "testnet"
		case strings.Contains(chain, "sim"):
			networkVal = "simnet"
		default:
			networkVal = chain
		}
	})
	return networkVal, networkErr
}
