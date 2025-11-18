// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
)

// FetchRecentBlocks gets the last N blocks
func FetchRecentBlocks(ctx context.Context, count int) ([]types.BlockSummary, error) {
	if count <= 0 {
		count = 10
	}
	if count > 50 {
		count = 50 // Limit to 50 blocks
	}

	// Get current block count
	height, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get block count: %w", err)
	}

	blocks := make([]types.BlockSummary, 0, count)
	startHeight := height - int64(count) + 1
	if startHeight < 0 {
		startHeight = 0
	}

	for h := height; h >= startHeight; h-- {
		block, err := FetchBlockSummaryByHeight(ctx, h)
		if err != nil {
			log.Printf("Warning: Failed to fetch block %d: %v", h, err)
			continue
		}
		blocks = append(blocks, *block)
	}

	return blocks, nil
}

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
			log.Printf("Warning: Failed to fetch block %d: %v", h, err)
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

	// Get block header
	result, err := rpc.DcrdClient.RawRequest(ctx, "getblockheader", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, hash.String())),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get block header: %w", err)
	}

	var header struct {
		Hash          string  `json:"hash"`
		Confirmations int64   `json:"confirmations"`
		Height        int64   `json:"height"`
		Time          int64   `json:"time"`
		PreviousHash  string  `json:"previousblockhash"`
		Difficulty    float64 `json:"difficulty"`
	}

	if err := json.Unmarshal(result, &header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block header: %w", err)
	}

	// Get full block to count transactions
	blockResult, err := rpc.DcrdClient.RawRequest(ctx, "getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, hash.String())),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	var block struct {
		Tx   []string `json:"tx"`
		STx  []string `json:"stx"`
		Size int64    `json:"size"`
	}

	if err := json.Unmarshal(blockResult, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	txCount := len(block.Tx) + len(block.STx)

	return &types.BlockSummary{
		Height:        header.Height,
		Hash:          header.Hash,
		PreviousHash:  header.PreviousHash,
		Timestamp:     time.Unix(header.Time, 0),
		Confirmations: header.Confirmations,
		TxCount:       txCount,
		Size:          block.Size,
		Difficulty:    header.Difficulty,
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
	// Get full block with verbose transactions
	// getblock takes: blockhash, verbose (bool), verbosetx (bool)
	result, err := rpc.DcrdClient.RawRequest(ctx, "getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, hash)),
		json.RawMessage(`true`), // verbose = true (returns JSON instead of hex)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	var rawBlock struct {
		Hash          string   `json:"hash"`
		Confirmations int64    `json:"confirmations"`
		Height        int64    `json:"height"`
		Version       int32    `json:"version"`
		MerkleRoot    string   `json:"merkleroot"`
		StakeRoot     string   `json:"stakeroot"`
		Time          int64    `json:"time"`
		Nonce         uint32   `json:"nonce"`
		VoteBits      uint16   `json:"votebits"`
		PreviousHash  string   `json:"previousblockhash"`
		NextHash      string   `json:"nextblockhash"`
		Difficulty    float64  `json:"difficulty"`
		StakeVersion  uint32   `json:"stakeversion"`
		Size          int64    `json:"size"`
		Tx            []string `json:"tx"`  // Transaction IDs
		STx           []string `json:"stx"` // Stake transaction IDs
	}

	if err := json.Unmarshal(result, &rawBlock); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	// Fetch full transaction details for each transaction ID
	transactions := make([]types.TransactionSummary, 0, len(rawBlock.Tx)+len(rawBlock.STx))

	// Process regular transactions
	for _, txID := range rawBlock.Tx {
		txResult, err := rpc.DcrdClient.RawRequest(ctx, "getrawtransaction", []json.RawMessage{
			json.RawMessage(fmt.Sprintf(`"%s"`, txID)),
			json.RawMessage(`1`), // verbose = 1 for decoded JSON
		})
		if err != nil {
			log.Printf("Warning: failed to fetch transaction %s: %v", txID, err)
			continue
		}

		var txData map[string]interface{}
		if err := json.Unmarshal(txResult, &txData); err != nil {
			log.Printf("Warning: failed to unmarshal transaction %s: %v", txID, err)
			continue
		}

		txSummary := extractTransactionSummary(txData, rawBlock.Height, rawBlock.Hash, rawBlock.Time, rawBlock.Confirmations)
		if txSummary != nil {
			transactions = append(transactions, *txSummary)
		}
	}

	// Process stake transactions
	for _, txID := range rawBlock.STx {
		txResult, err := rpc.DcrdClient.RawRequest(ctx, "getrawtransaction", []json.RawMessage{
			json.RawMessage(fmt.Sprintf(`"%s"`, txID)),
			json.RawMessage(`1`), // verbose = 1 for decoded JSON
		})
		if err != nil {
			log.Printf("Warning: failed to fetch stake transaction %s: %v", txID, err)
			continue
		}

		var txData map[string]interface{}
		if err := json.Unmarshal(txResult, &txData); err != nil {
			log.Printf("Warning: failed to unmarshal stake transaction %s: %v", txID, err)
			continue
		}

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
			TxCount:       len(rawBlock.Tx) + len(rawBlock.STx),
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
	// Get raw transaction
	result, err := rpc.DcrdClient.RawRequest(ctx, "getrawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, txHash)),
		json.RawMessage(`1`), // verbose
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	var rawTx struct {
		Hex           string `json:"hex"`
		Txid          string `json:"txid"`
		Version       int32  `json:"version"`
		LockTime      uint32 `json:"locktime"`
		Expiry        uint32 `json:"expiry"`
		BlockHash     string `json:"blockhash"`
		BlockHeight   int64  `json:"blockheight"`
		BlockIndex    uint32 `json:"blockindex"`
		Confirmations int64  `json:"confirmations"`
		Time          int64  `json:"time"`
		Size          int    `json:"size"`
		Vin           []struct {
			Coinbase    string  `json:"coinbase,omitempty"`
			Stakebase   string  `json:"stakebase,omitempty"`
			Txid        string  `json:"txid,omitempty"`
			Vout        uint32  `json:"vout"`
			Tree        int8    `json:"tree"`
			Sequence    uint32  `json:"sequence"`
			AmountIn    float64 `json:"amountin"`
			BlockHeight int64   `json:"blockheight"`
			BlockIndex  uint32  `json:"blockindex"`
			ScriptSig   struct {
				Asm string `json:"asm"`
				Hex string `json:"hex"`
			} `json:"scriptSig,omitempty"`
		} `json:"vin"`
		Vout []struct {
			Value        float64 `json:"value"`
			N            uint32  `json:"n"`
			Version      uint16  `json:"version"`
			ScriptPubKey struct {
				Asm       string   `json:"asm"`
				Hex       string   `json:"hex"`
				Type      string   `json:"type"`
				ReqSigs   int      `json:"reqSigs,omitempty"`
				Addresses []string `json:"addresses,omitempty"`
			} `json:"scriptPubKey"`
		} `json:"vout"`
	}

	if err := json.Unmarshal(result, &rawTx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %w", err)
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
			BlockHeight: vin.BlockHeight,
			BlockIndex:  vin.BlockIndex,
			Coinbase:    vin.Coinbase,
			Stakebase:   vin.Stakebase,
		}
		if vin.ScriptSig.Asm != "" {
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
				ReqSigs:   vout.ScriptPubKey.ReqSigs,
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

	// Calculate size from hex if not available
	size := rawTx.Size
	if size == 0 && rawTx.Hex != "" {
		size = len(rawTx.Hex) / 2
	}

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
			log.Printf("Warning: Could not get voting info for tspend %s: %v", rawTx.Txid, err)
		}
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
	}, nil
}

// extractPoliteiaKey extracts the politeia key from a tspend transaction's OP_RETURN output
func extractPoliteiaKey(vout []struct {
	Value        float64 `json:"value"`
	N            uint32  `json:"n"`
	Version      uint16  `json:"version"`
	ScriptPubKey struct {
		Asm       string   `json:"asm"`
		Hex       string   `json:"hex"`
		Type      string   `json:"type"`
		ReqSigs   int      `json:"reqSigs,omitempty"`
		Addresses []string `json:"addresses,omitempty"`
	} `json:"scriptPubKey"`
}) string {
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

	case "block_hash":
		block, err := FetchBlockByHash(ctx, query)
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
	if len(query) == 64 {
		// Check if it's valid hex
		if _, err := strconv.ParseUint(query, 16, 64); err == nil || isHex(query) {
			// Try as transaction first, then block
			return "tx_hash"
		}
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
	size, _ := txMap["size"].(float64)

	// If size is not available, calculate from hex
	if size == 0 {
		if hex, ok := txMap["hex"].(string); ok && hex != "" {
			size = float64(len(hex) / 2)
		}
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

func categorizeTransaction(vin []interface{}, vout []interface{}) string {
	// Check for stakebase (vote)
	if len(vin) > 0 {
		if vinMap, ok := vin[0].(map[string]interface{}); ok {
			if _, hasStakebase := vinMap["stakebase"]; hasStakebase {
				return "vote"
			}
			if _, hasCoinbase := vinMap["coinbase"]; hasCoinbase {
				return "coinbase"
			}
		}
	}

	// Check outputs for treasury and stake transaction types
	for _, v := range vout {
		if voutMap, ok := v.(map[string]interface{}); ok {
			if scriptPubKey, ok := voutMap["scriptPubKey"].(map[string]interface{}); ok {
				if scriptType, ok := scriptPubKey["type"].(string); ok {
					// Treasury transactions
					if strings.Contains(scriptType, "treasurygen") {
						return "tspend"
					}
					if strings.Contains(scriptType, "treasurybase") || strings.Contains(scriptType, "treasuryadd") {
						return "treasurybase"
					}

					// Stake transactions
					if strings.Contains(scriptType, "stakesubmission") {
						return "ticket"
					}
					if strings.Contains(scriptType, "stakegen") {
						return "vote"
					}
					if strings.Contains(scriptType, "stakerevoke") {
						return "revocation"
					}
				}
			}
		}
	}

	// Check if it's a CoinJoin transaction
	// Heuristic: 3+ inputs, 3+ outputs, 3+ outputs with matching values
	if len(vin) >= 3 && len(vout) >= 3 {
		// Count output values (rounded to avoid floating point issues)
		outputValues := make(map[int64]int)
		for _, v := range vout {
			if voutMap, ok := v.(map[string]interface{}); ok {
				if value, ok := voutMap["value"].(float64); ok {
					rounded := int64(value * 1e8) // Convert to atoms
					outputValues[rounded]++
				}
			}
		}

		// If we have 3+ outputs with the same value, likely a CoinJoin
		for _, count := range outputValues {
			if count >= 3 {
				return "coinjoin"
			}
		}
	}

	return "regular"
}

func categorizeTransactionFromMaps(vin []interface{}, vout []interface{}) string {
	return categorizeTransaction(vin, vout)
}

// categorizeTransactionTyped works with typed structs from RPC response
func categorizeTransactionTyped(vin []struct {
	Coinbase    string  `json:"coinbase,omitempty"`
	Stakebase   string  `json:"stakebase,omitempty"`
	Txid        string  `json:"txid,omitempty"`
	Vout        uint32  `json:"vout"`
	Tree        int8    `json:"tree"`
	Sequence    uint32  `json:"sequence"`
	AmountIn    float64 `json:"amountin"`
	BlockHeight int64   `json:"blockheight"`
	BlockIndex  uint32  `json:"blockindex"`
	ScriptSig   struct {
		Asm string `json:"asm"`
		Hex string `json:"hex"`
	} `json:"scriptSig,omitempty"`
}, vout []struct {
	Value        float64 `json:"value"`
	N            uint32  `json:"n"`
	Version      uint16  `json:"version"`
	ScriptPubKey struct {
		Asm       string   `json:"asm"`
		Hex       string   `json:"hex"`
		Type      string   `json:"type"`
		ReqSigs   int      `json:"reqSigs,omitempty"`
		Addresses []string `json:"addresses,omitempty"`
	} `json:"scriptPubKey"`
}) string {
	// Check for stakebase (vote) or coinbase
	if len(vin) > 0 {
		if vin[0].Stakebase != "" {
			return "vote"
		}
		if vin[0].Coinbase != "" {
			return "coinbase"
		}
	}

	// Check outputs for treasury and stake transaction types
	for _, v := range vout {
		scriptType := v.ScriptPubKey.Type

		// Treasury transactions
		if strings.Contains(scriptType, "treasurygen") {
			return "tspend"
		}
		if strings.Contains(scriptType, "treasurybase") || strings.Contains(scriptType, "treasuryadd") {
			return "treasurybase"
		}

		// Stake transactions
		if strings.Contains(scriptType, "stakesubmission") {
			return "ticket"
		}
		if strings.Contains(scriptType, "stakegen") {
			return "vote"
		}
		if strings.Contains(scriptType, "stakerevoke") {
			return "revocation"
		}
	}

	// Check if it's a CoinJoin transaction
	// Heuristic: 3+ inputs, 3+ outputs, 3+ outputs with matching values
	if len(vin) >= 3 && len(vout) >= 3 {
		// Count output values (rounded to avoid floating point issues)
		outputValues := make(map[int64]int)
		for _, v := range vout {
			rounded := int64(v.Value * 1e8) // Convert to atoms
			outputValues[rounded]++
		}

		// If we have 3+ outputs with the same value, likely a CoinJoin
		for _, count := range outputValues {
			if count >= 3 {
				return "coinjoin"
			}
		}
	}

	return "regular"
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
		json.RawMessage(fmt.Sprintf(`"%s"`, address)),
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
		json.RawMessage(fmt.Sprintf(`"%s"`, address)),
	})
	if err != nil {
		log.Printf("Warning: Failed to check address existence: %v", err)
	} else {
		var exists bool
		if err := json.Unmarshal(existsResult, &exists); err == nil {
			info.Exists = exists
		}
	}

	// 3. Get tickets owned by this address
	ticketsResult, err := rpc.DcrdClient.RawRequest(ctx, "ticketsforaddress", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, address)),
	})
	if err != nil {
		log.Printf("Warning: Failed to get tickets for address: %v", err)
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
			log.Printf("Warning: Failed to fetch mempool transaction %s: %v", txHash, err)
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
