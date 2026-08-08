// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// Constants for treasury
const (
	TreasuryActivationHeight = 552448 // Block where treasury was first activated (May 2021)

	// TreasuryVoteInterval is mainnet's TVI. Per DCP-0006 a TSpend may only be
	// mined in a block whose height is a non-zero multiple of the TVI (dcrd
	// standalone.IsTreasuryVoteInterval / ErrNotTVI), so only these blocks can
	// contain a TSpend and the historical scan can stride by it. Mainnet-
	// specific, like TreasuryActivationHeight.
	TreasuryVoteInterval = 288

	// A block dropped from the scan leaves a permanent hole in the treasury
	// history, so retry it. A full scan reads only ~1,900 blocks.
	scanBlockAttempts   = 3
	scanBlockRetryDelay = 500 * time.Millisecond
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
	scanFailedCount   int                   // Blocks the last scan could not read
	scanSafeHeight    int64                 // Height a later scan may resume above
)

// FetchTreasuryInfo gets current treasury status including balance and active TSpends
// Note: Historical TSpends are tracked in frontend localStorage, not fetched here
func FetchTreasuryInfo(ctx context.Context) (*types.TreasuryInfo, error) {
	// Get current treasury balance
	balance, err := getTreasuryBalance(ctx)
	if err != nil {
		govnLog.Warnf("Failed to get treasury balance: %v", err)
		balance = 0
	}

	// Scan mempool for active TSpends (pending votes)
	activeTSpends, err := scanMempoolForTSpends(ctx)
	if err != nil {
		govnLog.Warnf("Failed to scan mempool for TSpends: %v", err)
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

	return dcrutil.Amount(treasuryBalance.Balance).ToCoin(), nil
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

	// dcrd filters the mempool by transaction type, so ask it for the treasury
	// spends rather than fetching every entry to classify it here.
	hashes, err := rpc.DcrdClient.GetRawMempool(ctx, chainjson.GRMTSpend)
	if err != nil {
		return nil, fmt.Errorf("failed to get mempool: %w", err)
	}

	var tspends []types.TSpend
	currentHeight, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		govnLog.Warnf("Failed to get current height: %v", err)
		currentHeight = 0
	}

	for _, hash := range hashes {
		tx, err := getTransaction(ctx, hash.String())
		if err != nil {
			govnLog.Warnf("Failed to get transaction %s: %v", hash, err)
			continue
		}

		tspend := extractTSpendInfo(tx, currentHeight)
		if tspend != nil {
			tspends = append(tspends, *tspend)
		}
	}

	// Attach yes/no vote tallies in a single RPC call. A mempool tspend is
	// inside its voting window by definition, so the best block is the right
	// point to count to. Best-effort; tspends are still returned if this fails.
	if len(tspends) > 0 {
		hashes := make([]string, len(tspends))
		for i := range tspends {
			hashes[i] = tspends[i].TxHash
		}
		tally, err := tspendVoteTally(ctx, nil, hashes)
		if err != nil {
			govnLog.Warnf("Gettreasuryspendvotes: %v", err)
		} else {
			for i := range tspends {
				if v, ok := tally[tspends[i].TxHash]; ok {
					tspends[i].YesVotes = v.YesVotes
					tspends[i].NoVotes = v.NoVotes
				}
			}
		}
	}

	return tspends, nil
}

// Treasury balance-over-time series, sampled at a coarse cadence and cached.
var (
	balanceHistMu   sync.RWMutex
	balanceHistData []types.BalanceSample
	balanceHistAt   time.Time
)

const (
	balanceHistTTL      = 1 * time.Hour
	balanceSampleStride = 8640 // ~30 days of blocks (5 min/block)
)

// TreasuryBalanceHistory returns the treasury balance sampled from activation
// to tip at ~monthly cadence (plus the tip). Cheap: ~1 + 2/sample RPC calls
// (~120 total). Cached in-process for balanceHistTTL.
func TreasuryBalanceHistory(ctx context.Context) ([]types.BalanceSample, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}

	balanceHistMu.RLock()
	if balanceHistData != nil && time.Since(balanceHistAt) < balanceHistTTL {
		cached := balanceHistData
		balanceHistMu.RUnlock()
		return cached, nil
	}
	balanceHistMu.RUnlock()

	tip, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("get block count: %w", err)
	}

	var out []types.BalanceSample
	for h := int64(TreasuryActivationHeight); h <= tip; h += balanceSampleStride {
		s, err := balanceSampleAt(ctx, h)
		if err != nil {
			govnLog.Warnf("Treasury balance sample at %d: %v", h, err)
			continue
		}
		out = append(out, *s)
	}
	// Always include the current tip as the final point.
	if len(out) == 0 || out[len(out)-1].Height != tip {
		if s, err := balanceSampleAt(ctx, tip); err == nil {
			out = append(out, *s)
		}
	}

	balanceHistMu.Lock()
	balanceHistData = out
	balanceHistAt = time.Now()
	balanceHistMu.Unlock()
	return out, nil
}

// balanceSampleAt returns the treasury balance + block time at one height.
func balanceSampleAt(ctx context.Context, h int64) (*types.BalanceSample, error) {
	hash, err := rpc.DcrdClient.GetBlockHash(ctx, h)
	if err != nil {
		return nil, err
	}
	bal, err := rpc.DcrdClient.GetTreasuryBalance(ctx, hash, false)
	if err != nil {
		return nil, err
	}
	hres, err := rpc.DcrdClient.RawRequest(ctx, "getblockheader", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, hash.String())),
		json.RawMessage("true"),
	})
	if err != nil {
		return nil, err
	}
	var hdr struct {
		Time int64 `json:"time"`
	}
	_ = json.Unmarshal(hres, &hdr)
	return &types.BalanceSample{
		Height:  h,
		Time:    hdr.Time,
		Balance: dcrutil.Amount(bal.Balance).ToCoin(),
	}, nil
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
	scanFailedCount = 0
	scanSafeHeight = 0
	scanMutex.Unlock()

	go scanHistoricalTSpendsBackground(startHeight)
	return nil
}

// safeResumeHeight returns the height a later scan may safely resume above. A
// block that could not be read has to be revisited, so the watermark stops
// below the first such block no matter how many later ones succeeded.
func safeResumeHeight(lastScanned int64, failed []int64) int64 {
	first, found := int64(0), false
	for _, h := range failed {
		if !found || h < first {
			first, found = h, true
		}
	}
	if !found {
		return lastScanned
	}
	if first < 1 {
		return 0
	}
	return first - 1
}

// scanBlock is the part of a verbose getblock reply the historical scan reads.
type scanBlock struct {
	Hash   string                   `json:"hash"`
	Height int64                    `json:"height"`
	Time   int64                    `json:"time"`
	RawTx  []map[string]interface{} `json:"rawtx"`
	RawSTx []map[string]interface{} `json:"rawstx"`
}

// readScanBlock fetches one block for the historical scan.
func readScanBlock(ctx context.Context, height int64) (*scanBlock, error) {
	blockHash, err := rpc.DcrdClient.GetBlockHash(ctx, height)
	if err != nil {
		return nil, fmt.Errorf("get block hash: %w", err)
	}

	// verbose=true + verbosetx=true returns every tx's full vin/vout inline
	// (rawtx/rawstx), so no per-transaction getrawtransaction call is needed.
	blockResult, err := rpc.DcrdClient.RawRequest(ctx, "getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, blockHash.String())),
		json.RawMessage("true"),
		json.RawMessage("true"),
	})
	if err != nil {
		return nil, fmt.Errorf("getblock %s: %w", blockHash, err)
	}

	var block scanBlock
	if err := json.Unmarshal(blockResult, &block); err != nil {
		return nil, fmt.Errorf("decode block %s: %w", blockHash, err)
	}
	return &block, nil
}

// readScanBlockRetry retries readScanBlock, so that a transient dcrd failure
// does not silently cost the scan a block.
func readScanBlockRetry(ctx context.Context, height int64) (*scanBlock, error) {
	var err error
	for attempt := 1; attempt <= scanBlockAttempts; attempt++ {
		var block *scanBlock
		if block, err = readScanBlock(ctx, height); err == nil {
			return block, nil
		}
		govnLog.Warnf("Failed to read block %d (attempt %d/%d): %v", height, attempt, scanBlockAttempts, err)

		if attempt < scanBlockAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(scanBlockRetryDelay):
			}
		}
	}
	return nil, err
}

// scanHistoricalTSpendsBackground performs the historical scan in the background
func scanHistoricalTSpendsBackground(startHeight int64) {
	ctx := context.Background()

	currentHeight, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		govnLog.Errorf("Error getting block count for scan: %v", err)
		scanMutex.Lock()
		isScanRunning = false
		// No block was reached, so hold the resume point below the start
		// rather than let the client advance over unscanned ground.
		scanSafeHeight = safeResumeHeight(startHeight, []int64{startHeight})
		scanMutex.Unlock()
		return
	}

	scanMutex.Lock()
	totalScanHeight = currentHeight
	scanMutex.Unlock()

	// TSpends may only be mined in blocks on a treasury-vote-interval (TVI)
	// boundary (height % TVI == 0), so stride by the TVI and skip the ~99.7%
	// of blocks that cannot contain one. Align the start up to the first TVI
	// boundary at/after startHeight.
	firstTVI := startHeight
	if firstTVI < 1 {
		firstTVI = 1
	}
	if rem := firstTVI % TreasuryVoteInterval; rem != 0 {
		firstTVI += TreasuryVoteInterval - rem
	}

	govnLog.Infof("Starting historical TSpend scan from block %d to %d (TVI stride %d)", firstTVI, currentHeight, TreasuryVoteInterval)

	var failedHeights []int64

	for h := firstTVI; h <= currentHeight; h += TreasuryVoteInterval {
		// Update progress
		scanMutex.Lock()
		currentScanHeight = h
		scanMutex.Unlock()

		block, err := readScanBlockRetry(ctx, h)
		if err != nil {
			govnLog.Errorf("Dropping block %d from the scan after %d attempts: %v", h, scanBlockAttempts, err)
			failedHeights = append(failedHeights, h)
			continue
		}

		allTxs := append(block.RawTx, block.RawSTx...)
		for _, tx := range allTxs {
			if isTreasurySpend(tx) {
				history := extractTSpendHistory(tx, block.Height, block.Hash, block.Time)
				if history != nil {
					scanMutex.Lock()
					scanResults = append(scanResults, *history)
					newTSpendBuffer = append(newTSpendBuffer, *history)
					tspendFoundCount++
					govnLog.Infof("TSpend found at height %d: %s (amount: %.2f DCR)", block.Height, history.TxHash, history.Amount)
					scanMutex.Unlock()
				}
			}
		}
	}

	scanMutex.Lock()
	isScanRunning = false
	scanFailedCount = len(failedHeights)
	scanSafeHeight = safeResumeHeight(currentScanHeight, failedHeights)
	found, safeHeight := tspendFoundCount, scanSafeHeight
	scanMutex.Unlock()

	if len(failedHeights) > 0 {
		govnLog.Warnf("Historical TSpend scan finished with %d unread block(s), first at %d. Found %d TSpends; resume height held at %d",
			len(failedHeights), failedHeights[0], found, safeHeight)
		return
	}
	govnLog.Infof("Historical TSpend scan complete. Found %d TSpends", found)
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
		switch {
		case scanFailedCount > 0:
			message = fmt.Sprintf("Scan incomplete: %d block(s) could not be read. Found %d treasury spends so far; scan again to cover the rest",
				scanFailedCount, tspendFoundCount)
		case tspendFoundCount > 0:
			message = fmt.Sprintf("Scan complete. Found %d treasury spends", tspendFoundCount)
		default:
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
		FailedBlocks:  scanFailedCount,
		SafeHeight:    scanSafeHeight,
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

// tspendVoteTally asks dcrd for the yes/no counts of the given tspends, keyed by
// hash. A nil block tallies against the best block; an explicit block is needed
// once the voting window has closed, because dcrd counts nothing for a window
// that no longer contains the requested height.
func tspendVoteTally(ctx context.Context, block *chainhash.Hash, hashes []string) (map[string]chainjson.TreasurySpendVotes, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}
	if len(hashes) == 0 {
		return nil, nil
	}

	// An empty tspend list would make dcrd answer for every mempool tspend
	// instead, so only ever call this with hashes in hand.
	tspends := make([]*chainhash.Hash, 0, len(hashes))
	for _, h := range hashes {
		hash, err := chainhash.NewHashFromStr(h)
		if err != nil {
			return nil, fmt.Errorf("invalid tspend hash %q: %w", h, err)
		}
		tspends = append(tspends, hash)
	}

	res, err := rpc.DcrdClient.GetTreasurySpendVotes(ctx, block, tspends)
	if err != nil {
		return nil, err
	}

	out := make(map[string]chainjson.TreasurySpendVotes, len(res.Votes))
	for _, v := range res.Votes {
		out[v.Hash] = v
	}
	return out, nil
}

// GetTSpendVotingInfo reports the vote tally and derived approval state for a
// treasury spend. The counts come from dcrd's gettreasuryspendvotes; quorum and
// the required-yes threshold are derived from the network's consensus
// parameters so they match what the chain itself enforces.
func GetTSpendVotingInfo(ctx context.Context, txHash string, blockHeight int64, expiry uint32, inMempool bool) (*types.TSpendVotingInfo, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}

	params, err := chainParams(ctx)
	if err != nil {
		return nil, err
	}

	start32, end32, err := standalone.CalcTSpendWindow(expiry, params.TreasuryVoteInterval,
		params.TreasuryVoteIntervalMultiplier)
	if err != nil {
		return nil, fmt.Errorf("treasury spend %s has an invalid expiry %d: %w", txHash, expiry, err)
	}
	voteStart, voteEnd := int64(start32), int64(end32)

	tip, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current height: %w", err)
	}
	mined := !inMempool && blockHeight > 0

	queryHeight, explicitBlock, countedThrough := tspendCountHeights(tip, voteEnd, blockHeight, mined)

	var queryBlock *chainhash.Hash
	if explicitBlock {
		hash, herr := rpc.DcrdClient.GetBlockHash(ctx, queryHeight)
		if herr != nil {
			return nil, fmt.Errorf("failed to get block hash at %d: %w", queryHeight, herr)
		}
		queryBlock = hash
	}

	var yes, no int64
	if tip >= voteStart {
		tally, terr := tspendVoteTally(ctx, queryBlock, []string{txHash})
		if terr != nil {
			return nil, fmt.Errorf("gettreasuryspendvotes %s: %w", txHash, terr)
		}
		if v, ok := tally[txHash]; ok {
			yes, no = v.YesVotes, v.NoVotes
			voteStart, voteEnd = v.VoteStart, v.VoteEnd
		}
	}

	stats := evalTSpendVotes(params, voteStart, voteEnd, countedThrough, yes, no)

	var approvalRate, turnoutRate float64
	if stats.VotesCast > 0 {
		approvalRate = float64(yes) / float64(stats.VotesCast) * 100
	}
	if stats.MaxVotes > 0 {
		turnoutRate = float64(stats.VotesCast) / float64(stats.MaxVotes) * 100
	}

	startTime, endTime, endEstimated := tspendVotingTimes(ctx, params, voteStart, voteEnd, tip, blockHeight, mined)

	return &types.TSpendVotingInfo{
		VotingStartBlock: voteStart,
		VotingEndBlock:   voteEnd,
		YesVotes:         yes,
		NoVotes:          no,
		EligibleVotes:    stats.MaxVotes,
		VotesCast:        stats.VotesCast,
		QuorumRequired:   stats.QuorumRequired,
		ApprovalRate:     approvalRate,
		TurnoutRate:      turnoutRate,
		RequiredApprovalPct: float64(params.TreasuryVoteRequiredMultiplier) /
			float64(params.TreasuryVoteRequiredDivisor) * 100,
		QuorumAchieved: stats.QuorumAchieved,
		// A mined spend was accepted by consensus; nothing left to decide.
		Approved:           mined || stats.Approved,
		VotingComplete:     mined || tip >= voteEnd,
		InMempool:          inMempool,
		VotingStartTime:    startTime,
		VotingEndTime:      endTime,
		VotingEndEstimated: endEstimated,
	}, nil
}

// tspendCountHeights decides which block to ask dcrd about and which height the
// resulting tally actually covers. While the window is open the best block is
// inside it, so dcrd's default is already right; once it has closed a block
// inside the window must be named or the tally comes back empty. voteEnd itself
// is rejected, because dcrd tallies for the block after the one requested. A
// mined spend is only ever counted up to the block before it was mined,
// whichever block we ask about.
func tspendCountHeights(tip, voteEnd, minedHeight int64, mined bool) (queryHeight int64, explicitBlock bool, countedThrough int64) {
	queryHeight = tip
	if tip >= voteEnd {
		queryHeight = voteEnd - 1
		explicitBlock = true
	}
	countedThrough = queryHeight
	if mined {
		countedThrough = minedHeight - 1
	}
	return queryHeight, explicitBlock, countedThrough
}

// tspendVoteStats is what consensus would make of a tally at a point in the
// voting window.
type tspendVoteStats struct {
	MaxVotes       int64
	QuorumRequired int64
	VotesCast      int64
	QuorumAchieved bool
	RequiredVotes  int64
	Approved       bool
}

// evalTSpendVotes mirrors dcrd's checkTSpendHasVotes. Quorum is measured against
// the whole window and never shrinks, and every block still left in the window
// is treated as a potential no vote, so approval is only declared early when it
// can no longer be lost. The remaining-block count is clamped because a stale
// mined height (after a reorg) would otherwise drive it negative.
func evalTSpendVotes(params *chaincfg.Params, voteStart, voteEnd, countedThrough, yes, no int64) tspendVoteStats {
	ticketsPerBlock := int64(params.TicketsPerBlock)
	maxVotes := ticketsPerBlock * (voteEnd - voteStart)
	quorum := maxVotes * int64(params.TreasuryVoteQuorumMultiplier) /
		int64(params.TreasuryVoteQuorumDivisor)
	cast := yes + no

	remaining := voteEnd - (countedThrough + 1)
	if remaining < 0 {
		remaining = 0
	}
	required := (cast + remaining*ticketsPerBlock) *
		int64(params.TreasuryVoteRequiredMultiplier) / int64(params.TreasuryVoteRequiredDivisor)

	return tspendVoteStats{
		MaxVotes:       maxVotes,
		QuorumRequired: quorum,
		VotesCast:      cast,
		QuorumAchieved: cast >= quorum,
		RequiredVotes:  required,
		Approved:       cast >= quorum && yes >= required,
	}
}

// tspendVotingTimes resolves when voting opened and closed. The end is projected
// from the tip while the window is still open, so the response never carries a
// zero timestamp for the UI to render.
func tspendVotingTimes(ctx context.Context, params *chaincfg.Params, voteStart, voteEnd, tip, minedHeight int64, mined bool) (time.Time, time.Time, bool) {
	startTime := blockTime(ctx, voteStart)

	switch {
	case mined:
		return startTime, blockTime(ctx, minedHeight), false
	case voteEnd <= tip:
		return startTime, blockTime(ctx, voteEnd), false
	default:
		tipTime := blockTime(ctx, tip)
		if tipTime.IsZero() {
			return startTime, time.Time{}, false
		}
		return startTime, tipTime.Add(time.Duration(voteEnd-tip) * params.TargetTimePerBlock), true
	}
}

// blockTime returns the timestamp of the block at height, or the zero time when
// it cannot be resolved (typically a height above the tip).
func blockTime(ctx context.Context, height int64) time.Time {
	if rpc.DcrdClient == nil || height < 0 {
		return time.Time{}
	}
	hash, err := rpc.DcrdClient.GetBlockHash(ctx, height)
	if err != nil {
		return time.Time{}
	}
	header, err := rpc.DcrdClient.GetBlockHeader(ctx, hash)
	if err != nil {
		return time.Time{}
	}
	return header.Timestamp
}
