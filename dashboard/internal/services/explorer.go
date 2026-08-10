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
	blockResult, err := rpc.DcrdClient.RawRequest(ctx, "getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, hash.String())),
		json.RawMessage(`true`),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	var block struct {
		Hash          string   `json:"hash"`
		Confirmations int64    `json:"confirmations"`
		Height        int64    `json:"height"`
		Time          int64    `json:"time"`
		PreviousHash  string   `json:"previousblockhash"`
		Difficulty    float64  `json:"difficulty"`
		Size          int64    `json:"size"`
		Tx            []string `json:"tx"`
		STx           []string `json:"stx"`
	}

	if err := json.Unmarshal(blockResult, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	return &types.BlockSummary{
		Height:        block.Height,
		Hash:          block.Hash,
		PreviousHash:  block.PreviousHash,
		Timestamp:     time.Unix(block.Time, 0),
		Confirmations: block.Confirmations,
		TxCount:       len(block.Tx) + len(block.STx),
		Size:          block.Size,
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
	// verbose=true + verbosetx=true returns every tx's full vin/vout inline
	// (rawtx/rawstx), so no per-transaction getrawtransaction call is needed.
	// dcrd sends either the txid arrays or the inline ones, never both.
	result, err := rpc.DcrdClient.RawRequest(ctx, "getblock", []json.RawMessage{
		jsonStr(hash),
		json.RawMessage(`true`),
		json.RawMessage(`true`),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return blockDetailFromVerbose(result)
}

// blockDetailFromVerbose builds the block view from a getblock reply that was
// asked for its transactions inline.
func blockDetailFromVerbose(result json.RawMessage) (*types.BlockDetail, error) {
	var rawBlock struct {
		Hash          string                   `json:"hash"`
		Confirmations int64                    `json:"confirmations"`
		Height        int64                    `json:"height"`
		Version       int32                    `json:"version"`
		MerkleRoot    string                   `json:"merkleroot"`
		StakeRoot     string                   `json:"stakeroot"`
		Time          int64                    `json:"time"`
		Nonce         uint32                   `json:"nonce"`
		VoteBits      uint16                   `json:"votebits"`
		PreviousHash  string                   `json:"previousblockhash"`
		NextHash      string                   `json:"nextblockhash"`
		Difficulty    float64                  `json:"difficulty"`
		StakeVersion  uint32                   `json:"stakeversion"`
		Size          int64                    `json:"size"`
		RawTx         []map[string]interface{} `json:"rawtx"`
		RawSTx        []map[string]interface{} `json:"rawstx"`
	}

	if err := json.Unmarshal(result, &rawBlock); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	txCount := len(rawBlock.RawTx) + len(rawBlock.RawSTx)
	transactions := make([]types.TransactionSummary, 0, txCount)
	for _, txData := range append(rawBlock.RawTx, rawBlock.RawSTx...) {
		txSummary := extractTransactionSummary(txData, rawBlock.Height, rawBlock.Hash, rawBlock.Time, rawBlock.Confirmations)
		if txSummary != nil {
			transactions = append(transactions, *txSummary)
		}
	}

	return &types.BlockDetail{
		BlockSummary: types.BlockSummary{
			Height:        rawBlock.Height,
			Hash:          rawBlock.Hash,
			PreviousHash:  rawBlock.PreviousHash,
			Timestamp:     time.Unix(rawBlock.Time, 0),
			Confirmations: rawBlock.Confirmations,
			TxCount:       txCount,
			Size:          rawBlock.Size,
			Difficulty:    rawBlock.Difficulty,
		},
		NextHash:     rawBlock.NextHash,
		MerkleRoot:   rawBlock.MerkleRoot,
		StakeRoot:    rawBlock.StakeRoot,
		Version:      rawBlock.Version,
		VoteBits:     rawBlock.VoteBits,
		StakeVersion: rawBlock.StakeVersion,
		Nonce:        rawBlock.Nonce,
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
		politeiaKey = extractPoliteiaKey(rawTx.Vout)
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

// extractPoliteiaKey extracts the politeia key from a tspend transaction's OP_RETURN output
func extractPoliteiaKey(vout []chainjson.Vout) string {
	// TSpend transactions have OP_RETURN as the first output (index 0)
	if len(vout) == 0 {
		return ""
	}

	firstOutput := vout[0]
	if firstOutput.ScriptPubKey.Type != "nulldata" {
		return ""
	}

	// Parse hex: format is "6a20" + 32-byte politeia key
	// 6a = OP_RETURN, 20 = push 32 bytes (0x20 = 32 decimal)
	hex := firstOutput.ScriptPubKey.Hex
	if len(hex) < 68 { // 4 (6a20) + 64 (32 bytes hex) = 68 minimum
		return ""
	}

	// Check for OP_RETURN prefix
	if !strings.HasPrefix(hex, "6a20") {
		return ""
	}

	// Extract the 32-byte politeia key (64 hex characters after "6a20")
	politeiaKey := hex[4:68]
	return politeiaKey
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

func extractTransactionSummary(txData interface{}, blockHeight int64, blockHash string, blockTime int64, confirmations int64) *types.TransactionSummary {
	// If txData is just a string (tx hash), return minimal info
	if txHash, ok := txData.(string); ok {
		return &types.TransactionSummary{
			TxID:          txHash,
			Type:          "unknown",
			BlockHeight:   blockHeight,
			BlockHash:     blockHash,
			Timestamp:     time.Unix(blockTime, 0),
			Confirmations: confirmations,
		}
	}

	// If txData is a full transaction object, extract details
	txMap, ok := txData.(map[string]interface{})
	if !ok {
		return nil
	}

	txid, _ := txMap["txid"].(string)

	// dcrd's verbose transactions carry no size field; the serialized hex is
	// the size.
	var size float64
	if hex, ok := txMap["hex"].(string); ok {
		size = float64(len(hex) / 2)
	}

	// Get vout to calculate total value
	totalValue := 0.0
	if vout, ok := txMap["vout"].([]interface{}); ok {
		for _, v := range vout {
			if voutMap, ok := v.(map[string]interface{}); ok {
				if value, ok := voutMap["value"].(float64); ok {
					totalValue += value
				}
			}
		}
	}

	// Calculate fee
	fee := 0.0
	totalIn := 0.0
	if vin, ok := txMap["vin"].([]interface{}); ok {
		for _, v := range vin {
			if vinMap, ok := v.(map[string]interface{}); ok {
				if amountIn, ok := vinMap["amountin"].(float64); ok {
					totalIn += amountIn
				}
			}
		}
		fee = totalIn - totalValue
	}

	// Determine transaction type
	txType := "regular"
	if vin, ok := txMap["vin"].([]interface{}); ok {
		if vout, ok := txMap["vout"].([]interface{}); ok {
			txType = categorizeTransactionFromMaps(vin, vout)
		}
	}

	return &types.TransactionSummary{
		TxID:          txid,
		Type:          txType,
		BlockHeight:   blockHeight,
		BlockHash:     blockHash,
		Timestamp:     time.Unix(blockTime, 0),
		Confirmations: confirmations,
		TotalValue:    totalValue,
		Fee:           fee,
		Size:          int(size),
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

func categorizeTransaction(vin []interface{}, vout []interface{}) string {
	// dcrd emits a different key set per input class, so presence of the key
	// is the test, not its value.
	if len(vin) > 0 {
		if vinMap, ok := vin[0].(map[string]interface{}); ok {
			if _, has := vinMap["stakebase"]; has {
				return "vote"
			}
			if _, has := vinMap["coinbase"]; has {
				return "coinbase"
			}
		}
	}

	values := make([]float64, 0, len(vout))
	for _, v := range vout {
		voutMap, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if scriptPubKey, ok := voutMap["scriptPubKey"].(map[string]interface{}); ok {
			if scriptType, ok := scriptPubKey["type"].(string); ok {
				if kind := classifyScriptType(scriptType); kind != "" {
					return kind
				}
			}
		}
		if value, ok := voutMap["value"].(float64); ok {
			values = append(values, value)
		}
	}

	if looksLikeCoinJoin(len(vin), values) {
		return "coinjoin"
	}

	return "regular"
}

func categorizeTransactionFromMaps(vin []interface{}, vout []interface{}) string {
	return categorizeTransaction(vin, vout)
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
	result, err := rpc.DcrdClient.RawRequest(ctx, "getrawmempool", []json.RawMessage{})
	if err != nil {
		return nil, fmt.Errorf("failed to get mempool: %w", err)
	}

	var txHashes []string
	if err := json.Unmarshal(result, &txHashes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mempool hashes: %w", err)
	}

	// Limit to reasonable number for performance
	maxTxs := 500
	if len(txHashes) > maxTxs {
		txHashes = txHashes[:maxTxs]
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
	transactions := make([]types.TransactionSummary, 0, len(txHashes))
	for _, txHash := range txHashes {
		tx, err := FetchTransaction(ctx, txHash)
		if err != nil {
			nodeLog.Warnf("Failed to fetch mempool transaction %s: %v", txHash, err)
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
