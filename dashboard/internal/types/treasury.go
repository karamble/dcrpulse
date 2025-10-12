// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "time"

// TreasuryInfo represents the complete treasury status
type TreasuryInfo struct {
	Balance       float64         `json:"balance"`       // Current treasury balance in DCR
	BalanceUSD    float64         `json:"balanceUsd"`    // USD equivalent (if available)
	TotalAdded    float64         `json:"totalAdded"`    // Lifetime treasury additions
	TotalSpent    float64         `json:"totalSpent"`    // Lifetime treasury expenditures
	ActiveTSpends []TSpend        `json:"activeTSpends"` // TSpends currently in mempool
	RecentTSpends []TSpendHistory `json:"recentTSpends"` // Recently approved TSpends
	LastUpdate    time.Time       `json:"lastUpdate"`
}

// TSpend represents an active treasury spend transaction in mempool
type TSpend struct {
	TxHash          string    `json:"txHash"`
	Amount          float64   `json:"amount"`
	Payee           string    `json:"payee"`           // Recipient address
	ExpiryHeight    int64     `json:"expiryHeight"`    // Block height when voting expires
	CurrentHeight   int64     `json:"currentHeight"`   // Current blockchain height
	BlocksRemaining int64     `json:"blocksRemaining"` // Blocks until expiry
	Status          string    `json:"status"`          // "voting", "approved", "rejected"
	DetectedAt      time.Time `json:"detectedAt"`
}

// TSpendHistory represents a historical approved treasury spend
type TSpendHistory struct {
	TxHash      string    `json:"txHash"`
	Amount      float64   `json:"amount"`
	Payee       string    `json:"payee"`       // Recipient address
	BlockHeight int64     `json:"blockHeight"` // Block where it was mined
	BlockHash   string    `json:"blockHash"`
	Timestamp   time.Time `json:"timestamp"`
	VoteResult  string    `json:"voteResult"` // "approved"
}

// TSpendScanProgress tracks the progress of historical TSpend scanning
type TSpendScanProgress struct {
	IsScanning    bool            `json:"isScanning"`
	CurrentHeight int64           `json:"currentHeight"`
	TotalHeight   int64           `json:"totalHeight"`
	Progress      float64         `json:"progress"`    // 0-100%
	TSpendFound   int             `json:"tspendFound"` // Count of TSpends found so far
	NewTSpends    []TSpendHistory `json:"newTSpends"`  // TSpends found since last progress check
	Message       string          `json:"message"`
}
