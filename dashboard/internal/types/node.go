// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "time"

// DashboardData represents all dashboard metrics
type DashboardData struct {
	NodeStatus     NodeStatus     `json:"nodeStatus"`
	BlockchainInfo BlockchainInfo `json:"blockchainInfo"`
	NetworkInfo    NetworkInfo    `json:"networkInfo"`
	Peers          []Peer         `json:"peers"`
	SupplyInfo     SupplyInfo     `json:"supplyInfo"`
	StakingInfo    StakingInfo    `json:"stakingInfo"`
	MempoolInfo    MempoolInfo    `json:"mempoolInfo"`
	LastUpdate     time.Time      `json:"lastUpdate"`
}

type NodeStatus struct {
	Status       string  `json:"status"`
	SyncProgress float64 `json:"syncProgress"`
	Version      string  `json:"version"`
	SyncPhase    string  `json:"syncPhase"`   // "headers" or "blocks"
	SyncMessage  string  `json:"syncMessage"` // e.g., "Processed 36,000 headers in the last 30 seconds"
}

type BlockchainInfo struct {
	BlockHeight  int64         `json:"blockHeight"`
	BlockHash    string        `json:"blockHash"`
	Difficulty   float64       `json:"difficulty"`
	ChainSize    int64         `json:"chainSize"`
	BlockTime    string        `json:"blockTime"`
	RecentBlocks []RecentBlock `json:"recentBlocks"`
}

type RecentBlock struct {
	Height    int64  `json:"height"`
	Hash      string `json:"hash"`
	Timestamp int64  `json:"timestamp"` // Unix timestamp
}

type NetworkInfo struct {
	PeerCount     int     `json:"peerCount"`
	Hashrate      string  `json:"hashrate"`
	NetworkHashPS float64 `json:"networkHashPS"`
}

type Peer struct {
	ID         int    `json:"id"`
	Address    string `json:"address"`
	Protocol   string `json:"protocol"`
	Latency    string `json:"latency"`
	ConnTime   string `json:"connTime"`
	Traffic    string `json:"traffic"`
	Version    string `json:"version"`
	IsSyncNode bool   `json:"isSyncNode"`
}

type SupplyInfo struct {
	CirculatingSupply string  `json:"circulatingSupply"`
	StakedSupply      string  `json:"stakedSupply"`
	StakedPercent     float64 `json:"stakedPercent"`
	ExchangeRate      string  `json:"exchangeRate"`
	TreasurySize      string  `json:"treasurySize"`
	MixedPercent      string  `json:"mixedPercent"`
}

type StakingInfo struct {
	TicketPrice       float64 `json:"ticketPrice"`
	NextTicketPrice   float64 `json:"nextTicketPrice"`
	PoolSize          uint32  `json:"poolSize"`
	LockedDCR         float64 `json:"lockedDCR"`
	ParticipationRate float64 `json:"participationRate"`
	AllMempoolTix     uint32  `json:"allMempoolTix"`
	Immature          uint32  `json:"immature"`
	Live              uint32  `json:"live"`
	Voted             uint32  `json:"voted"`
	Missed            uint32  `json:"missed"`
	Revoked           uint32  `json:"revoked"`
}

type MempoolInfo struct {
	Size           uint64  `json:"size"`
	Bytes          uint64  `json:"bytes"`
	TxCount        int     `json:"txCount"`
	TotalFee       float64 `json:"totalFee"`
	AverageFeeRate float64 `json:"averageFeeRate"`
	Tickets        int     `json:"tickets"`     // Ticket purchases in mempool
	Votes          int     `json:"votes"`       // Votes in mempool
	Revocations    int     `json:"revocations"` // Revocations in mempool
	RegularTxs     int     `json:"regularTxs"`  // Regular transactions (non-CoinJoin)
	CoinJoinTxs    int     `json:"coinJoinTxs"` // CoinJoin/StakeShuffle transactions
}

// RPC connection types
type RPCConnectionRequest struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type RPCConnectionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
