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

// GetMempoolTSpends retrieves active tspends from mempool with voting info
func GetMempoolTSpends(ctx context.Context) ([]types.TSpend, error) {
	return scanMempoolForTSpends(ctx)
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

// Vote counting and caching
var (
	votingCache         = make(map[string]*types.TSpendVotingInfo)
	votingCacheMutex    sync.RWMutex
	voteParsingProgress = make(map[string]*types.VoteParsingProgress)
	progressMutex       sync.RWMutex
	parsingJobs         = make(map[string]bool) // Track active parsing jobs
	jobsMutex           sync.RWMutex
)

// GetTSpendVotingInfo retrieves or calculates voting information for a tspend transaction
func GetTSpendVotingInfo(ctx context.Context, txHash string, blockHeight int64, expiry uint32, inMempool bool) (*types.TSpendVotingInfo, error) {
	// Check cache first (only for confirmed tspends)
	if !inMempool {
		votingCacheMutex.RLock()
		if cached, ok := votingCache[txHash]; ok {
			votingCacheMutex.RUnlock()
			return cached, nil
		}
		votingCacheMutex.RUnlock()
	}

	// Check if parsing is already in progress
	jobsMutex.RLock()
	isJobRunning := parsingJobs[txHash]
	jobsMutex.RUnlock()

	if isJobRunning {
		// Return partial results if available
		progressMutex.RLock()
		progress, hasProgress := voteParsingProgress[txHash]
		progressMutex.RUnlock()

		if hasProgress {
			// Return voting info with current progress
			return &types.TSpendVotingInfo{
				VotingStartBlock: progress.CurrentBlock - int64(float64(progress.CurrentBlock-blockHeight+2880)*(progress.Progress/100.0)),
				VotingEndBlock:   blockHeight,
				YesVotes:         progress.YesVotes,
				NoVotes:          progress.NoVotes,
				VotesCast:        progress.YesVotes + progress.NoVotes,
				VotingComplete:   !progress.IsParsing,
				InMempool:        inMempool,
			}, nil
		}
	}

	// Start async vote counting for confirmed tspends
	if !inMempool && !isJobRunning {
		jobsMutex.Lock()
		parsingJobs[txHash] = true
		jobsMutex.Unlock()

		go calculateTSpendVotesAsync(context.Background(), txHash, blockHeight, expiry, inMempool)

		// Return initial empty state - frontend will poll for progress
		return &types.TSpendVotingInfo{
			VotingStartBlock: blockHeight - 2880,
			VotingEndBlock:   blockHeight,
			VotingComplete:   false,
			InMempool:        inMempool,
		}, nil
	}

	// For mempool or if immediate calculation is needed
	votingInfo, err := calculateTSpendVotes(ctx, txHash, blockHeight, expiry, inMempool)
	if err != nil {
		return nil, err
	}

	return votingInfo, nil
}

// GetVoteParsingProgress retrieves current progress for a tspend vote counting job
func GetVoteParsingProgress(txHash string) (*types.VoteParsingProgress, bool) {
	progressMutex.RLock()
	defer progressMutex.RUnlock()
	progress, ok := voteParsingProgress[txHash]
	return progress, ok
}

// calculateTSpendVotes counts votes for a tspend in the voting period
func calculateTSpendVotes(ctx context.Context, txHash string, blockHeight int64, expiry uint32, inMempool bool) (*types.TSpendVotingInfo, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}

	currentHeight, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current height: %w", err)
	}

	// Determine voting period
	var votingStartBlock, votingEndBlock int64
	var votingComplete bool

	if inMempool {
		// For mempool tspends, estimate start block (could be when first seen, but we'll use a lookback)
		// Typical voting period is ~2880 blocks (TVI - Treasury Vote Interval)
		votingStartBlock = currentHeight - 2880
		if votingStartBlock < 0 {
			votingStartBlock = 0
		}
		votingEndBlock = int64(expiry)
		votingComplete = false
	} else {
		// For confirmed tspends, voting ended when mined
		// Start block is approximately voting interval before
		votingStartBlock = blockHeight - 2880
		if votingStartBlock < TreasuryActivationHeight {
			votingStartBlock = TreasuryActivationHeight
		}
		votingEndBlock = blockHeight
		votingComplete = true
	}

	// Count votes in the range
	yesVotes, noVotes, err := countTSpendVotesInRange(ctx, txHash, votingStartBlock, votingEndBlock)
	if err != nil {
		log.Printf("Warning: Failed to count votes: %v", err)
		// Return partial data even if vote counting fails
		yesVotes, noVotes = 0, 0
	}

	votesCast := yesVotes + noVotes

	// Calculate statistics
	var approvalRate, turnoutRate float64
	if votesCast > 0 {
		approvalRate = float64(yesVotes) / float64(votesCast) * 100
	}

	// Estimate eligible votes based on ticket pool (simplified)
	// In reality, this would need to check ticket pool size during voting period
	eligibleVotes := int(float64(votingEndBlock-votingStartBlock) * 5) // Approximate: 5 votes per block
	if eligibleVotes > 0 {
		turnoutRate = float64(votesCast) / float64(eligibleVotes) * 100
	}

	// Quorum requirement (simplified - typically 20% of ticket pool)
	quorumRequired := eligibleVotes / 5
	quorumAchieved := votesCast >= quorumRequired

	// Get timestamps
	startTime, endTime := getBlockTimestamps(ctx, votingStartBlock, votingEndBlock)

	return &types.TSpendVotingInfo{
		VotingStartBlock: votingStartBlock,
		VotingEndBlock:   votingEndBlock,
		YesVotes:         yesVotes,
		NoVotes:          noVotes,
		EligibleVotes:    eligibleVotes,
		VotesCast:        votesCast,
		QuorumRequired:   quorumRequired,
		ApprovalRate:     approvalRate,
		TurnoutRate:      turnoutRate,
		QuorumAchieved:   quorumAchieved,
		VotingComplete:   votingComplete,
		InMempool:        inMempool,
		VotingStartTime:  startTime,
		VotingEndTime:    endTime,
	}, nil
}

// calculateTSpendVotesAsync calculates votes asynchronously with progress tracking
func calculateTSpendVotesAsync(ctx context.Context, txHash string, blockHeight int64, expiry uint32, inMempool bool) {
	defer func() {
		// Clean up job tracking
		jobsMutex.Lock()
		delete(parsingJobs, txHash)
		jobsMutex.Unlock()
	}()

	if rpc.DcrdClient == nil {
		return
	}

	// Determine voting period
	var votingStartBlock, votingEndBlock int64
	votingStartBlock = blockHeight - 2880
	if votingStartBlock < TreasuryActivationHeight {
		votingStartBlock = TreasuryActivationHeight
	}
	votingEndBlock = blockHeight

	totalBlocks := votingEndBlock - votingStartBlock + 1 // +1 because we scan inclusively

	// Initialize progress
	progressMutex.Lock()
	voteParsingProgress[txHash] = &types.VoteParsingProgress{
		IsParsing:     true,
		Progress:      0,
		CurrentBlock:  votingStartBlock,
		TotalBlocks:   totalBlocks,
		YesVotes:      0,
		NoVotes:       0,
		EstimatedTime: int(totalBlocks / 10), // Rough estimate: 10 blocks/sec
		Message:       "Starting vote count...",
	}
	progressMutex.Unlock()

	// Count votes with progress updates
	yesVotes, noVotes := 0, 0
	startTime := time.Now()

	// Limit scan range for performance
	maxScanRange := int64(3000)
	if votingEndBlock-votingStartBlock > maxScanRange {
		votingStartBlock = votingEndBlock - maxScanRange
	}

	for height := votingStartBlock; height <= votingEndBlock; height++ {
		blockHash, err := rpc.DcrdClient.GetBlockHash(ctx, height)
		if err != nil {
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
			STx []string `json:"stx"`
		}
		if err := json.Unmarshal(blockResult, &block); err != nil {
			continue
		}

		// Check each stake transaction for votes
		for _, stxHash := range block.STx {
			tx, err := getTransaction(ctx, stxHash)
			if err != nil {
				continue
			}

			if !isVoteTransaction(tx) {
				continue
			}

			vote := parseTSpendVote(tx, txHash)
			if vote == "yes" {
				yesVotes++
			} else if vote == "no" {
				noVotes++
			}
		}

		// Update progress every 50 blocks
		if height%50 == 0 || height == votingEndBlock {
			blocksProcessed := height - votingStartBlock + 1 // +1 because we count inclusively
			progress := float64(blocksProcessed) / float64(totalBlocks) * 100
			elapsed := time.Since(startTime).Seconds()
			blocksRemaining := votingEndBlock - height
			estimatedTime := 0
			if blocksProcessed > 0 {
				timePerBlock := elapsed / float64(blocksProcessed)
				estimatedTime = int(float64(blocksRemaining) * timePerBlock)
			}

			progressMutex.Lock()
			voteParsingProgress[txHash] = &types.VoteParsingProgress{
				IsParsing:     height < votingEndBlock,
				Progress:      progress,
				CurrentBlock:  height,
				TotalBlocks:   totalBlocks,
				YesVotes:      yesVotes,
				NoVotes:       noVotes,
				EstimatedTime: estimatedTime,
				Message:       fmt.Sprintf("Scanning block %d of %d...", height, votingEndBlock),
			}
			progressMutex.Unlock()
		}
	}

	// Calculate final statistics
	votesCast := yesVotes + noVotes
	var approvalRate, turnoutRate float64
	if votesCast > 0 {
		approvalRate = float64(yesVotes) / float64(votesCast) * 100
	}

	eligibleVotes := int(float64(totalBlocks) * 5)
	if eligibleVotes > 0 {
		turnoutRate = float64(votesCast) / float64(eligibleVotes) * 100
	}

	quorumRequired := eligibleVotes / 5
	quorumAchieved := votesCast >= quorumRequired

	startTm, endTime := getBlockTimestamps(ctx, votingStartBlock, votingEndBlock)

	// Create final result
	finalResult := &types.TSpendVotingInfo{
		VotingStartBlock: votingStartBlock,
		VotingEndBlock:   votingEndBlock,
		YesVotes:         yesVotes,
		NoVotes:          noVotes,
		EligibleVotes:    eligibleVotes,
		VotesCast:        votesCast,
		QuorumRequired:   quorumRequired,
		ApprovalRate:     approvalRate,
		TurnoutRate:      turnoutRate,
		QuorumAchieved:   quorumAchieved,
		VotingComplete:   true,
		InMempool:        false,
		VotingStartTime:  startTm,
		VotingEndTime:    endTime,
	}

	// Cache the result
	votingCacheMutex.Lock()
	votingCache[txHash] = finalResult
	votingCacheMutex.Unlock()

	// Mark progress as complete
	progressMutex.Lock()
	voteParsingProgress[txHash] = &types.VoteParsingProgress{
		IsParsing:     false,
		Progress:      100,
		CurrentBlock:  votingEndBlock,
		TotalBlocks:   totalBlocks,
		YesVotes:      yesVotes,
		NoVotes:       noVotes,
		EstimatedTime: 0,
		Message:       "Vote counting complete",
	}
	progressMutex.Unlock()

	log.Printf("Vote counting complete for tspend %s: %d yes, %d no (%.1f%% approval)",
		txHash, yesVotes, noVotes, approvalRate)
}

// countTSpendVotesInRange scans blocks and counts votes for a specific tspend
func countTSpendVotesInRange(ctx context.Context, txHash string, startHeight, endHeight int64) (yesVotes int, noVotes int, err error) {
	// Limit the scan range for performance
	maxScanRange := int64(3000)
	if endHeight-startHeight > maxScanRange {
		startHeight = endHeight - maxScanRange
	}

	// Scan blocks in range
	for height := startHeight; height <= endHeight; height++ {
		blockHash, err := rpc.DcrdClient.GetBlockHash(ctx, height)
		if err != nil {
			continue
		}

		// Get block with stake transactions
		blockResult, err := rpc.DcrdClient.RawRequest(ctx, "getblock", []json.RawMessage{
			json.RawMessage(fmt.Sprintf(`"%s"`, blockHash.String())),
			json.RawMessage("true"),
			json.RawMessage("false"),
		})
		if err != nil {
			continue
		}

		var block struct {
			STx []string `json:"stx"` // Stake transactions
		}
		if err := json.Unmarshal(blockResult, &block); err != nil {
			continue
		}

		// Check each stake transaction for votes on this tspend
		for _, stxHash := range block.STx {
			tx, err := getTransaction(ctx, stxHash)
			if err != nil {
				continue
			}

			// Check if it's a vote transaction
			if !isVoteTransaction(tx) {
				continue
			}

			// Parse vote for this tspend
			vote := parseTSpendVote(tx, txHash)
			if vote == "yes" {
				yesVotes++
			} else if vote == "no" {
				noVotes++
			}
		}
	}

	return yesVotes, noVotes, nil
}

// isVoteTransaction checks if a transaction is a vote (SSGen)
func isVoteTransaction(tx map[string]interface{}) bool {
	vin, ok := tx["vin"].([]interface{})
	if !ok || len(vin) == 0 {
		return false
	}

	firstVin, ok := vin[0].(map[string]interface{})
	if !ok {
		return false
	}

	// Vote transactions have stakebase input
	_, hasStakebase := firstVin["stakebase"]
	return hasStakebase
}

// parseTSpendVote attempts to parse vote bits to determine vote on tspend
func parseTSpendVote(tx map[string]interface{}, tspendHash string) string {
	// Get vout to extract vote bits
	vout, ok := tx["vout"].([]interface{})
	if !ok || len(vout) < 2 {
		return "unknown"
	}

	// Vote transactions have multiple OP_RETURN outputs
	// We need to find the one that contains the tspend hash
	for _, output := range vout {
		voutMap, ok := output.(map[string]interface{})
		if !ok {
			continue
		}

		scriptPubKey, ok := voutMap["scriptPubKey"].(map[string]interface{})
		if !ok {
			continue
		}

		scriptType, _ := scriptPubKey["type"].(string)
		if scriptType != "nulldata" {
			continue
		}

		// Get hex data
		hexData, ok := scriptPubKey["hex"].(string)
		if !ok || hexData == "" {
			continue
		}

		// Check if this output contains tspend vote data
		vote := parseVoteBitsForTSpend(hexData, tspendHash)
		if vote != "unknown" {
			return vote
		}
	}

	return "unknown"
}

// parseVoteBitsForTSpend extracts tspend vote from vote bits hex
// Treasury spend votes are encoded in OP_RETURN outputs that contain:
// [OP_RETURN][length][prefix][tspend_hash_reversed][vote_bits]
func parseVoteBitsForTSpend(hexData string, tspendHash string) string {
	// Decode hex to bytes
	if len(hexData) < 4 {
		return "unknown"
	}

	// Skip OP_RETURN opcode (0x6a) and length byte
	dataStart := 4
	if len(hexData) <= dataStart {
		return "unknown"
	}

	dataHex := hexData[dataStart:]

	// TSpend votes have a 2-byte prefix before the hash
	// Format: [2 bytes prefix][32 bytes tspend hash][1+ bytes vote bits]
	// Skip the first 2 bytes (4 hex chars)
	if len(dataHex) < 4 {
		return "unknown"
	}

	dataWithoutPrefix := dataHex[4:]

	// Tspend hash is 32 bytes (64 hex chars)
	if len(dataWithoutPrefix) < 64 {
		return "unknown"
	}

	// Extract the hash portion (first 64 hex chars = 32 bytes)
	hashHex := dataWithoutPrefix[:64]

	// Reverse the hash bytes to match tspend format
	// Transaction hashes are stored in reverse byte order
	reversedHash := reverseHexBytes(hashHex)

	// Check if this matches our tspend hash
	if !strings.EqualFold(reversedHash, tspendHash) {
		return "unknown"
	}

	// Found matching tspend! Now extract vote bits
	// Vote bits follow the hash
	if len(dataWithoutPrefix) < 66 { // Need at least 2 more hex chars for vote byte
		return "unknown"
	}

	// Get the vote byte (after the 64-char hash)
	voteHex := dataWithoutPrefix[64:66]
	var voteByte byte
	fmt.Sscanf(voteHex, "%02x", &voteByte)

	// Extract the vote choice from the byte
	// Tspend votes use 2 bits: 00=abstain, 01=yes, 10=no, 11=invalid
	// The vote bits are in the lower 2 bits
	voteChoice := voteByte & 0x03

	switch voteChoice {
	case 0x00:
		return "abstain"
	case 0x01:
		return "yes"
	case 0x02:
		return "no"
	case 0x03:
		return "invalid"
	default:
		return "unknown"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// reverseHexBytes reverses a hex string by bytes
// Input: "abcd1234" -> Output: "3412cdab"
func reverseHexBytes(hexStr string) string {
	if len(hexStr)%2 != 0 {
		return hexStr
	}

	result := make([]byte, len(hexStr))
	for i := 0; i < len(hexStr); i += 2 {
		// Copy each byte pair in reverse order
		srcPos := len(hexStr) - i - 2
		result[i] = hexStr[srcPos]
		result[i+1] = hexStr[srcPos+1]
	}

	return string(result)
}

// getBlockTimestamps retrieves timestamps for start and end blocks
func getBlockTimestamps(ctx context.Context, startHeight, endHeight int64) (time.Time, time.Time) {
	var startTime, endTime time.Time

	// Get start block timestamp
	if startHash, err := rpc.DcrdClient.GetBlockHash(ctx, startHeight); err == nil {
		if result, err := rpc.DcrdClient.RawRequest(ctx, "getblockheader", []json.RawMessage{
			json.RawMessage(fmt.Sprintf(`"%s"`, startHash.String())),
		}); err == nil {
			var header struct {
				Time int64 `json:"time"`
			}
			if err := json.Unmarshal(result, &header); err == nil {
				startTime = time.Unix(header.Time, 0)
			}
		}
	}

	// Get end block timestamp
	if endHash, err := rpc.DcrdClient.GetBlockHash(ctx, endHeight); err == nil {
		if result, err := rpc.DcrdClient.RawRequest(ctx, "getblockheader", []json.RawMessage{
			json.RawMessage(fmt.Sprintf(`"%s"`, endHash.String())),
		}); err == nil {
			var header struct {
				Time int64 `json:"time"`
			}
			if err := json.Unmarshal(result, &header); err == nil {
				endTime = time.Unix(header.Time, 0)
			}
		}
	}

	return startTime, endTime
}
