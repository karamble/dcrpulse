// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "time"

// WalletDashboardData represents all wallet dashboard metrics
type WalletDashboardData struct {
	WalletStatus WalletStatus       `json:"walletStatus"`
	AccountInfo  AccountInfo        `json:"accountInfo"`
	Accounts     []AccountInfo      `json:"accounts"`
	StakingInfo  *WalletStakingInfo `json:"stakingInfo,omitempty"`
	LastUpdate   time.Time          `json:"lastUpdate"`
}

type WalletStatus struct {
	Status           string  `json:"status"` // "locked", "unlocked", "syncing", "synced", "no_wallet"
	SyncProgress     float64 `json:"syncProgress"`
	SyncHeight       int64   `json:"syncHeight"`
	BestBlockHash    string  `json:"bestBlockHash"`
	Version          string  `json:"version"`
	Unlocked         bool    `json:"unlocked"`
	RescanInProgress bool    `json:"rescanInProgress"`
	SyncMessage      string  `json:"syncMessage"`
}

type AccountInfo struct {
	AccountName             string  `json:"accountName"`
	TotalBalance            float64 `json:"totalBalance"`
	SpendableBalance        float64 `json:"spendableBalance"`
	ImmatureBalance         float64 `json:"immatureBalance"`
	UnconfirmedBalance      float64 `json:"unconfirmedBalance"`
	LockedByTickets         float64 `json:"lockedByTickets"`
	VotingAuthority         float64 `json:"votingAuthority"`
	ImmatureCoinbaseRewards float64 `json:"immatureCoinbaseRewards"`
	ImmatureStakeGeneration float64 `json:"immatureStakeGeneration"`
	AccountNumber           uint32  `json:"accountNumber"`
	// Wallet-wide totals (only populated in primary AccountInfo)
	CumulativeTotal      float64 `json:"cumulativeTotal,omitempty"`
	TotalSpendable       float64 `json:"totalSpendable,omitempty"`
	TotalLockedByTickets float64 `json:"totalLockedByTickets,omitempty"`
}

type Transaction struct {
	TxID          string    `json:"txid"`
	Amount        float64   `json:"amount"`
	Fee           float64   `json:"fee,omitempty"`
	Confirmations int64     `json:"confirmations"`
	BlockHash     string    `json:"blockHash,omitempty"`
	BlockTime     int64     `json:"blockTime,omitempty"`
	Time          time.Time `json:"time"`
	Category      string    `json:"category"` // "send", "receive", "immature", "generate"
	TxType        string    `json:"txType"`   // "regular", "ticket", "vote", "revocation"
	Address       string    `json:"address,omitempty"`
	Account       string    `json:"account,omitempty"`
	Vout          uint32    `json:"vout"`
	Generated     bool      `json:"generated,omitempty"`
	IsMixed       bool      `json:"isMixed,omitempty"` // true if from CoinJoin/StakeShuffle
}

type TransactionListResponse struct {
	Transactions []Transaction `json:"transactions"`
	Total        int           `json:"total"`
}

type Address struct {
	Address string `json:"address"`
	Account string `json:"account"`
	Used    bool   `json:"used"`
	Path    string `json:"path"` // BIP44 path
}

type ImportXpubRequest struct {
	Xpub        string `json:"xpub"`
	AccountName string `json:"accountName"`
	Rescan      bool   `json:"rescan"`
}

type ImportXpubResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	AccountNum uint32 `json:"accountNum,omitempty"`
}

type RescanRequest struct {
	BeginHeight int32 `json:"beginHeight"`
}

type RescanResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type SyncProgressResponse struct {
	IsRescanning bool    `json:"isRescanning"`
	ScanHeight   int64   `json:"scanHeight"`
	ChainHeight  int64   `json:"chainHeight"`
	Progress     float64 `json:"progress"`
	Message      string  `json:"message"`
}

// WalletStakingInfo represents wallet staking information
type WalletStakingInfo struct {
	// From getstakeinfo
	BlockHeight    int64   `json:"blockHeight"`
	Difficulty     float64 `json:"difficulty"`
	TotalSubsidy   float64 `json:"totalSubsidy"`
	OwnMempoolTix  int32   `json:"ownMempoolTix"`
	Immature       int32   `json:"immature"`
	Unspent        int32   `json:"unspent"`
	Voted          int32   `json:"voted"`
	Revoked        int32   `json:"revoked"`
	UnspentExpired int32   `json:"unspentExpired"`
	PoolSize       int32   `json:"poolSize"`
	AllMempoolTix  int32   `json:"allMempoolTix"`
	// From estimatestakediff
	EstimatedMin      float64 `json:"estimatedMin"`
	EstimatedMax      float64 `json:"estimatedMax"`
	EstimatedExpected float64 `json:"estimatedExpected"`
	// From getstakedifficulty
	CurrentDifficulty float64 `json:"currentDifficulty"`
	NextDifficulty    float64 `json:"nextDifficulty"`
}
