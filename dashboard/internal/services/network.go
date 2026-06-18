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
	networkMu  sync.Mutex
	networkVal string
)

// CurrentNetwork returns "mainnet" or "testnet", lazily resolved via
// dcrd's getblockchaininfo. Only a successful resolution is cached for
// the process lifetime, since the chain identity doesn't change at
// runtime; a failed lookup (e.g. dcrd not yet reachable at startup) is
// returned without caching so a later call retries.
func CurrentNetwork(ctx context.Context) (string, error) {
	networkMu.Lock()
	defer networkMu.Unlock()
	if networkVal != "" {
		return networkVal, nil
	}
	if rpc.DcrdClient == nil {
		return "", fmt.Errorf("dcrd client not initialized")
	}
	info, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("get blockchain info: %w", err)
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
	return networkVal, nil
}
