// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "time"

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

// ListVSPsResponse is the envelope returned by GET /api/wallet/staking/vsps.
// VSPs is empty when the registry toggle is off or the upstream fetch
// failed (in which case RegistryError carries a hint). UsedVSPs is the
// per-wallet history from used_vsps and is always provided when present.
type ListVSPsResponse struct {
	VSPs            []VSPInfo `json:"vsps"`
	UsedVSPs        []VSPInfo `json:"usedVSPs"`
	RegistryEnabled bool      `json:"registryEnabled"`
	RegistryError   string    `json:"registryError,omitempty"`
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

// VSPFeeStatusCounts holds wallet-wide ticket counts per VSP fee-processing
// status, keyed to the same short names as TicketRecord.FeeStatus.
type VSPFeeStatusCounts struct {
	Unpaid    int `json:"unpaid"`
	Paid      int `json:"paid"`
	Errored   int `json:"errored"`
	Confirmed int `json:"confirmed"`
}

// SyncFailedVSPTicketsResponse summarizes a sync run. dcrwallet's
// SyncVSPFailedTickets RPC returns no data, so the summary is derived from
// before/after GetVSPTicketsByFeeStatus snapshots.
type SyncFailedVSPTicketsResponse struct {
	VspHost string             `json:"vspHost"`
	Before  VSPFeeStatusCounts `json:"before"`
	After   VSPFeeStatusCounts `json:"after"`
}

// AutobuyerSettings is the persistable configuration for the ticket autobuyer.
// No secret material is stored here; the passphrase is supplied per-start only.
type AutobuyerSettings struct {
	Account           uint32  `json:"account"`
	VspHost           string  `json:"vspHost"`
	VspPubkey         string  `json:"vspPubkey"`
	BalanceToMaintain float64 `json:"balanceToMaintain"`
}

// AutobuyerEvent is a structured log line emitted by the autobuyer supervisor.
type AutobuyerEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// AutobuyerStatus is what /api/wallet/staking/autobuyer/status returns.
type AutobuyerStatus struct {
	Running   bool               `json:"running"`
	LastError string             `json:"lastError"`
	Settings  *AutobuyerSettings `json:"settings"`
}

// StartAutobuyerRequest is the body posted to /api/wallet/staking/autobuyer/start.
type StartAutobuyerRequest struct {
	AutobuyerSettings
	Passphrase string `json:"passphrase"`
}
