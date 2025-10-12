// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "time"

// BlockSummary for lists and basic block info
type BlockSummary struct {
	Height        int64     `json:"height"`
	Hash          string    `json:"hash"`
	PreviousHash  string    `json:"previousHash"`
	Timestamp     time.Time `json:"timestamp"`
	Confirmations int64     `json:"confirmations"`
	TxCount       int       `json:"txCount"`
	Size          int64     `json:"size"`
	Difficulty    float64   `json:"difficulty"`
}

// BlockDetail for detailed block view
type BlockDetail struct {
	BlockSummary
	NextHash     string               `json:"nextHash,omitempty"`
	MerkleRoot   string               `json:"merkleRoot"`
	StakeRoot    string               `json:"stakeRoot"`
	Version      int32                `json:"version"`
	VoteBits     uint16               `json:"voteBits"`
	Transactions []TransactionSummary `json:"transactions"`
	StakeVersion uint32               `json:"stakeVersion"`
	Nonce        uint32               `json:"nonce"`
}

// TransactionSummary for lists
type TransactionSummary struct {
	TxID          string    `json:"txid"`
	Type          string    `json:"type"` // regular, ticket, vote, revocation
	BlockHeight   int64     `json:"blockHeight"`
	BlockHash     string    `json:"blockHash,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Confirmations int64     `json:"confirmations"`
	TotalValue    float64   `json:"totalValue"`
	Fee           float64   `json:"fee"`
	Size          int       `json:"size"`
}

// TransactionDetail for detail view
type TransactionDetail struct {
	TransactionSummary
	Version  int32      `json:"version"`
	LockTime uint32     `json:"lockTime"`
	Expiry   uint32     `json:"expiry"`
	Inputs   []TxInput  `json:"inputs"`
	Outputs  []TxOutput `json:"outputs"`
	RawHex   string     `json:"rawHex,omitempty"`
}

// TxInput represents a transaction input
type TxInput struct {
	PrevTxID    string  `json:"prevTxid,omitempty"`
	Vout        uint32  `json:"vout"`
	Tree        int8    `json:"tree"`
	Sequence    uint32  `json:"sequence"`
	AmountIn    float64 `json:"amountIn"`
	BlockHeight int64   `json:"blockHeight"`
	BlockIndex  uint32  `json:"blockIndex"`
	ScriptSig   string  `json:"scriptSig,omitempty"`
	Address     string  `json:"address,omitempty"` // if decodable
	Stakebase   string  `json:"stakebase,omitempty"`
	Coinbase    string  `json:"coinbase,omitempty"`
}

// TxOutput represents a transaction output
type TxOutput struct {
	Value        float64 `json:"value"`
	Index        uint32  `json:"index"`
	Version      uint16  `json:"version"`
	ScriptPubKey Script  `json:"scriptPubKey"`
	Spent        bool    `json:"spent,omitempty"`
	SpentBy      string  `json:"spentBy,omitempty"` // txid if known
}

// Script represents a script in transaction input/output
type Script struct {
	Asm       string   `json:"asm"`
	Hex       string   `json:"hex"`
	Type      string   `json:"type"`
	ReqSigs   int      `json:"reqSigs,omitempty"`
	Addresses []string `json:"addresses,omitempty"`
}

// SearchResult for universal search
type SearchResult struct {
	Type  string      `json:"type"` // block, transaction, address, unknown
	Found bool        `json:"found"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// AddressInfo for address detail view
type AddressInfo struct {
	Address  string   `json:"address"`
	IsValid  bool     `json:"isValid"`
	Exists   bool     `json:"exists"`
	Tickets  []string `json:"tickets"`
	HasIndex bool     `json:"hasIndex"` // whether address indexing is enabled
}

// PaginatedBlocksResponse for paginated block listings
type PaginatedBlocksResponse struct {
	Blocks      []BlockSummary `json:"blocks"`
	CurrentPage int            `json:"currentPage"`
	PageSize    int            `json:"pageSize"`
	TotalBlocks int64          `json:"totalBlocks"`
	TotalPages  int            `json:"totalPages"`
}
