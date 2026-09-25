// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "time"

// TreasuryInfo represents the complete treasury status
type TreasuryInfo struct {
	Balance       float64         `json:"balance"`       // Current treasury balance in DCR
	BalanceAtoms  int64           `json:"balanceAtoms"`  // The same balance in atoms
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
	YesVotes        int64     `json:"yesVotes"`        // Yes votes so far (from gettreasuryspendvotes)
	NoVotes         int64     `json:"noVotes"`         // No votes so far
	DetectedAt      time.Time `json:"detectedAt"`
}

// BalanceSample is one point in the treasury balance-over-time series.
type BalanceSample struct {
	Height  int64   `json:"height"`
	Time    int64   `json:"time"`    // block unix time
	Balance float64 `json:"balance"` // treasury balance in DCR at that block
}

// TSpendHistory represents a historical approved treasury spend
type TSpendHistory struct {
	TxHash      string    `json:"txHash"`
	Amount      float64   `json:"amount"`
	AmountAtoms int64     `json:"amountAtoms"` // Sum of the paid outputs
	FeeAtoms    int64     `json:"feeAtoms"`    // Fee the treasury paid for it
	Payee       string    `json:"payee"`       // Recipient address
	BlockHeight int64     `json:"blockHeight"` // Block where it was mined
	BlockHash   string    `json:"blockHash"`
	Timestamp   time.Time `json:"timestamp"`
	VoteResult  string    `json:"voteResult"` // "approved"
}

// TreasuryTAdd is a voluntary contribution to the treasury.
type TreasuryTAdd struct {
	TxHash      string    `json:"txHash"`
	AmountAtoms int64     `json:"amountAtoms"`
	BlockHeight int64     `json:"blockHeight"`
	BlockHash   string    `json:"blockHash"`
	Timestamp   time.Time `json:"timestamp"`
}

// TreasuryScanResults is everything the treasury received and paid in the
// blocks FromHeight through ToHeight.
type TreasuryScanResults struct {
	FromHeight   int64            `json:"fromHeight"`
	ToHeight     int64            `json:"toHeight"`
	TSpends      []TSpendHistory  `json:"tspends"`
	TAdds        []TreasuryTAdd   `json:"tadds"`
	TBaseByMonth map[string]int64 `json:"tbaseByMonth"` // UTC "2006-01" -> block-reward atoms
}

// TSpendScanProgress tracks the progress of historical TSpend scanning
type TSpendScanProgress struct {
	IsScanning    bool            `json:"isScanning"`
	CurrentHeight int64           `json:"currentHeight"`
	TotalHeight   int64           `json:"totalHeight"`
	Progress      float64         `json:"progress"`    // 0-100%
	TSpendFound   int             `json:"tspendFound"` // Count of TSpends found so far
	TAddFound     int             `json:"taddFound"`   // Count of contributions found so far
	NewTSpends    []TSpendHistory `json:"newTSpends"`  // TSpends found since last progress check
	Message       string          `json:"message"`
	FailedBlocks  int             `json:"failedBlocks,omitempty"` // Blocks the scan could not read
	SafeHeight    int64           `json:"safeHeight,omitempty"`   // Height a later scan may resume above
}

// TreasurySpendLimit is what dcrd's DCP-0013 expenditure rule lets the treasury
// spend in the block after Height, were that block a TVI.
type TreasurySpendLimit struct {
	Active             bool  `json:"active"`  // DCP-0013 is in force
	Height             int64 `json:"height"`  // Block the limit is computed after
	NextTVI            int64 `json:"nextTvi"` // Next block that may carry treasury spends
	AtTVI              bool  `json:"atTvi"`   // Height+1 is that TVI block
	PolicyWindowBlocks int64 `json:"policyWindowBlocks"`
	SpentInWindowAtoms int64 `json:"spentInWindowAtoms"`
	BalanceAtoms       int64 `json:"balanceAtoms"` // Balance as of the block after Height
	MaxSpendableAtoms  int64 `json:"maxSpendableAtoms"`
	FloorAtoms         int64 `json:"floorAtoms"`
	AllowedAtoms       int64 `json:"allowedAtoms"`
}

// TreasuryOutlook is the treasury's projected block reward per calendar month
// after FromHeight, with blocks at the target block time.
type TreasuryOutlook struct {
	FromHeight         int64                  `json:"fromHeight"`
	TargetBlockSeconds int64                  `json:"targetBlockSeconds"`
	Months             []TreasuryOutlookMonth `json:"months"`
}

// TreasuryOutlookMonth is one projected month.
type TreasuryOutlookMonth struct {
	Month      string `json:"month"` // UTC "2006-01"
	Blocks     int64  `json:"blocks"`
	TBaseAtoms int64  `json:"tbaseAtoms"`
}

// TreasuryRunway is how many whole calendar months after FromHeight the
// balance lasts at MonthlySpendAtoms, each month adding the block reward
// projected at the target block time. Beyond is set when it outlasts
// ProjectionMonths.
type TreasuryRunway struct {
	FromHeight         int64  `json:"fromHeight"`
	BalanceAtoms       int64  `json:"balanceAtoms"`
	MonthlySpendAtoms  int64  `json:"monthlySpendAtoms"`
	FirstMonthNetAtoms int64  `json:"firstMonthNetAtoms"` // Block reward less spend, next month
	TargetBlockSeconds int64  `json:"targetBlockSeconds"`
	ProjectionMonths   int    `json:"projectionMonths"`
	Months             int    `json:"months"`
	ExhaustedMonth     string `json:"exhaustedMonth,omitempty"` // UTC "2006-01" the balance runs out in
	Beyond             bool   `json:"beyond"`
}
