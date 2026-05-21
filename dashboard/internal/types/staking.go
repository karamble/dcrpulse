// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// VSPInfo describes one Voting Service Provider entry returned by the
// public registry at api.decred.org or by a single VSP's /api/v3/vspinfo.
type VSPInfo struct {
	Host             string  `json:"host"`
	PubKey           string  `json:"pubkey"`
	Network          string  `json:"network"`
	APIVersions      []int   `json:"apiVersions,omitempty"`
	FeePercentage    float64 `json:"feePercentage"`
	VspdVersion      string  `json:"vspdVersion,omitempty"`
	BlockHeight      uint32  `json:"blockHeight,omitempty"`
	NetworkProportion float64 `json:"networkProportion,omitempty"`
	Voting           uint32  `json:"voting,omitempty"`
	Voted            uint32  `json:"voted,omitempty"`
	Expired          uint32  `json:"expired,omitempty"`
	Missed           uint32  `json:"missed,omitempty"`
	Outdated         bool    `json:"outdated,omitempty"`
}

// PurchaseTicketsRequest is the body posted to /api/wallet/staking/purchase.
type PurchaseTicketsRequest struct {
	Account       uint32 `json:"account"`
	NumTickets    uint32 `json:"numTickets"`
	VspHost       string `json:"vspHost"`
	VspPubkey     string `json:"vspPubkey"`
	ChangeAccount uint32 `json:"changeAccount"`
	Passphrase    string `json:"passphrase"`
}

// PurchaseTicketsResponse is returned from /api/wallet/staking/purchase.
type PurchaseTicketsResponse struct {
	TicketHashes []string `json:"ticketHashes"`
	SplitTxHash  string   `json:"splitTxHash,omitempty"`
}

// TicketRecord is one wallet ticket joined with its VSP fee state.
type TicketRecord struct {
	Hash          string  `json:"hash"`
	Status        string  `json:"status"`
	FeeStatus     string  `json:"feeStatus"`
	VSPHost       string  `json:"vspHost"`
	BlockHeight   int32   `json:"blockHeight"`
	BlockTime     int64   `json:"blockTime"`
	TicketPrice   float64 `json:"ticketPrice"`
	SpenderHash   string  `json:"spenderHash"`
	SpenderHeight int32   `json:"spenderHeight"`
	SpenderTime   int64   `json:"spenderTime"`
	Reward        float64 `json:"reward"`
}

// SyncFailedVSPTicketsRequest is the body posted to /api/wallet/staking/sync-failed-vsp-tickets.
type SyncFailedVSPTicketsRequest struct {
	VspHost       string `json:"vspHost"`
	VspPubkey     string `json:"vspPubkey"`
	Account       uint32 `json:"account"`
	ChangeAccount uint32 `json:"changeAccount"`
	Passphrase    string `json:"passphrase"`
}
