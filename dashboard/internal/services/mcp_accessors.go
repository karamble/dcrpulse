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
// each call carries its own snapshot.

func FetchNodeStatus() (*types.NodeStatus, error) {
	return fetchNodeStatus(context.Background(), &chainSnapshot{})
}

func FetchBlockchainInfo() (*types.BlockchainInfo, error) {
	return fetchBlockchainInfo(context.Background(), &chainSnapshot{})
}

func FetchNetworkInfo() (*types.NetworkInfo, error) {
	return fetchNetworkInfo(context.Background(), &chainSnapshot{})
}

func FetchPeers() ([]types.Peer, error) {
	return fetchPeers(context.Background(), &chainSnapshot{})
}

func FetchSupplyInfo() (*types.SupplyInfo, error) {
	return fetchSupplyInfo(context.Background(), &chainSnapshot{})
}

func FetchStakingInfo() (*types.StakingInfo, error) {
	return fetchStakingInfo(context.Background(), &chainSnapshot{})
}

func FetchMempoolInfo() (*types.MempoolInfo, error) {
	return fetchMempoolInfo(context.Background(), &chainSnapshot{})
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
