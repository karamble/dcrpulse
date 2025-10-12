// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
)

// Constants for treasury
const (
	TreasuryActivationHeight = 552448 // Block where treasury was first activated (May 2021)
)

// Global scan state
var (
	scanMutex         sync.RWMutex
	isScanRunning     bool
	currentScanHeight int64
	totalScanHeight   int64
	tspendFoundCount  int
	scanResults       []types.TSpendHistory
	newTSpendBuffer   []types.TSpendHistory // Buffer for TSpends found since last progress check
)

// FetchTreasuryInfo gets current treasury status including balance and active TSpends
// Note: Historical TSpends are tracked in frontend localStorage, not fetched here
func FetchTreasuryInfo(ctx context.Context) (*types.TreasuryInfo, error) {
	// Get current treasury balance
	balance, err := getTreasuryBalance(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get treasury balance: %v", err)
		balance = 0
	}

	// Scan mempool for active TSpends (pending votes)
	activeTSpends, err := scanMempoolForTSpends(ctx)
	if err != nil {
		log.Printf("Warning: Failed to scan mempool for TSpends: %v", err)
		activeTSpends = []types.TSpend{}
	}
	// Ensure activeTSpends is never nil
	if activeTSpends == nil {
		activeTSpends = []types.TSpend{}
	}

	return &types.TreasuryInfo{
		Balance:       balance,
		BalanceUSD:    0, // TODO: Add USD conversion if needed
		TotalAdded:    0, // Tracked in frontend localStorage
		TotalSpent:    0, // Tracked in frontend localStorage
		ActiveTSpends: activeTSpends,
		RecentTSpends: []types.TSpendHistory{}, // Not used - data comes from localStorage
		LastUpdate:    time.Now(),
	}, nil
}

// getTreasuryBalance retrieves current treasury balance from dcrd
func getTreasuryBalance(ctx context.Context) (float64, error) {
	if rpc.DcrdClient == nil {
		return 0, fmt.Errorf("dcrd client not available")
	}

	treasuryBalance, err := rpc.DcrdClient.GetTreasuryBalance(ctx, nil, false)
	if err != nil {
		return 0, fmt.Errorf("failed to get treasury balance: %w", err)
	}

	// Convert atoms to DCR
	balanceDCR := float64(treasuryBalance.Balance) / 1e8
	return balanceDCR, nil
}

// scanMempoolForTSpends scans the mempool for active treasury spend transactions
func scanMempoolForTSpends(ctx context.Context) ([]types.TSpend, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}

	// Get raw mempool with verbose=true
	result, err := rpc.DcrdClient.RawRequest(ctx, "getrawmempool", []json.RawMessage{
		json.RawMessage("true"), // verbose
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get mempool: %w", err)
	}

	// Parse mempool response
	var mempoolMap map[string]interface{}
	if err := json.Unmarshal(result, &mempoolMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mempool: %w", err)
	}

	var tspends []types.TSpend
	currentHeight, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get current height: %v", err)
		currentHeight = 0
	}

	// Check each transaction
	for txHash := range mempoolMap {
		// Get transaction details
		tx, err := getTransaction(ctx, txHash)
		if err != nil {
			log.Printf("Warning: Failed to get transaction %s: %v", txHash, err)
			continue
		}

		// Check if it's a treasury spend
		if isTreasurySpend(tx) {
			tspend := extractTSpendInfo(tx, currentHeight)
			if tspend != nil {
				tspends = append(tspends, *tspend)
			}
		}
	}

	return tspends, nil
}

// getTransaction retrieves transaction details
func getTransaction(ctx context.Context, txHash string) (map[string]interface{}, error) {
	result, err := rpc.DcrdClient.RawRequest(ctx, "getrawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, txHash)),
		json.RawMessage("1"), // verbose
	})
	if err != nil {
		return nil, err
	}

	var tx map[string]interface{}
	if err := json.Unmarshal(result, &tx); err != nil {
		return nil, err
	}

	return tx, nil
}

// isTreasurySpend checks if a transaction is a treasury spend (not treasurybase)
func isTreasurySpend(tx map[string]interface{}) bool {
	// Method 1: Check for "treasuryspend" field in vin (MOST RELIABLE)
	// Real TSpend transactions have this special field instead of txid/vout
	vin, ok := tx["vin"].([]interface{})
	if ok && len(vin) > 0 {
		for _, v := range vin {
			vinMap, ok := v.(map[string]interface{})
			if !ok {
				continue
			}

			// Check if this input has a "treasuryspend" field
			if _, hasTreasurySpend := vinMap["treasuryspend"]; hasTreasurySpend {
				return true
			}
		}
	}

	// Method 2: Check for treasurygen output type (SECONDARY CHECK)
	// TSpend transactions have "treasurygen-pubkeyhash" or similar in output
	vout, ok := tx["vout"].([]interface{})
	if ok && len(vout) > 0 {
		for _, v := range vout {
			voutMap, ok := v.(map[string]interface{})
			if !ok {
				continue
			}

			scriptPubKey, ok := voutMap["scriptPubKey"].(map[string]interface{})
			if !ok {
				continue
			}

			scriptType, ok := scriptPubKey["type"].(string)
			if !ok {
				continue
			}

			// TSpend transactions have "treasurygen" in the output type
			// Can be "treasurygen-pubkeyhash", "treasurygen-scripthash", etc.
			if strings.Contains(strings.ToLower(scriptType), "treasurygen") {
				// Additional validation: must be version 3
				version, _ := tx["version"].(float64)
				if version == 3 {
					return true
				}
			}
		}
	}

	return false
}

// extractTSpendInfo extracts TSpend information from a transaction
func extractTSpendInfo(tx map[string]interface{}, currentHeight int64) *types.TSpend {
	txid, _ := tx["txid"].(string)
	expiry, _ := tx["expiry"].(float64)

	// Calculate amount from outputs
	amount := 0.0
	payee := ""
	vout, _ := tx["vout"].([]interface{})
	for _, v := range vout {
		voutMap, _ := v.(map[string]interface{})
		value, _ := voutMap["value"].(float64)
		amount += value

		// Try to get payee address
		if scriptPubKey, ok := voutMap["scriptPubKey"].(map[string]interface{}); ok {
			if addresses, ok := scriptPubKey["addresses"].([]interface{}); ok && len(addresses) > 0 {
				if addr, ok := addresses[0].(string); ok {
					payee = addr
				}
			}
		}
	}

	expiryHeight := int64(expiry)
	blocksRemaining := expiryHeight - currentHeight

	return &types.TSpend{
		TxHash:          txid,
		Amount:          amount,
		Payee:           payee,
		ExpiryHeight:    expiryHeight,
		CurrentHeight:   currentHeight,
		BlocksRemaining: blocksRemaining,
		Status:          "voting",
		DetectedAt:      time.Now(),
	}
}

// extractTSpendHistory extracts historical TSpend information
func extractTSpendHistory(tx map[string]interface{}, blockHeight int64, blockHash string, blockTime int64) *types.TSpendHistory {
	txid, _ := tx["txid"].(string)

	// Calculate amount from outputs
	amount := 0.0
	payee := ""
	vout, _ := tx["vout"].([]interface{})
	for _, v := range vout {
		voutMap, _ := v.(map[string]interface{})
		value, _ := voutMap["value"].(float64)
		amount += value

		// Try to get payee address
		if scriptPubKey, ok := voutMap["scriptPubKey"].(map[string]interface{}); ok {
			if addresses, ok := scriptPubKey["addresses"].([]interface{}); ok && len(addresses) > 0 {
				if addr, ok := addresses[0].(string); ok {
					payee = addr
				}
			}
		}
	}

	return &types.TSpendHistory{
		TxHash:      txid,
		Amount:      amount,
		Payee:       payee,
		BlockHeight: blockHeight,
		BlockHash:   blockHash,
		Timestamp:   time.Unix(blockTime, 0),
		VoteResult:  "approved",
	}
}

// TriggerHistoricalScan starts a background scan of the blockchain for all TSpends
func TriggerHistoricalScan(startHeight int64) error {
	scanMutex.Lock()
	if isScanRunning {
		scanMutex.Unlock()
		return fmt.Errorf("scan already in progress")
	}
	isScanRunning = true

	// Validate startHeight
	if startHeight < TreasuryActivationHeight {
		startHeight = TreasuryActivationHeight
	}

	currentScanHeight = startHeight
	tspendFoundCount = 0
	scanResults = []types.TSpendHistory{}
	newTSpendBuffer = []types.TSpendHistory{}
	scanMutex.Unlock()

	go scanHistoricalTSpendsBackground(startHeight)
	return nil
}

// scanHistoricalTSpendsBackground performs the historical scan in the background
func scanHistoricalTSpendsBackground(startHeight int64) {
	ctx := context.Background()

	currentHeight, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		log.Printf("Error getting block count for scan: %v", err)
		scanMutex.Lock()
		isScanRunning = false
		scanMutex.Unlock()
		return
	}

	scanMutex.Lock()
	totalScanHeight = currentHeight
	scanMutex.Unlock()

	log.Printf("Starting historical TSpend scan from block %d to %d", startHeight, currentHeight)

	for h := startHeight; h <= currentHeight; h++ {
		// Update progress
		scanMutex.Lock()
		currentScanHeight = h
		scanMutex.Unlock()

		blockHash, err := rpc.DcrdClient.GetBlockHash(ctx, h)
		if err != nil {
			log.Printf("Warning: Failed to get block hash at height %d: %v", h, err)
			continue
		}

		blockResult, err := rpc.DcrdClient.RawRequest(ctx, "getblock", []json.RawMessage{
			json.RawMessage(fmt.Sprintf(`"%s"`, blockHash.String())),
			json.RawMessage("true"),
			json.RawMessage("false"),
		})
		if err != nil {
			continue
		}

		var block struct {
			Hash   string   `json:"hash"`
			Height int64    `json:"height"`
			Time   int64    `json:"time"`
			Tx     []string `json:"tx"`
			STx    []string `json:"stx"`
		}

		if err := json.Unmarshal(blockResult, &block); err != nil {
			continue
		}

		allTxs := append(block.Tx, block.STx...)
		for _, txHash := range allTxs {
			tx, err := getTransaction(ctx, txHash)
			if err != nil {
				continue
			}

			if isTreasurySpend(tx) {
				history := extractTSpendHistory(tx, block.Height, block.Hash, block.Time)
				if history != nil {
					scanMutex.Lock()
					scanResults = append(scanResults, *history)
					newTSpendBuffer = append(newTSpendBuffer, *history)
					tspendFoundCount++
					log.Printf("TSpend found at height %d: %s (amount: %.2f DCR)", block.Height, history.TxHash, history.Amount)
					scanMutex.Unlock()
				}
			}
		}
	}

	scanMutex.Lock()
	isScanRunning = false
	scanMutex.Unlock()

	log.Printf("Historical TSpend scan complete. Found %d TSpends", tspendFoundCount)
}

// GetScanProgress returns the current scan progress
func GetScanProgress() (*types.TSpendScanProgress, error) {
	scanMutex.Lock()
	defer scanMutex.Unlock()

	progress := 0.0
	if totalScanHeight > TreasuryActivationHeight {
		progress = float64(currentScanHeight-TreasuryActivationHeight) / float64(totalScanHeight-TreasuryActivationHeight) * 100
	}

	message := "Scanning blockchain for treasury spends..."
	if !isScanRunning {
		if tspendFoundCount > 0 {
			message = fmt.Sprintf("Scan complete. Found %d treasury spends", tspendFoundCount)
		} else {
			message = "No scan in progress"
		}
	}

	// Get new TSpends and clear the buffer
	newTSpends := make([]types.TSpendHistory, len(newTSpendBuffer))
	copy(newTSpends, newTSpendBuffer)
	newTSpendBuffer = []types.TSpendHistory{} // Clear buffer after copying

	return &types.TSpendScanProgress{
		IsScanning:    isScanRunning,
		CurrentHeight: currentScanHeight,
		TotalHeight:   totalScanHeight,
		Progress:      progress,
		TSpendFound:   tspendFoundCount,
		NewTSpends:    newTSpends,
		Message:       message,
	}, nil
}

// GetScanResults returns the results from the last completed scan
func GetScanResults() []types.TSpendHistory {
	scanMutex.RLock()
	defer scanMutex.RUnlock()

	// Return a copy
	results := make([]types.TSpendHistory, len(scanResults))
	copy(results, scanResults)
	return results
}
