// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
	"dcrpulse/internal/utils"

	"github.com/decred/dcrd/dcrutil/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

var (
	// Track previous sync values to calculate delta
	prevHeaders int64
	prevBlocks  int64
	syncMutex   sync.Mutex
)

// chainSnapshot fetches each value shared by several dashboard sections once
// per poll. Lazy, so a caller only pays for what it reads; the sync.Once makes
// it safe for the sections to share one concurrently.
type chainSnapshot struct {
	chainOnce sync.Once
	chainInfo *chainjson.GetBlockChainInfoResult
	chainErr  error

	peerOnce sync.Once
	peerInfo []chainjson.GetPeerInfoResult
	peerErr  error

	supplyOnce sync.Once
	coinSupply dcrutil.Amount
	supplyErr  error

	poolOnce  sync.Once
	poolValue dcrutil.Amount
	poolErr   error
}

func (s *chainSnapshot) blockChainInfo(ctx context.Context) (*chainjson.GetBlockChainInfoResult, error) {
	s.chainOnce.Do(func() { s.chainInfo, s.chainErr = rpc.DcrdClient.GetBlockChainInfo(ctx) })
	return s.chainInfo, s.chainErr
}

func (s *chainSnapshot) peers(ctx context.Context) ([]chainjson.GetPeerInfoResult, error) {
	s.peerOnce.Do(func() { s.peerInfo, s.peerErr = rpc.DcrdClient.GetPeerInfo(ctx) })
	return s.peerInfo, s.peerErr
}

func (s *chainSnapshot) supply(ctx context.Context) (dcrutil.Amount, error) {
	s.supplyOnce.Do(func() { s.coinSupply, s.supplyErr = rpc.DcrdClient.GetCoinSupply(ctx) })
	return s.coinSupply, s.supplyErr
}

func (s *chainSnapshot) ticketPoolValue(ctx context.Context) (dcrutil.Amount, error) {
	s.poolOnce.Do(func() { s.poolValue, s.poolErr = rpc.DcrdClient.GetTicketPoolValue(ctx) })
	return s.poolValue, s.poolErr
}

// FetchDashboardData assembles the dashboard payload. The sections are
// independent, so they run concurrently; one that fails is named in Degraded
// and left zeroed. Only a total failure returns an error.
func FetchDashboardData() (*types.DashboardData, error) {
	ctx := context.Background()
	snap := &chainSnapshot{}

	var (
		nodeStatus     *types.NodeStatus
		blockchainInfo *types.BlockchainInfo
		networkInfo    *types.NetworkInfo
		peers          []types.Peer
		supplyInfo     *types.SupplyInfo
		stakingInfo    *types.StakingInfo
		mempoolInfo    *types.MempoolInfo

		mu       sync.Mutex
		degraded []string
		firstErr error
	)

	// Sections are recorded in a fixed order below, so Degraded stays stable
	// across polls regardless of which goroutine finishes first.
	section := func(name string, fn func() error) func() {
		return func() {
			if err := fn(); err != nil {
				mu.Lock()
				degraded = append(degraded, name)
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				nodeLog.Warnf("Dashboard section %q failed: %v", name, err)
			}
		}
	}

	work := []struct {
		name string
		run  func()
	}{
		{"nodeStatus", section("nodeStatus", func() (err error) {
			nodeStatus, err = fetchNodeStatus(ctx, snap)
			return
		})},
		{"blockchainInfo", section("blockchainInfo", func() (err error) {
			blockchainInfo, err = fetchBlockchainInfo(ctx, snap)
			return
		})},
		{"networkInfo", section("networkInfo", func() (err error) {
			networkInfo, err = fetchNetworkInfo(ctx, snap)
			return
		})},
		{"peers", section("peers", func() (err error) {
			peers, err = fetchPeers(ctx, snap)
			return
		})},
		{"supplyInfo", section("supplyInfo", func() (err error) {
			supplyInfo, err = fetchSupplyInfo(ctx, snap)
			return
		})},
		{"stakingInfo", section("stakingInfo", func() (err error) {
			stakingInfo, err = fetchStakingInfo(ctx, snap)
			return
		})},
		{"mempoolInfo", section("mempoolInfo", func() (err error) {
			mempoolInfo, err = fetchMempoolInfo(ctx, snap)
			return
		})},
	}

	var wg sync.WaitGroup
	for _, w := range work {
		wg.Add(1)
		go func(run func()) {
			defer wg.Done()
			run()
		}(w.run)
	}
	wg.Wait()

	// Every section failed, so dcrd is down or unreachable.
	if len(degraded) == len(work) {
		return nil, firstErr
	}

	data := &types.DashboardData{
		Peers:      peers,
		LastUpdate: time.Now(),
	}
	if nodeStatus != nil {
		data.NodeStatus = *nodeStatus
	}
	if blockchainInfo != nil {
		data.BlockchainInfo = *blockchainInfo
	}
	if networkInfo != nil {
		data.NetworkInfo = *networkInfo
	}
	if supplyInfo != nil {
		data.SupplyInfo = *supplyInfo
	}
	if stakingInfo != nil {
		data.StakingInfo = *stakingInfo
	}
	if mempoolInfo != nil {
		data.MempoolInfo = *mempoolInfo
	}

	// Report in the fixed order above, not completion order.
	for _, w := range work {
		for _, d := range degraded {
			if d == w.name {
				data.Degraded = append(data.Degraded, w.name)
				break
			}
		}
	}

	return data, nil
}

func FetchNodeStatus() (*types.NodeStatus, error) {
	return fetchNodeStatus(context.Background(), &chainSnapshot{})
}

func fetchNodeStatus(ctx context.Context, snap *chainSnapshot) (*types.NodeStatus, error) {
	// Get version info using version command
	versionInfo, err := rpc.DcrdClient.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %v", err)
	}

	// Get blockchain info for accurate sync status
	chainInfo, err := snap.blockChainInfo(ctx)
	if err != nil {
		return nil, err
	}

	// Debug logging
	nodeLog.Debugf("Blockchain sync status - InitialBlockDownload: %v, Blocks: %d, Headers: %d, SyncHeight: %d, VerificationProgress: %f",
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
	return fetchBlockchainInfo(context.Background(), &chainSnapshot{})
}

func fetchBlockchainInfo(ctx context.Context, snap *chainSnapshot) (*types.BlockchainInfo, error) {
	info, err := snap.blockChainInfo(ctx)
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
			nodeLog.Warnf("Failed to get block hash for height %d: %v", currentHeight-i, err)
			continue
		}

		header, err := rpc.DcrdClient.GetBlockHeader(ctx, blockHash)
		if err != nil {
			nodeLog.Warnf("Failed to get block header for hash %s: %v", blockHash.String(), err)
			continue
		}

		recentBlocks = append(recentBlocks, types.RecentBlock{
			Height:    currentHeight - i,
			Hash:      blockHash.String(),
			Timestamp: header.Timestamp.Unix(),
		})
	}

	return &types.BlockchainInfo{
		BlockHeight: info.Blocks,
		BlockHash:   bestBlockHash.String(),
		// Difficulty is the packed nBits target; DifficultyRatio is the readable one.
		Difficulty:   info.DifficultyRatio,
		ChainSize:    0, // Would need to calculate from disk usage
		BlockTime:    blockTime,
		RecentBlocks: recentBlocks,
	}, nil
}

func fetchNetworkInfo(ctx context.Context, snap *chainSnapshot) (*types.NetworkInfo, error) {
	// Get peer count
	peerCount := 0
	peerInfo, err := snap.peers(ctx)
	if err == nil {
		peerCount = len(peerInfo)
	}

	// dcrd measures work over recent blocks; deriving this from difficulty
	// instead assumes blocks arrive exactly on target. Returns 0, not an error.
	hashrateStr := "N/A"
	networkHashPS := float64(0)

	hashPS, err := rpc.DcrdClient.GetNetworkHashPS(ctx)
	if err == nil && hashPS > 0 {
		networkHashPS = float64(hashPS)
		hashrateStr = utils.FormatHashrate(networkHashPS)
	}

	return &types.NetworkInfo{
		PeerCount:     peerCount,
		Hashrate:      hashrateStr,
		NetworkHashPS: networkHashPS,
	}, nil
}

func FetchPeers() ([]types.Peer, error) {
	return fetchPeers(context.Background(), &chainSnapshot{})
}

func fetchPeers(ctx context.Context, snap *chainSnapshot) ([]types.Peer, error) {
	peerInfo, err := snap.peers(ctx)
	if err != nil {
		return nil, err
	}

	peers := make([]types.Peer, 0, len(peerInfo))
	now := time.Now().Unix()

	// Resolve the Tor container IP(s) to flag INBOUND onion peers: Tor terminates
	// inbound connections at the local tor proxy, so dcrd reports the source as the
	// tor container's docker IP rather than a .onion. (Outbound onion peers carry a
	// .onion addr directly.)
	torIPs := map[string]bool{}
	if torHost := os.Getenv("TOR_PROXY_IP"); torHost != "" {
		if ips, lerr := net.LookupHost(torHost); lerr == nil {
			for _, ip := range ips {
				torIPs[ip] = true
			}
		}
	}

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

		// Outbound onion peers carry a .onion addr; inbound onion peers arrive via
		// the local tor proxy, so dcrd sees the tor container's IP instead.
		tor := strings.Contains(p.Addr, ".onion")
		if !tor && p.Inbound {
			if host, _, serr := net.SplitHostPort(p.Addr); serr == nil && torIPs[host] {
				tor = true
			}
		}

		peers = append(peers, types.Peer{
			ID:         i + 1,
			Address:    p.Addr,
			Protocol:   "TCP",
			Latency:    latency,
			ConnTime:   connTime,
			Traffic:    traffic,
			Version:    version,
			IsSyncNode: p.SyncNode,
			Inbound:    p.Inbound,
			Tor:        tor,
		})
	}

	return peers, nil
}

// formatDuration formats a duration in seconds to a human-readable string

func fetchSupplyInfo(ctx context.Context, snap *chainSnapshot) (*types.SupplyInfo, error) {
	// Get real circulating supply from dcrd - direct RPC method
	// Nil means dcrd could not supply the figure, which is distinct from zero.
	var circulatingSupply, stakedSupply, treasuryBalance *float64
	stakedPercent := float64(0)

	// Check if node is fully synced before calling TicketPoolValue
	chainInfo, err := snap.blockChainInfo(ctx)
	isSynced := err == nil && !chainInfo.InitialBlockDownload

	coinSupply, err := snap.supply(ctx)
	if err == nil && coinSupply > 0 {
		coinSupplyDCR := coinSupply.ToCoin()
		circulatingSupply = &coinSupplyDCR

		// Calculate staked supply from ticket pool
		// Only call GetTicketPoolValue if node is fully synced to avoid nil pointer panic during initial sync
		if isSynced {
			ticketPoolValue, err := snap.ticketPoolValue(ctx)
			if err == nil && ticketPoolValue > 0 {
				lockedDCR := ticketPoolValue.ToCoin()
				stakedSupply = &lockedDCR

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
		// Balance is atoms; dcrutil holds the authoritative AtomsPerCoin.
		treasuryDCR := dcrutil.Amount(treasuryBalanceResult.Balance).ToCoin()
		treasuryBalance = &treasuryDCR
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

func fetchStakingInfo(ctx context.Context, snap *chainSnapshot) (*types.StakingInfo, error) {
	// Check if node is fully synced before calling TicketPoolValue
	chainInfo, err := snap.blockChainInfo(ctx)
	isSynced := err == nil && !chainInfo.InitialBlockDownload

	// Get stake difficulty (ticket price) - using RawRequest to get current price
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
	}

	// Get estimated next ticket price based on current pool size
	estimateResult, err := rpc.DcrdClient.RawRequest(ctx, "estimatestakediff", []json.RawMessage{})
	if err == nil {
		var estimateData struct {
			Min      float64 `json:"min"`
			Max      float64 `json:"max"`
			Expected float64 `json:"expected"`
		}
		if err := json.Unmarshal(estimateResult, &estimateData); err == nil {
			nextTicketPrice = estimateData.Expected
		}
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
		poolValue, err := snap.ticketPoolValue(ctx)
		if err == nil {
			lockedDCR = poolValue.ToCoin()
		}
	}

	// Get total coin supply for participation rate calculation - direct RPC method
	// Returns dcrutil.Amount which needs to be converted to float64 DCR
	participationRate := float64(0)
	coinSupply, err := snap.supply(ctx)
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

func fetchMempoolInfo(ctx context.Context, _ *chainSnapshot) (*types.MempoolInfo, error) {
	// Use getmempoolinfo RPC to get actual mempool statistics
	result, err := rpc.DcrdClient.RawRequest(ctx, "getmempoolinfo", []json.RawMessage{})
	if err != nil {
		nodeLog.Warnf("Failed to get mempool info: %v", err)
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
		nodeLog.Warnf("Failed to unmarshal mempool info: %v", err)
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
		nodeLog.Warn("Could not get stake difficulty, falling back to transaction counting")
		t, v, r, reg := analyzeMempoolTransactionsLegacy(ctx)
		return t, v, r, reg, 0
	}

	// Get all transaction hashes from mempool
	result, err := rpc.DcrdClient.RawRequest(ctx, "getrawmempool", []json.RawMessage{})
	if err != nil {
		nodeLog.Warnf("Failed to get raw mempool: %v", err)
		return 0, 0, 0, 0, 0
	}

	var txHashes []string
	if err := json.Unmarshal(result, &txHashes); err != nil {
		nodeLog.Warnf("Failed to unmarshal mempool hashes: %v", err)
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
		nodeLog.Warnf("Failed to get stake difficulty: %v", err)
		return 0
	}

	var diffResult struct {
		Current float64 `json:"current"`
	}
	if err := json.Unmarshal(result, &diffResult); err != nil {
		nodeLog.Warnf("Failed to unmarshal stake difficulty: %v", err)
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
