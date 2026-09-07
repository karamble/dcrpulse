// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
)

// Accessors for the agent MCP tools. The dashboard reads these values through
// FetchDashboardData's shared snapshot; the tools read them one at a time, so
// each call carries its own snapshot and the caller's context, so an agent that
// hangs up does not leave the RPCs running.

func FetchNodeStatus(ctx context.Context) (*types.NodeStatus, error) {
	return fetchNodeStatus(ctx, &chainSnapshot{})
}

func FetchBlockchainInfo(ctx context.Context) (*types.BlockchainInfo, error) {
	return fetchBlockchainInfo(ctx, &chainSnapshot{})
}

func FetchNetworkInfo(ctx context.Context) (*types.NetworkInfo, error) {
	return fetchNetworkInfo(ctx, &chainSnapshot{})
}

func FetchPeers(ctx context.Context) ([]types.Peer, error) {
	return fetchPeers(ctx, &chainSnapshot{})
}

func FetchSupplyInfo(ctx context.Context) (*types.SupplyInfo, error) {
	return fetchSupplyInfo(ctx, &chainSnapshot{})
}

func FetchStakingInfo(ctx context.Context) (*types.StakingInfo, error) {
	return fetchStakingInfo(ctx, &chainSnapshot{})
}

func FetchMempoolInfo(ctx context.Context) (*types.MempoolInfo, error) {
	return fetchMempoolInfo(ctx, &chainSnapshot{})
}

// AgentAddress is one funded receiving address in wallet_addresses output.
type AgentAddress struct {
	Address string `json:"address"`
	Account string `json:"account"`
	Used    bool   `json:"used"`
}

// FetchAddressesWithContext lists receiving addresses that have received
// funds, capped at 100 so a large wallet cannot blow up the reply.
func FetchAddressesWithContext(ctx context.Context) ([]AgentAddress, error) {
	result, err := rpc.WalletClient.RawRequest(ctx, "listreceivedbyaddress", []json.RawMessage{
		json.RawMessage(`0`),     // minconf
		json.RawMessage(`false`), // exclude never-funded addresses
	})
	if err != nil {
		return nil, err
	}

	var raw []struct {
		Address string  `json:"address"`
		Account string  `json:"account"`
		Amount  float64 `json:"amount"`
	}
	if err := json.Unmarshal(result, &raw); err != nil {
		return nil, err
	}
	if len(raw) > 100 {
		raw = raw[:100]
	}

	addresses := make([]AgentAddress, 0, len(raw))
	for _, a := range raw {
		addresses = append(addresses, AgentAddress{
			Address: a.Address,
			Account: a.Account,
			Used:    a.Amount > 0,
		})
	}
	return addresses, nil
}
