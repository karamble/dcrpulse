// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
	"dcrpulse/internal/utils"
)

var (
	// Track previous sync values to calculate delta
	prevHeaders int64
	prevBlocks  int64
	syncMutex   sync.Mutex
)

func FetchDashboardData() (*types.DashboardData, error) {
	nodeStatus, err := FetchNodeStatus()
	if err != nil {
		return nil, err
	}

	blockchainInfo, err := FetchBlockchainInfo()
	if err != nil {
		return nil, err
	}

	networkInfo, err := FetchNetworkInfo()
	if err != nil {
		return nil, err
	}

	peers, err := FetchPeers()
	if err != nil {
		return nil, err
	}

	supplyInfo, err := FetchSupplyInfo()
	if err != nil {
		return nil, err
	}

	stakingInfo, err := FetchStakingInfo()
	if err != nil {
		return nil, err
	}

	mempoolInfo, err := FetchMempoolInfo()
	if err != nil {
		return nil, err
	}

	return &types.DashboardData{
		NodeStatus:     *nodeStatus,
		BlockchainInfo: *blockchainInfo,
		NetworkInfo:    *networkInfo,
		Peers:          peers,
		SupplyInfo:     *supplyInfo,
		StakingInfo:    *stakingInfo,
		MempoolInfo:    *mempoolInfo,
		LastUpdate:     time.Now(),
	}, nil
}

func FetchNodeStatus() (*types.NodeStatus, error) {
	ctx := context.Background()

	// Get version info using version command
	versionInfo, err := rpc.DcrdClient.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %v", err)
	}

	// Get blockchain info for accurate sync status
	chainInfo, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
	if err != nil {
		return nil, err
	}

	// Debug logging
	log.Printf("Blockchain sync status - InitialBlockDownload: %v, Blocks: %d, Headers: %d, SyncHeight: %d, VerificationProgress: %f",
		chainInfo.InitialBlockDownload, chainInfo.Blocks, chainInfo.Headers, chainInfo.SyncHeight, chainInfo.VerificationProgress)

	// Calculate sync progress based on actual blockchain sync
	var syncProgress float64
	var status string
	var syncPhase string
	var syncMessage string

	// Thread-safe access to previous values
	syncMutex.Lock()
	currentHeaders := chainInfo.Headers
	currentBlocks := chainInfo.Blocks
	deltaHeaders := currentHeaders - prevHeaders
	deltaBlocks := currentBlocks - prevBlocks
	prevHeaders = currentHeaders
	prevBlocks = currentBlocks
	syncMutex.Unlock()

	if chainInfo.InitialBlockDownload {
		// Node is still syncing
		status = "syncing"

		// Determine sync phase: headers or blocks
		if chainInfo.Blocks == 0 && chainInfo.Headers > 0 {
			// Syncing headers
			syncPhase = "headers"
			if chainInfo.SyncHeight > 0 {
				syncProgress = (float64(chainInfo.Headers) / float64(chainInfo.SyncHeight)) * 100
			}
			if deltaHeaders > 0 {
				syncMessage = fmt.Sprintf("Processed %s headers in the last 30 seconds", utils.FormatNumber(deltaHeaders))
			} else {
				syncMessage = "Syncing headers..."
			}
		} else if chainInfo.Blocks > 0 {
			// Syncing blocks
			syncPhase = "blocks"
			if chainInfo.SyncHeight > 0 {
				syncProgress = (float64(chainInfo.Blocks) / float64(chainInfo.SyncHeight)) * 100
			}
			if deltaBlocks > 0 {
				syncMessage = fmt.Sprintf("Processed %s blocks in the last 30 seconds", utils.FormatNumber(deltaBlocks))
			} else {
				syncMessage = "Syncing blocks..."
			}
		} else {
			// Initial state
			syncPhase = "starting"
			syncMessage = "Starting sync..."
			syncProgress = 0
		}

		// Use verification progress if available and more accurate
		if chainInfo.VerificationProgress > 0 {
			syncProgress = chainInfo.VerificationProgress * 100
		}
	} else {
		// Node is fully synced
		status = "running"
		syncProgress = 100.0
		syncPhase = "synced"
		syncMessage = "Fully synced"
	}

	// Ensure progress is between 0 and 100
	if syncProgress > 100 {
		syncProgress = 100
	} else if syncProgress < 0 {
		syncProgress = 0
	}

	return &types.NodeStatus{
		Status:       status,
		SyncProgress: syncProgress,
		Version:      fmt.Sprintf("v%d.%d.%d", versionInfo["dcrd"].Major, versionInfo["dcrd"].Minor, versionInfo["dcrd"].Patch),
		SyncPhase:    syncPhase,
		SyncMessage:  syncMessage,
	}, nil
}

func FetchBlockchainInfo() (*types.BlockchainInfo, error) {
	ctx := context.Background()
	info, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
	if err != nil {
		return nil, err
	}

	bestBlockHash, err := rpc.DcrdClient.GetBestBlockHash(ctx)
	if err != nil {
		return nil, err
	}

	blockHeader, err := rpc.DcrdClient.GetBlockHeader(ctx, bestBlockHash)
	if err != nil {
		return nil, err
	}

	// Estimate block time (time since last block)
	timeSinceBlock := time.Since(blockHeader.Timestamp)
	blockTime := fmt.Sprintf("%dm %ds", int(timeSinceBlock.Minutes()), int(timeSinceBlock.Seconds())%60)

	// Fetch last 3 blocks for the recent blocks list
	recentBlocks := make([]types.RecentBlock, 0, 3)
	currentHeight := info.Blocks
	for i := int64(0); i < 3 && currentHeight-i >= 0; i++ {
		blockHash, err := rpc.DcrdClient.GetBlockHash(ctx, currentHeight-i)
		if err != nil {
			log.Printf("Warning: Failed to get block hash for height %d: %v", currentHeight-i, err)
			continue
		}

		header, err := rpc.DcrdClient.GetBlockHeader(ctx, blockHash)
		if err != nil {
			log.Printf("Warning: Failed to get block header for hash %s: %v", blockHash.String(), err)
			continue
		}

		recentBlocks = append(recentBlocks, types.RecentBlock{
			Height:    currentHeight - i,
			Hash:      blockHash.String(),
			Timestamp: header.Timestamp.Unix(),
		})
	}

	return &types.BlockchainInfo{
		BlockHeight:  info.Blocks,
		BlockHash:    bestBlockHash.String(),
		Difficulty:   float64(info.Difficulty),
		ChainSize:    0, // Would need to calculate from disk usage
		BlockTime:    blockTime,
		RecentBlocks: recentBlocks,
	}, nil
}

func FetchNetworkInfo() (*types.NetworkInfo, error) {
	ctx := context.Background()

	// Get peer count
	peerCount := 0
	peerInfo, err := rpc.DcrdClient.GetPeerInfo(ctx)
	if err == nil {
		peerCount = len(peerInfo)
	}

	// Get network difficulty and calculate hashrate - direct RPC method
	hashrateStr := "N/A"
	networkHashPS := float64(0)

	difficulty, err := rpc.DcrdClient.GetDifficulty(ctx)
	if err == nil && difficulty > 0 {
		// Calculate network hashrate from difficulty
		// Formula: hashrate = difficulty * 2^32 / target_block_time
		// For Decred, target block time is 5 minutes = 300 seconds
		networkHashPS = difficulty * math.Pow(2, 32) / 300
		hashrateStr = utils.FormatHashrate(networkHashPS)
	}

	return &types.NetworkInfo{
		PeerCount:     peerCount,
		Hashrate:      hashrateStr,
		NetworkHashPS: networkHashPS,
	}, nil
}

func FetchPeers() ([]types.Peer, error) {
	ctx := context.Background()
	peerInfo, err := rpc.DcrdClient.GetPeerInfo(ctx)
	if err != nil {
		return nil, err
	}

	peers := make([]types.Peer, 0, len(peerInfo))
	now := time.Now().Unix()

	for i, p := range peerInfo {
		// Convert pingtime from microseconds to milliseconds
		latency := fmt.Sprintf("%.0fms", float64(p.PingTime)/1000)

		// Calculate connection time
		connDuration := now - p.ConnTime
		connTime := utils.FormatDuration(connDuration)

		// Calculate total traffic in MB
		totalBytes := p.BytesSent + p.BytesRecv
		traffic := utils.FormatTraffic(totalBytes)

		// Extract version from subver string (e.g., "/dcrwire:1.0.0/dcrd:2.0.5/" -> "2.0.5")
		version := utils.ExtractDcrdVersion(p.SubVer)

		peers = append(peers, types.Peer{
			ID:         i + 1,
			Address:    p.Addr,
			Protocol:   "TCP",
			Latency:    latency,
			ConnTime:   connTime,
			Traffic:    traffic,
			Version:    version,
			IsSyncNode: p.SyncNode,
		})
	}

	return peers, nil
}

// formatDuration formats a duration in seconds to a human-readable string

func FetchSupplyInfo() (*types.SupplyInfo, error) {
	ctx := context.Background()

	// Get real circulating supply from dcrd - direct RPC method
	circulatingSupply := "N/A"
	stakedSupply := "N/A"
	stakedPercent := float64(0)
	treasuryBalance := "N/A"

	// Check if node is fully synced before calling TicketPoolValue
	chainInfo, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
	isSynced := err == nil && !chainInfo.InitialBlockDownload

	coinSupply, err := rpc.DcrdClient.GetCoinSupply(ctx)
	if err == nil && coinSupply > 0 {
		// Convert atoms to DCR and format with commas
		coinSupplyDCR := coinSupply.ToCoin()
		circulatingSupply = utils.FormatDCRAmount(coinSupplyDCR)

		// Calculate staked supply from ticket pool
		// Only call GetTicketPoolValue if node is fully synced to avoid nil pointer panic during initial sync
		if isSynced {
			ticketPoolValue, err := rpc.DcrdClient.GetTicketPoolValue(ctx)
			if err == nil && ticketPoolValue > 0 {
				lockedDCR := ticketPoolValue.ToCoin()
				stakedSupply = utils.FormatDCRAmount(lockedDCR)

				if coinSupplyDCR > 0 {
					stakedPercent = (lockedDCR / coinSupplyDCR) * 100
				}
			}
		}
	}

	// Get treasury balance - direct RPC method
	// Pass nil for hash (gets latest) and false for verbose
	treasuryBalanceResult, err := rpc.DcrdClient.GetTreasuryBalance(ctx, nil, false)
	if err == nil && treasuryBalanceResult.Balance > 0 {
		// Balance is in atoms (uint64), convert to DCR by dividing by 1e8
		treasuryBalanceDCR := float64(treasuryBalanceResult.Balance) / 1e8
		// Format with 2 decimal places and commas
		treasuryBalance = utils.FormatDCRAmountWithDecimals(treasuryBalanceDCR, 2)
	}

	return &types.SupplyInfo{
		CirculatingSupply: circulatingSupply,
		StakedSupply:      stakedSupply,
		StakedPercent:     stakedPercent,
		ExchangeRate:      "N/A", // Requires external API
		TreasurySize:      treasuryBalance,
		MixedPercent:      "N/A", // Requires mixer statistics
	}, nil
}

func FetchStakingInfo() (*types.StakingInfo, error) {
	ctx := context.Background()

	// Check if node is fully synced before calling TicketPoolValue
	chainInfo, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
	isSynced := err == nil && !chainInfo.InitialBlockDownload

	// Get stake difficulty (ticket price) - using RawRequest to get both current and next
	ticketPrice := float64(0)
	nextTicketPrice := float64(0)

	result, err := rpc.DcrdClient.RawRequest(ctx, "getstakedifficulty", []json.RawMessage{})
	if err != nil {
		return nil, fmt.Errorf("failed to get stake difficulty: %v", err)
	}

	var diffResult struct {
		Current float64 `json:"current"`
		Next    float64 `json:"next"`
	}
	if err := json.Unmarshal(result, &diffResult); err == nil {
		ticketPrice = diffResult.Current
		nextTicketPrice = diffResult.Next
	}

	// Get live tickets from pool - direct RPC method
	// LiveTickets returns []*chainhash.Hash directly
	liveTickets, err := rpc.DcrdClient.LiveTickets(ctx)
	poolSize := uint32(0)
	if err == nil && liveTickets != nil {
		// Count the actual number of live tickets
		poolSize = uint32(len(liveTickets))
	}

	// Get ticket pool value (total locked DCR) - direct RPC method
	// Returns dcrutil.Amount which needs to be converted to float64 DCR
	// Only call GetTicketPoolValue if node is fully synced to avoid nil pointer panic during initial sync
	lockedDCR := float64(0)
	if isSynced {
		poolValue, err := rpc.DcrdClient.GetTicketPoolValue(ctx)
		if err == nil {
			lockedDCR = poolValue.ToCoin()
		}
	}

	// Get total coin supply for participation rate calculation - direct RPC method
	// Returns dcrutil.Amount which needs to be converted to float64 DCR
	participationRate := float64(0)
	coinSupply, err := rpc.DcrdClient.GetCoinSupply(ctx)
	if err == nil && coinSupply > 0 {
		// Calculate participation rate as percentage of total supply
		coinSupplyDCR := coinSupply.ToCoin()
		participationRate = (lockedDCR / coinSupplyDCR) * 100
	}

	return &types.StakingInfo{
		TicketPrice:       ticketPrice,
		NextTicketPrice:   nextTicketPrice,
		PoolSize:          poolSize,
		LockedDCR:         lockedDCR,
		ParticipationRate: participationRate,
		AllMempoolTix:     0, // Would need livetickets/missedtickets commands
		Immature:          0, // Not available from dcrd alone
		Live:              poolSize,
		Voted:             0, // Would need block analysis
		Missed:            0, // Would need missedtickets command
		Revoked:           0, // Would need block analysis
	}, nil
}

func FetchMempoolInfo() (*types.MempoolInfo, error) {
	ctx := context.Background()

	// Use getmempoolinfo RPC to get actual mempool statistics
	result, err := rpc.DcrdClient.RawRequest(ctx, "getmempoolinfo", []json.RawMessage{})
	if err != nil {
		log.Printf("Warning: Failed to get mempool info: %v", err)
		// If mempool query fails (e.g., during sync), return empty mempool
		return &types.MempoolInfo{
			Size:           0,
			Bytes:          0,
			TxCount:        0,
			TotalFee:       0,
			AverageFeeRate: 0,
			Tickets:        0,
			Votes:          0,
			Revocations:    0,
			RegularTxs:     0,
		}, nil
	}

	// Parse the getmempoolinfo response
	type MempoolInfoResponse struct {
		Size  int    `json:"size"`  // Number of transactions
		Bytes uint64 `json:"bytes"` // Total size in bytes
	}

	var mempoolResp MempoolInfoResponse
	if err := json.Unmarshal(result, &mempoolResp); err != nil {
		log.Printf("Warning: Failed to unmarshal mempool info: %v", err)
		return &types.MempoolInfo{
			Size:           0,
			Bytes:          0,
			TxCount:        0,
			TotalFee:       0,
			AverageFeeRate: 0,
			Tickets:        0,
			Votes:          0,
			Revocations:    0,
			RegularTxs:     0,
		}, nil
	}

	// Analyze mempool transactions to categorize staking transactions
	tickets, votes, revocations, regular, coinjoins := analyzeMempoolTransactions(ctx)

	return &types.MempoolInfo{
		Size:           uint64(mempoolResp.Size),
		Bytes:          mempoolResp.Bytes,
		TxCount:        mempoolResp.Size,
		TotalFee:       0, // Not available from getmempoolinfo
		AverageFeeRate: 0, // Not available from getmempoolinfo
		Tickets:        tickets,
		Votes:          votes,
		Revocations:    revocations,
		RegularTxs:     regular,
		CoinJoinTxs:    coinjoins,
	}, nil
}

func analyzeMempoolTransactions(ctx context.Context) (tickets, votes, revocations, regular, coinjoins int) {
	// Get current stake difficulty (ticket price)
	stakeDiff := getStakeDifficulty(ctx)
	if stakeDiff <= 0 {
		log.Printf("Warning: Could not get stake difficulty, falling back to transaction counting")
		t, v, r, reg := analyzeMempoolTransactionsLegacy(ctx)
		return t, v, r, reg, 0
	}

	// Get all transaction hashes from mempool
	result, err := rpc.DcrdClient.RawRequest(ctx, "getrawmempool", []json.RawMessage{})
	if err != nil {
		log.Printf("Warning: Failed to get raw mempool: %v", err)
		return 0, 0, 0, 0, 0
	}

	var txHashes []string
	if err := json.Unmarshal(result, &txHashes); err != nil {
		log.Printf("Warning: Failed to unmarshal mempool hashes: %v", err)
		return 0, 0, 0, 0, 0
	}

	// Analyze each transaction (limit to reasonable number to avoid performance issues)
	maxToAnalyze := 100
	if len(txHashes) > maxToAnalyze {
		txHashes = txHashes[:maxToAnalyze]
	}

	// Sum up total stakesubmission output values to calculate actual ticket count
	var totalStakeValue float64

	for _, txHash := range txHashes {
		txType, stakeValue, isCoinJoin := getTransactionTypeAndStakeValueWithCoinJoin(ctx, txHash)
		switch txType {
		case "ticket":
			totalStakeValue += stakeValue
		case "vote":
			votes++
		case "revocation":
			revocations++
		default:
			if isCoinJoin {
				coinjoins++
			} else {
				regular++
			}
		}
	}

	// Calculate actual ticket count based on stake difficulty
	// Each ticket costs exactly stakeDiff DCR
	if totalStakeValue > 0 {
		tickets = int(math.Round(totalStakeValue / stakeDiff))
	}

	return tickets, votes, revocations, regular, coinjoins
}

// getStakeDifficulty fetches the current ticket price from dcrd
func getStakeDifficulty(ctx context.Context) float64 {
	result, err := rpc.DcrdClient.RawRequest(ctx, "getstakedifficulty", []json.RawMessage{})
	if err != nil {
		log.Printf("Warning: Failed to get stake difficulty: %v", err)
		return 0
	}

	var diffResult struct {
		Current float64 `json:"current"`
	}
	if err := json.Unmarshal(result, &diffResult); err != nil {
		log.Printf("Warning: Failed to unmarshal stake difficulty: %v", err)
		return 0
	}

	return diffResult.Current
}

// getTransactionTypeAndStakeValueWithCoinJoin returns the transaction type, stake value, and whether it's a CoinJoin
func getTransactionTypeAndStakeValueWithCoinJoin(ctx context.Context, txHash string) (string, float64, bool) {
	// Get raw transaction
	rawTxResult, err := rpc.DcrdClient.RawRequest(ctx, "getrawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, txHash)),
	})
	if err != nil {
		return "regular", 0, false
	}

	var rawTxHex string
	if err := json.Unmarshal(rawTxResult, &rawTxHex); err != nil {
		return "regular", 0, false
	}

	// Decode the transaction
	decodedResult, err := rpc.DcrdClient.RawRequest(ctx, "decoderawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, rawTxHex)),
	})
	if err != nil {
		return "regular", 0, false
	}

	// Parse decoded transaction
	type VinData struct {
		Stakebase string `json:"stakebase,omitempty"`
	}
	type ScriptPubKey struct {
		Asm  string `json:"asm"`
		Type string `json:"type"`
	}
	type VoutData struct {
		Value        float64      `json:"value"`
		ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
	}
	type DecodedTx struct {
		Vin  []VinData  `json:"vin"`
		Vout []VoutData `json:"vout"`
	}

	var decoded DecodedTx
	if err := json.Unmarshal(decodedResult, &decoded); err != nil {
		return "regular", 0, false
	}

	// Check for vote (SSGen) - has stakebase input
	if len(decoded.Vin) > 0 && decoded.Vin[0].Stakebase != "" {
		return "vote", 0, false
	}

	// Check outputs for stake transaction types and sum stakesubmission values
	hasStakeSubmission := false
	var totalStakeValue float64

	for _, vout := range decoded.Vout {
		scriptType := vout.ScriptPubKey.Type
		asm := vout.ScriptPubKey.Asm

		// Ticket purchase (SSTx) - sum up stakesubmission output values
		if strings.Contains(scriptType, "stakesubmission") {
			hasStakeSubmission = true
			totalStakeValue += vout.Value
		}

		// Vote (SSGen) - already caught by stakebase check above
		if scriptType == "stakegen-pubkeyhash" || scriptType == "stakegen" {
			return "vote", 0, false
		}
		if len(asm) >= 8 && strings.HasPrefix(asm, "OP_SSGEN") {
			return "vote", 0, false
		}

		// Revocation (SSRtx)
		if scriptType == "stakerevoke" {
			return "revocation", 0, false
		}
		if len(asm) >= 8 && strings.HasPrefix(asm, "OP_SSRTX") {
			return "revocation", 0, false
		}
	}

	// If we found stakesubmission outputs, it's a ticket transaction
	if hasStakeSubmission {
		return "ticket", totalStakeValue, false
	}

	// Check if it's a CoinJoin transaction
	// Heuristic: 3+ inputs, 3+ outputs, 3+ outputs with matching values
	isCoinJoin := false
	if len(decoded.Vin) >= 3 && len(decoded.Vout) >= 3 {
		// Count output values (rounded to avoid floating point issues)
		outputValues := make(map[int64]int)
		for _, vout := range decoded.Vout {
			rounded := int64(vout.Value * 1e8) // Convert to atoms
			outputValues[rounded]++
		}

		// If we have 3+ outputs with the same value, likely a CoinJoin
		for _, count := range outputValues {
			if count >= 3 {
				isCoinJoin = true
				break
			}
		}
	}

	// Regular transaction or CoinJoin
	return "regular", 0, isCoinJoin
}

// analyzeMempoolTransactionsLegacy is the old transaction-counting method (fallback)
func analyzeMempoolTransactionsLegacy(ctx context.Context) (tickets, votes, revocations, regular int) {
	result, err := rpc.DcrdClient.RawRequest(ctx, "getrawmempool", []json.RawMessage{})
	if err != nil {
		return 0, 0, 0, 0
	}

	var txHashes []string
	if err := json.Unmarshal(result, &txHashes); err != nil {
		return 0, 0, 0, 0
	}

	maxToAnalyze := 100
	if len(txHashes) > maxToAnalyze {
		txHashes = txHashes[:maxToAnalyze]
	}

	for _, txHash := range txHashes {
		txType := getTransactionType(ctx, txHash)
		switch txType {
		case "ticket":
			tickets++
		case "vote":
			votes++
		case "revocation":
			revocations++
		default:
			regular++
		}
	}

	return tickets, votes, revocations, regular
}

func getTransactionType(ctx context.Context, txHash string) string {
	// Get raw transaction
	rawTxResult, err := rpc.DcrdClient.RawRequest(ctx, "getrawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, txHash)),
	})
	if err != nil {
		return "regular"
	}

	var rawTxHex string
	if err := json.Unmarshal(rawTxResult, &rawTxHex); err != nil {
		return "regular"
	}

	// Decode the transaction
	decodedResult, err := rpc.DcrdClient.RawRequest(ctx, "decoderawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, rawTxHex)),
	})
	if err != nil {
		return "regular"
	}

	// Parse decoded transaction
	type VinData struct {
		Stakebase string `json:"stakebase,omitempty"`
	}
	type ScriptPubKey struct {
		Asm  string `json:"asm"`
		Type string `json:"type"`
	}
	type VoutData struct {
		ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
	}
	type DecodedTx struct {
		Vin  []VinData  `json:"vin"`
		Vout []VoutData `json:"vout"`
	}

	var decoded DecodedTx
	if err := json.Unmarshal(decodedResult, &decoded); err != nil {
		return "regular"
	}

	// Check for vote (SSGen) - has stakebase input
	if len(decoded.Vin) > 0 && decoded.Vin[0].Stakebase != "" {
		return "vote"
	}

	// Check outputs for stake transaction types
	hasStakeSubmission := false
	for _, vout := range decoded.Vout {
		scriptType := vout.ScriptPubKey.Type
		asm := vout.ScriptPubKey.Asm

		// Ticket purchase (SSTx) - check for stakesubmission types or OP_SSTX in ASM
		if strings.Contains(scriptType, "stakesubmission") || strings.Contains(scriptType, "sstx") {
			hasStakeSubmission = true
		}
		if len(asm) >= 7 && strings.HasPrefix(asm, "OP_SSTX") {
			hasStakeSubmission = true
		}

		// Vote (SSGen) - already caught by stakebase check above, but double-check
		if scriptType == "stakegen-pubkeyhash" || scriptType == "stakegen" {
			return "vote"
		}
		if len(asm) >= 8 && strings.HasPrefix(asm, "OP_SSGEN") {
			return "vote"
		}

		// Revocation (SSRtx)
		if scriptType == "stakerevoke" {
			return "revocation"
		}
		if len(asm) >= 8 && strings.HasPrefix(asm, "OP_SSRTX") {
			return "revocation"
		}
	}

	// If we found stakesubmission in any output, it's a ticket
	if hasStakeSubmission {
		return "ticket"
	}

	return "regular"
}
