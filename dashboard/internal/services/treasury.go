// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/rpcclient/v8"
)

// Constants for treasury
const (
	TreasuryActivationHeight = 552448 // Block where treasury was first activated (May 2021)

	// A block the scan cannot read ends it, so retry it first.
	scanBlockAttempts   = 3
	scanBlockRetryDelay = 500 * time.Millisecond
)

// Global scan state
var (
	scanMutex         sync.RWMutex
	isScanRunning     bool
	scanStartHeight   int64
	currentScanHeight int64
	totalScanHeight   int64
	tspendFoundCount  int
	taddFoundCount    int
	scanResults       []types.TSpendHistory
	scanTAdds         []types.TreasuryTAdd
	scanTBase         map[string]int64
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
		Balance:       balance.ToCoin(),
		BalanceAtoms:  int64(balance),
		BalanceUSD:    0, // TODO: Add USD conversion if needed
		TotalAdded:    0, // Tracked in frontend localStorage
		TotalSpent:    0, // Tracked in frontend localStorage
		ActiveTSpends: activeTSpends,
		RecentTSpends: []types.TSpendHistory{}, // Not used - data comes from localStorage
		LastUpdate:    time.Now(),
	}, nil
}

// getTreasuryBalance retrieves current treasury balance from dcrd
func getTreasuryBalance(ctx context.Context) (dcrutil.Amount, error) {
	if rpc.DcrdClient == nil {
		return 0, fmt.Errorf("dcrd client not available")
	}

	treasuryBalance, err := rpc.DcrdClient.GetTreasuryBalance(ctx, nil, false)
	if err != nil {
		return 0, fmt.Errorf("failed to get treasury balance: %w", err)
	}

	return dcrutil.Amount(treasuryBalance.Balance), nil
}

// scanMempoolForTSpends scans the mempool for active treasury spend transactions
func scanMempoolForTSpends(ctx context.Context) ([]types.TSpend, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}

	// dcrd filters the mempool by transaction type, so ask it for the treasury
	// spends rather than fetching every entry to classify it here.
	hashes, err := mempoolHashes(ctx, chainjson.GRMTSpend)
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
		tx, err := rpc.DcrdClient.GetRawTransactionVerbose(ctx, hash)
		if err != nil {
			govnLog.Warnf("Failed to get transaction %s: %v", hash, err)
			continue
		}

		tspend := extractTSpendInfo(*tx, currentHeight)
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

// Treasury balance-over-time series, sampled at the first block of every UTC
// month and cached. Month starts never change once found, so only new months
// and the tip are fetched on refresh.
var (
	balanceHistMu     sync.Mutex
	balanceHistMonths []types.BalanceSample
	balanceHistData   []types.BalanceSample
	balanceHistAt     time.Time
)

const balanceHistTTL = 1 * time.Hour

// TreasuryBalanceHistory returns the treasury balance at activation, at the
// first block of every UTC month since, and at the tip. Cached in-process for
// balanceHistTTL.
func TreasuryBalanceHistory(ctx context.Context) ([]types.BalanceSample, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}

	balanceHistMu.Lock()
	defer balanceHistMu.Unlock()
	if balanceHistData != nil && time.Since(balanceHistAt) < balanceHistTTL {
		return balanceHistData, nil
	}

	tip, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("get block count: %w", err)
	}
	months, err := extendMonthStarts(ctx, balanceHistMonths, tip, balanceSampleAt, blockTimeAt)
	if err != nil {
		return nil, err
	}
	balanceHistMonths = months

	out := append([]types.BalanceSample(nil), months...)
	if out[len(out)-1].Height != tip {
		if s, err := balanceSampleAt(ctx, tip); err == nil {
			out = append(out, *s)
		}
	}
	balanceHistData = out
	balanceHistAt = time.Now()
	return out, nil
}

// extendMonthStarts appends a sample at the first block of every UTC month
// that has begun by the tip, starting from the activation block.
func extendMonthStarts(ctx context.Context, have []types.BalanceSample, tip int64,
	sampleAt func(context.Context, int64) (*types.BalanceSample, error),
	timeAt func(context.Context, int64) (int64, error)) ([]types.BalanceSample, error) {

	out := append([]types.BalanceSample(nil), have...)
	if len(out) == 0 {
		s, err := sampleAt(ctx, TreasuryActivationHeight)
		if err != nil {
			return nil, fmt.Errorf("treasury balance at activation: %w", err)
		}
		out = append(out, *s)
	}
	tipTime, err := timeAt(ctx, tip)
	if err != nil {
		return nil, fmt.Errorf("tip time: %w", err)
	}
	for {
		last := out[len(out)-1]
		next := nextMonthStart(last.Time)
		if next > tipTime || last.Height >= tip {
			return out, nil
		}
		h, err := firstBlockAtOrAfter(ctx, last.Height+1, tip, next, timeAt)
		if err != nil {
			return nil, fmt.Errorf("first block of %s: %w", time.Unix(next, 0).UTC().Format("2006-01"), err)
		}
		s, err := sampleAt(ctx, h)
		if err != nil {
			return nil, fmt.Errorf("treasury balance at %d: %w", h, err)
		}
		out = append(out, *s)
	}
}

// nextMonthStart returns the start of the UTC month after the given time.
func nextMonthStart(unix int64) int64 {
	t := time.Unix(unix, 0).UTC()
	return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC).Unix()
}

// firstBlockAtOrAfter returns the lowest height in [lo, hi] whose block time
// is at or after cutoff, or hi when none is.
func firstBlockAtOrAfter(ctx context.Context, lo, hi, cutoff int64,
	timeAt func(context.Context, int64) (int64, error)) (int64, error) {

	for lo < hi {
		mid := lo + (hi-lo)/2
		t, err := timeAt(ctx, mid)
		if err != nil {
			return 0, err
		}
		if t >= cutoff {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo, nil
}

// blockTimeAt returns the block time at one height.
func blockTimeAt(ctx context.Context, h int64) (int64, error) {
	hash, err := rpc.DcrdClient.GetBlockHash(ctx, h)
	if err != nil {
		return 0, err
	}
	hdr, err := rpc.DcrdClient.GetBlockHeaderVerbose(ctx, hash)
	if err != nil {
		return 0, err
	}
	return hdr.Time, nil
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
	hdr, err := rpc.DcrdClient.GetBlockHeaderVerbose(ctx, hash)
	if err != nil {
		return nil, err
	}
	return &types.BalanceSample{
		Height:  h,
		Time:    hdr.Time,
		Balance: dcrutil.Amount(bal.Balance).ToCoin(),
	}, nil
}

// extractTSpendInfo extracts TSpend information from a transaction
func extractTSpendInfo(tx chainjson.TxRawResult, currentHeight int64) *types.TSpend {
	// Sum the outputs; the last address-bearing output names the payee.
	amount := 0.0
	payee := ""
	for _, vout := range tx.Vout {
		amount += vout.Value
		if len(vout.ScriptPubKey.Addresses) > 0 {
			payee = vout.ScriptPubKey.Addresses[0]
		}
	}

	expiryHeight := int64(tx.Expiry)
	blocksRemaining := expiryHeight - currentHeight

	return &types.TSpend{
		TxHash:          tx.Txid,
		Amount:          amount,
		Payee:           payee,
		ExpiryHeight:    expiryHeight,
		CurrentHeight:   currentHeight,
		BlocksRemaining: blocksRemaining,
		Status:          "voting",
		DetectedAt:      time.Now(),
	}
}

// ErrInvalidScanHeight identifies a start beyond the captured chain tip.
var ErrInvalidScanHeight = errors.New("invalid treasury scan start height")

// TriggerHistoricalScan validates admission before replacing shared scan state.
// Once admitted, the background scan outlives the requesting HTTP/MCP call.
func TriggerHistoricalScan(ctx context.Context, startHeight int64) error {
	scanMutex.RLock()
	running := isScanRunning
	scanMutex.RUnlock()
	if running {
		return fmt.Errorf("scan already in progress")
	}
	if startHeight < TreasuryActivationHeight {
		startHeight = TreasuryActivationHeight
	}
	client := rpc.DcrdClient
	if client == nil {
		return fmt.Errorf("dcrd client not available")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	currentHeight, err := client.GetBlockCount(ctx)
	if err != nil {
		return fmt.Errorf("get block count for scan: %w", err)
	}
	if currentHeight < 0 {
		return fmt.Errorf("invalid chain tip: %d", currentHeight)
	}
	if startHeight > currentHeight {
		return fmt.Errorf("%w: requested %d exceeds current chain tip %d", ErrInvalidScanHeight, startHeight, currentHeight)
	}

	scanMutex.Lock()
	defer scanMutex.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if isScanRunning {
		return fmt.Errorf("scan already in progress")
	}
	isScanRunning = true
	scanStartHeight = startHeight
	currentScanHeight = startHeight
	totalScanHeight = currentHeight
	tspendFoundCount = 0
	taddFoundCount = 0
	scanResults = []types.TSpendHistory{}
	scanTAdds = []types.TreasuryTAdd{}
	scanTBase = map[string]int64{}
	newTSpendBuffer = []types.TSpendHistory{}
	scanFailedCount = 0
	scanSafeHeight = 0
	go scanHistoricalTSpendsBackground(client, startHeight, currentHeight)
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

// scanHistoricalTSpendsBackground reads every block from startHeight to
// currentHeight and records what each paid into and out of the treasury. It
// stops at the first block it cannot read, so the results always cover one
// unbroken range of blocks.
func scanHistoricalTSpendsBackground(client *rpcclient.Client, startHeight, currentHeight int64) {
	ctx := context.Background()
	var failedHeights []int64
	defer func() {
		scanMutex.Lock()
		isScanRunning = false
		scanFailedCount = len(failedHeights)
		scanSafeHeight = safeResumeHeight(currentScanHeight, failedHeights)
		found, safeHeight := tspendFoundCount, scanSafeHeight
		scanMutex.Unlock()
		if len(failedHeights) > 0 {
			govnLog.Warnf("Treasury scan stopped at unreadable block %d. Found %d TSpends; resume height held at %d",
				failedHeights[0], found, safeHeight)
			return
		}
		govnLog.Infof("Treasury scan complete. Found %d TSpends", found)
	}()

	params, err := chainParams(ctx)
	if err != nil {
		govnLog.Errorf("Treasury scan cannot start: %v", err)
		failedHeights = append(failedHeights, startHeight)
		return
	}
	govnLog.Infof("Starting treasury scan from block %d to %d", startHeight, currentHeight)

	for h := startHeight; h <= currentHeight; h++ {
		scanMutex.Lock()
		currentScanHeight = h
		scanMutex.Unlock()

		f, err := readTreasuryFlowsRetry(ctx, client, h, params)
		if err != nil {
			govnLog.Errorf("Treasury scan stops at block %d after %d attempts: %v", h, scanBlockAttempts, err)
			failedHeights = append(failedHeights, h)
			return
		}

		scanMutex.Lock()
		scanTBase[flowMonth(f.time)] += f.tbase
		scanTAdds = append(scanTAdds, f.tadds...)
		taddFoundCount += len(f.tadds)
		scanResults = append(scanResults, f.tspends...)
		newTSpendBuffer = append(newTSpendBuffer, f.tspends...)
		tspendFoundCount += len(f.tspends)
		scanMutex.Unlock()
		for _, t := range f.tspends {
			govnLog.Infof("TSpend found at height %d: %s (%v)", h, t.TxHash, dcrutil.Amount(t.AmountAtoms))
		}
		if h == currentHeight {
			return
		}
	}
}

// GetScanProgress returns the current scan progress
func GetScanProgress() (*types.TSpendScanProgress, error) {
	scanMutex.Lock()
	defer scanMutex.Unlock()

	progress := 0.0
	if totalScanHeight > TreasuryActivationHeight {
		progress = float64(currentScanHeight-TreasuryActivationHeight) / float64(totalScanHeight-TreasuryActivationHeight) * 100
	}

	message := "Scanning the blockchain for treasury flows..."
	if !isScanRunning {
		switch {
		case scanFailedCount > 0:
			message = fmt.Sprintf("Scan stopped at an unreadable block. Found %d treasury spends and %d contributions so far; scan again to cover the rest",
				tspendFoundCount, taddFoundCount)
		case tspendFoundCount > 0 || taddFoundCount > 0 || len(scanTBase) > 0:
			message = fmt.Sprintf("Scan complete. Found %d treasury spends and %d contributions", tspendFoundCount, taddFoundCount)
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
		TAddFound:     taddFoundCount,
		NewTSpends:    newTSpends,
		Message:       message,
		FailedBlocks:  scanFailedCount,
		SafeHeight:    scanSafeHeight,
	}, nil
}

// GetScanResults returns what the last scan recorded, over the blocks it read.
func GetScanResults() types.TreasuryScanResults {
	scanMutex.RLock()
	defer scanMutex.RUnlock()

	tbase := make(map[string]int64, len(scanTBase))
	for m, v := range scanTBase {
		tbase[m] = v
	}
	return types.TreasuryScanResults{
		FromHeight:   scanStartHeight,
		ToHeight:     scanSafeHeight,
		TSpends:      append([]types.TSpendHistory{}, scanResults...),
		TAdds:        append([]types.TreasuryTAdd{}, scanTAdds...),
		TBaseByMonth: tbase,
	}
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
