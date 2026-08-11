// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/wire"
)

// FetchRecentBlocksPaginated gets blocks with pagination
func FetchRecentBlocksPaginated(ctx context.Context, page int, pageSize int) (*types.PaginatedBlocksResponse, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100 // Limit to 100 blocks per page
	}

	// Get current block count (total blocks)
	currentHeight, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get block count: %w", err)
	}

	totalBlocks := currentHeight + 1 // +1 because height is 0-indexed
	totalPages := int((totalBlocks + int64(pageSize) - 1) / int64(pageSize))

	// Calculate start and end heights for this page
	// Page 1 should show the most recent blocks
	startHeight := currentHeight - int64((page-1)*pageSize)
	endHeight := startHeight - int64(pageSize) + 1

	if endHeight < 0 {
		endHeight = 0
	}
	if startHeight < 0 {
		return &types.PaginatedBlocksResponse{
			Blocks:      []types.BlockSummary{},
			CurrentPage: page,
			PageSize:    pageSize,
			TotalBlocks: totalBlocks,
			TotalPages:  totalPages,
		}, nil
	}

	// Fetch blocks for this page
	blocks := make([]types.BlockSummary, 0, pageSize)
	for h := startHeight; h >= endHeight; h-- {
		block, err := FetchBlockSummaryByHeight(ctx, h)
		if err != nil {
			nodeLog.Warnf("Failed to fetch block %d: %v", h, err)
			continue
		}
		blocks = append(blocks, *block)
	}

	return &types.PaginatedBlocksResponse{
		Blocks:      blocks,
		CurrentPage: page,
		PageSize:    pageSize,
		TotalBlocks: totalBlocks,
		TotalPages:  totalPages,
	}, nil
}

// FetchBlockSummaryByHeight gets basic block info by height
func FetchBlockSummaryByHeight(ctx context.Context, height int64) (*types.BlockSummary, error) {
	// Get block hash
	hash, err := rpc.DcrdClient.GetBlockHash(ctx, height)
	if err != nil {
		return nil, fmt.Errorf("failed to get block hash: %w", err)
	}

	// The verbose block carries every field a summary needs, so it answers on
	// its own. verbosetx stays off: a count does not need the transactions
	// themselves, and a listing page asks for up to a hundred of these.
	block, err := rpc.DcrdClient.GetBlockVerbose(ctx, hash, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return &types.BlockSummary{
		Height:        block.Height,
		Hash:          block.Hash,
		PreviousHash:  block.PreviousHash,
		Timestamp:     time.Unix(block.Time, 0),
		Confirmations: block.Confirmations,
		TxCount:       len(block.Tx) + len(block.STx),
		Size:          int64(block.Size),
		Difficulty:    block.Difficulty,
	}, nil
}

// FetchBlockByHeight gets detailed block info by height
func FetchBlockByHeight(ctx context.Context, height int64) (*types.BlockDetail, error) {
	// Get block hash
	hash, err := rpc.DcrdClient.GetBlockHash(ctx, height)
	if err != nil {
		return nil, fmt.Errorf("failed to get block hash: %w", err)
	}

	return FetchBlockByHash(ctx, hash.String())
}

// FetchBlockByHash gets detailed block info by hash
func FetchBlockByHash(ctx context.Context, hash string) (*types.BlockDetail, error) {
	h, err := chainhash.NewHashFromStr(hash)
	if err != nil {
		return nil, fmt.Errorf("invalid block hash: %w", err)
	}

	// verboseTx=true returns every tx's full vin/vout inline (rawtx/rawstx),
	// so no per-transaction getrawtransaction call is needed. dcrd sends
	// either the txid arrays or the inline ones, never both.
	res, err := rpc.DcrdClient.GetBlockVerbose(ctx, h, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return blockDetailFromVerbose(res)
}

// blockDetailFromVerbose builds the block view from a getblock reply that was
// asked for its transactions inline.
func blockDetailFromVerbose(block *chainjson.GetBlockVerboseResult) (*types.BlockDetail, error) {
	txCount := len(block.RawTx) + len(block.RawSTx)
	transactions := make([]types.TransactionSummary, 0, txCount)
	for _, tx := range append(block.RawTx, block.RawSTx...) {
		summary := txSummaryFromRaw(tx, block.Height, block.Hash, block.Time, block.Confirmations)
		transactions = append(transactions, *summary)
	}

	return &types.BlockDetail{
		BlockSummary: types.BlockSummary{
			Height:        block.Height,
			Hash:          block.Hash,
			PreviousHash:  block.PreviousHash,
			Timestamp:     time.Unix(block.Time, 0),
			Confirmations: block.Confirmations,
			TxCount:       txCount,
			Size:          int64(block.Size),
			Difficulty:    block.Difficulty,
		},
		NextHash:     block.NextHash,
		MerkleRoot:   block.MerkleRoot,
		StakeRoot:    block.StakeRoot,
		Version:      block.Version,
		VoteBits:     block.VoteBits,
		StakeVersion: block.StakeVersion,
		Nonce:        block.Nonce,
		Transactions: transactions,
	}, nil
}

// FetchTransaction gets detailed transaction info
func FetchTransaction(ctx context.Context, txHash string) (*types.TransactionDetail, error) {
	hash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction hash: %w", err)
	}
	rawTx, err := rpc.DcrdClient.GetRawTransactionVerbose(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	// Convert inputs
	inputs := make([]types.TxInput, 0, len(rawTx.Vin))
	for _, vin := range rawTx.Vin {
		input := types.TxInput{
			PrevTxID:    vin.Txid,
			Vout:        vin.Vout,
			Tree:        vin.Tree,
			Sequence:    vin.Sequence,
			AmountIn:    vin.AmountIn,
			BlockHeight: int64(vin.BlockHeight),
			BlockIndex:  vin.BlockIndex,
			Coinbase:    vin.Coinbase,
			Stakebase:   vin.Stakebase,
		}
		if vin.ScriptSig != nil {
			input.ScriptSig = vin.ScriptSig.Asm
		}
		inputs = append(inputs, input)
	}

	// Convert outputs
	outputs := make([]types.TxOutput, 0, len(rawTx.Vout))
	totalValue := 0.0
	for _, vout := range rawTx.Vout {
		output := types.TxOutput{
			Value:   vout.Value,
			Index:   vout.N,
			Version: vout.Version,
			ScriptPubKey: types.Script{
				Asm:       vout.ScriptPubKey.Asm,
				Hex:       vout.ScriptPubKey.Hex,
				Type:      vout.ScriptPubKey.Type,
				ReqSigs:   int(vout.ScriptPubKey.ReqSigs),
				Addresses: vout.ScriptPubKey.Addresses,
			},
		}
		outputs = append(outputs, output)
		totalValue += vout.Value
	}

	// Calculate fee (total inputs - total outputs)
	totalIn := 0.0
	for _, vin := range rawTx.Vin {
		totalIn += vin.AmountIn
	}
	fee := totalIn - totalValue

	// Categorize transaction type
	txType := categorizeTransactionTyped(rawTx.Vin, rawTx.Vout)

	// dcrd's verbose reply carries no size field; the serialized hex is the size.
	size := len(rawTx.Hex) / 2

	// Extract treasury-specific information for tspend transactions
	var politeiaKey string
	recipientCount := 0
	var votingInfo *types.TSpendVotingInfo

	if txType == "tspend" {
		politeiaKey = tspendPiKey(rawTx.Hex)
		// Count recipients (exclude OP_RETURN output at index 0)
		for _, vout := range rawTx.Vout {
			if strings.Contains(vout.ScriptPubKey.Type, "treasurygen") {
				recipientCount++
			}
		}

		// Get voting information
		// Determine if in mempool (blockHeight == 0 or no block hash)
		inMempool := rawTx.BlockHeight == 0 || rawTx.BlockHash == ""
		if vInfo, err := GetTSpendVotingInfo(ctx, rawTx.Txid, rawTx.BlockHeight, rawTx.Expiry, inMempool); err == nil {
			votingInfo = vInfo
		} else {
			nodeLog.Warnf("Could not get voting info for tspend %s: %v", rawTx.Txid, err)
		}
	}

	var voteInfo *types.SSGenVoteInfo
	if txType == "vote" {
		var params *chaincfg.Params
		if p, err := chainParams(ctx); err == nil {
			params = p
		}
		voteInfo = ssgenVoteInfo(rawTx.Hex, params)
	}

	return &types.TransactionDetail{
		TransactionSummary: types.TransactionSummary{
			TxID:          rawTx.Txid,
			Type:          txType,
			BlockHeight:   rawTx.BlockHeight,
			BlockHash:     rawTx.BlockHash,
			Timestamp:     time.Unix(rawTx.Time, 0),
			Confirmations: rawTx.Confirmations,
			TotalValue:    totalValue,
			Fee:           fee,
			Size:          size,
		},
		Version:        rawTx.Version,
		LockTime:       rawTx.LockTime,
		Expiry:         rawTx.Expiry,
		Inputs:         inputs,
		Outputs:        outputs,
		RawHex:         rawTx.Hex,
		PoliteiaKey:    politeiaKey,
		RecipientCount: recipientCount,
		VotingInfo:     votingInfo,
		VoteInfo:       voteInfo,
	}, nil
}

// ssgenVoteInfo decodes a vote transaction's content from its serialized hex:
// the voted-on block, the validity bit, the agenda choices for the vote's
// version, and any treasury spend votes. params may be nil, which skips the
// agenda mapping. Returns nil for anything that is not a valid vote.
func ssgenVoteInfo(txHex string, params *chaincfg.Params) *types.SSGenVoteInfo {
	raw, err := hex.DecodeString(txHex)
	if err != nil {
		return nil
	}
	var mtx wire.MsgTx
	if err := mtx.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil
	}
	tvotes, err := stake.CheckSSGenVotes(&mtx)
	if err != nil {
		return nil
	}
	votedHash, votedHeight := stake.SSGenBlockVotedOn(&mtx)
	bits := stake.SSGenVoteBits(&mtx)
	info := &types.SSGenVoteInfo{
		VotedOnHash:   votedHash.String(),
		VotedOnHeight: votedHeight,
		BlockValid:    dcrutil.IsFlagSet16(bits, dcrutil.BlockValid),
		VoteVersion:   stake.SSGenVersion(&mtx),
		VoteBits:      bits,
	}
	if params != nil {
		for i := range params.Deployments[info.VoteVersion] {
			vote := &params.Deployments[info.VoteVersion][i].Vote
			choice := vote.VoteIndex(bits)
			if choice < 0 {
				// Bits matching no defined choice count as abstain,
				// same as the tally reporting path.
				for k := range vote.Choices {
					if vote.Choices[k].IsAbstain {
						choice = k
						break
					}
				}
			}
			if choice < 0 {
				continue
			}
			info.Agendas = append(info.Agendas, types.AgendaVoteChoice{
				AgendaID:          vote.Id,
				Description:       vote.Description,
				Choice:            vote.Choices[choice].Id,
				ChoiceDescription: vote.Choices[choice].Description,
			})
		}
	}
	for _, tv := range tvotes {
		choice := "invalid"
		switch tv.Vote {
		case stake.TreasuryVoteYes:
			choice = "yes"
		case stake.TreasuryVoteNo:
			choice = "no"
		}
		info.TreasuryVotes = append(info.TreasuryVotes, types.TreasuryVoteChoice{
			TSpend: tv.Hash.String(),
			Choice: choice,
		})
	}
	return info
}

// tspendPiKey extracts the Pi public key from a tspend's signature script.
func tspendPiKey(txHex string) string {
	raw, err := hex.DecodeString(txHex)
	if err != nil {
		return ""
	}
	var mtx wire.MsgTx
	if err := mtx.Deserialize(bytes.NewReader(raw)); err != nil {
		return ""
	}
	_, pubKey, err := stake.CheckTSpend(&mtx)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(pubKey)
}

// UniversalSearch auto-detects and searches for block/tx/address
func UniversalSearch(ctx context.Context, query string) (*types.SearchResult, error) {
	query = strings.TrimSpace(query)

	// Try to detect query type
	searchType := detectSearchType(query)

	switch searchType {
	case "block_height":
		height, _ := strconv.ParseInt(query, 10, 64)
		block, err := FetchBlockByHeight(ctx, height)
		if err != nil {
			return &types.SearchResult{
				Type:  "block",
				Found: false,
				Error: "Block not found",
			}, nil
		}
		return &types.SearchResult{
			Type:  "block",
			Found: true,
			Data:  block,
		}, nil

	case "tx_hash":
		// Try as transaction first
		tx, err := FetchTransaction(ctx, query)
		if err == nil {
			return &types.SearchResult{
				Type:  "transaction",
				Found: true,
				Data:  tx,
			}, nil
		}

		// If transaction not found, try as block hash
		block, err := FetchBlockByHash(ctx, query)
		if err == nil {
			return &types.SearchResult{
				Type:  "block",
				Found: true,
				Data:  block,
			}, nil
		}

		// Neither found
		return &types.SearchResult{
			Type:  "unknown",
			Found: false,
			Error: "Transaction or block not found",
		}, nil

	case "address":
		info, err := FetchAddressInfo(ctx, query)
		if err != nil {
			return &types.SearchResult{
				Type:  "address",
				Found: false,
				Error: "Failed to fetch address information",
			}, nil
		}
		return &types.SearchResult{
			Type:  "address",
			Found: true,
			Data:  info,
		}, nil

	default:
		return &types.SearchResult{
			Type:  "unknown",
			Found: false,
			Error: "Invalid search query. Enter a block height, transaction hash, or block hash.",
		}, nil
	}
}

// Helper functions

func detectSearchType(query string) string {
	// Block height (1-7 digits)
	if len(query) <= 7 {
		if _, err := strconv.ParseInt(query, 10, 64); err == nil {
			return "block_height"
		}
	}

	// Transaction hash or block hash (64 hex characters)
	if len(query) == 64 && isHex(query) {
		// Try as transaction first, then block
		return "tx_hash"
	}

	// Decred address (starts with D)
	if strings.HasPrefix(query, "D") && len(query) >= 26 && len(query) <= 35 {
		return "address"
	}

	return "unknown"
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// txSummaryFromRaw summarizes one inline transaction of a verbose getblock
// reply.
func txSummaryFromRaw(tx chainjson.TxRawResult, blockHeight int64, blockHash string, blockTime int64, confirmations int64) *types.TransactionSummary {
	// dcrd's verbose transactions carry no size field; the serialized hex is
	// the size.
	size := len(tx.Hex) / 2

	totalValue := 0.0
	for _, vout := range tx.Vout {
		totalValue += vout.Value
	}

	// A transaction without inputs has no fee to derive.
	fee := 0.0
	if len(tx.Vin) > 0 {
		totalIn := 0.0
		for _, vin := range tx.Vin {
			totalIn += vin.AmountIn
		}
		fee = totalIn - totalValue
	}

	return &types.TransactionSummary{
		TxID:          tx.Txid,
		Type:          categorizeTransactionTyped(tx.Vin, tx.Vout),
		BlockHeight:   blockHeight,
		BlockHash:     blockHash,
		Timestamp:     time.Unix(blockTime, 0),
		Confirmations: confirmations,
		TotalValue:    totalValue,
		Fee:           fee,
		Size:          size,
	}
}

// classifyScriptType maps one output's script type to a transaction kind, or
// "" when it says nothing. Callers apply it per output so the first output that
// matches decides, which is what both decode shapes did separately.
func classifyScriptType(scriptType string) string {
	switch {
	case strings.Contains(scriptType, "treasurygen"):
		return "tspend"
	case strings.Contains(scriptType, "treasurybase"), strings.Contains(scriptType, "treasuryadd"):
		return "treasurybase"
	case strings.Contains(scriptType, "stakesubmission"):
		return "ticket"
	case strings.Contains(scriptType, "stakegen"):
		return "vote"
	case strings.Contains(scriptType, "stakerevoke"):
		return "revocation"
	}
	return ""
}

// categorizeTransactionTyped works with the daemon's own wire types.
func categorizeTransactionTyped(vin []chainjson.Vin, vout []chainjson.Vout) string {
	// Check for stakebase (vote) or coinbase
	if len(vin) > 0 {
		if vin[0].Stakebase != "" {
			return "vote"
		}
		if vin[0].Coinbase != "" {
			return "coinbase"
		}
	}

	for _, v := range vout {
		if kind := classifyScriptType(v.ScriptPubKey.Type); kind != "" {
			return kind
		}
	}

	values := make([]float64, len(vout))
	for i, v := range vout {
		values[i] = v.Value
	}
	if looksLikeCoinJoin(len(vin), values) {
		return "coinjoin"
	}

	return "regular"
}

// jsonStr encodes s as a JSON string parameter for a dcrd/dcrwallet RawRequest,
// so a quote or backslash in user-supplied input cannot break the request.
func jsonStr(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// FetchAddressInfo gets limited information about an address
// Note: This uses only basic RPC methods available without --addrindex
func FetchAddressInfo(ctx context.Context, address string) (*types.AddressInfo, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}

	info := &types.AddressInfo{
		Address:  address,
		IsValid:  false,
		Exists:   false,
		Tickets:  []string{},
		HasIndex: false, // We don't have address indexing enabled
	}

	// 1. Validate address format
	validateResult, err := rpc.DcrdClient.RawRequest(ctx, "validateaddress", []json.RawMessage{
		jsonStr(address),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to validate address: %w", err)
	}

	var validateResp struct {
		IsValid bool   `json:"isvalid"`
		Address string `json:"address,omitempty"`
	}
	if err := json.Unmarshal(validateResult, &validateResp); err != nil {
		return nil, fmt.Errorf("failed to parse validate response: %w", err)
	}

	info.IsValid = validateResp.IsValid
	if !info.IsValid {
		return info, nil // Return early if invalid
	}

	// 2. Check if address exists on blockchain
	existsResult, err := rpc.DcrdClient.RawRequest(ctx, "existsaddress", []json.RawMessage{
		jsonStr(address),
	})
	if err != nil {
		nodeLog.Warnf("Failed to check address existence: %v", err)
	} else {
		var exists bool
		if err := json.Unmarshal(existsResult, &exists); err == nil {
			info.Exists = exists
		}
	}

	// 3. Get tickets owned by this address
	ticketsResult, err := rpc.DcrdClient.RawRequest(ctx, "ticketsforaddress", []json.RawMessage{
		jsonStr(address),
	})
	if err != nil {
		nodeLog.Warnf("Failed to get tickets for address: %v", err)
	} else {
		var ticketsResp struct {
			Tickets []string `json:"tickets"`
		}
		if err := json.Unmarshal(ticketsResult, &ticketsResp); err == nil {
			info.Tickets = ticketsResp.Tickets
			if info.Tickets == nil {
				info.Tickets = []string{} // Ensure it's an empty array, not null
			}
		}
	}

	return info, nil
}

// FetchMempoolTransactions retrieves all current mempool transactions
func FetchMempoolTransactions(ctx context.Context) (*types.MempoolTransactions, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}

	// Get raw mempool transaction hashes
	hashes, err := rpc.DcrdClient.GetRawMempool(ctx, chainjson.GRMAll)
	if err != nil {
		return nil, fmt.Errorf("failed to get mempool: %w", err)
	}

	// Limit to reasonable number for performance
	maxTxs := 500
	if len(hashes) > maxTxs {
		hashes = hashes[:maxTxs]
	}

	// Get mempool info for size
	mempoolInfoResult, err := rpc.DcrdClient.RawRequest(ctx, "getmempoolinfo", []json.RawMessage{})
	var mempoolSize uint64
	if err == nil {
		var mempoolInfo struct {
			Size  int    `json:"size"`
			Bytes uint64 `json:"bytes"`
		}
		if err := json.Unmarshal(mempoolInfoResult, &mempoolInfo); err == nil {
			mempoolSize = mempoolInfo.Bytes
		}
	}

	// Fetch each transaction
	transactions := make([]types.TransactionSummary, 0, len(hashes))
	for _, hash := range hashes {
		tx, err := FetchTransaction(ctx, hash.String())
		if err != nil {
			nodeLog.Warnf("Failed to fetch mempool transaction %s: %v", hash, err)
			continue
		}

		// Convert to TransactionSummary
		transactions = append(transactions, tx.TransactionSummary)
	}

	return &types.MempoolTransactions{
		Transactions: transactions,
		Count:        len(transactions),
		Size:         mempoolSize,
	}, nil
}
