// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "time"

// VSPInfo describes one Voting Service Provider entry returned by the
// public registry at api.decred.org or by a single VSP's /api/v3/vspinfo.
type VSPInfo struct {
	Host              string  `json:"host"`
	PubKey            string  `json:"pubkey"`
	Network           string  `json:"network"`
	APIVersions       []int   `json:"apiVersions,omitempty"`
	FeePercentage     float64 `json:"feePercentage"`
	VspdVersion       string  `json:"vspdVersion,omitempty"`
	BlockHeight       uint32  `json:"blockHeight,omitempty"`
	NetworkProportion float64 `json:"networkProportion,omitempty"`
	Voting            uint32  `json:"voting,omitempty"`
	Voted             uint32  `json:"voted,omitempty"`
	Expired           uint32  `json:"expired,omitempty"`
	Missed            uint32  `json:"missed,omitempty"`
	Outdated          bool    `json:"outdated,omitempty"`
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

// PurchaseTicketsResponse is returned from /api/wallet/staking/purchase for a
// plain (non-privacy) purchase, which completes synchronously.
type PurchaseTicketsResponse struct {
	TicketHashes []string `json:"ticketHashes"`
	SplitTxHash  string   `json:"splitTxHash,omitempty"`
}

// PurchaseTicketsAsyncResponse is returned with HTTP 202 when a privacy/mixed
// purchase is dispatched to the background worker. The funds must be CSPP-mixed
// before the ticket is bought, which can take up to ~10 minutes, so the result
// arrives over the /wallet/staking/purchase/events WebSocket instead of the
// HTTP response.
type PurchaseTicketsAsyncResponse struct {
	Async bool `json:"async"`
}

// PurchaseEvent is a structured progress line emitted by the manual ticket
// purchase worker. Kind is "progress" for intermediate steps, "done" for a
// successful terminal event (with TicketHashes set), or "error" for a failure.
type PurchaseEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	Level        string    `json:"level"`
	Message      string    `json:"message"`
	Kind         string    `json:"kind"`
	TicketHashes []string  `json:"ticketHashes,omitempty"`
	SplitTxHash  string    `json:"splitTxHash,omitempty"`
}

// PurchaseStatus is what /api/wallet/staking/purchase/status returns. It lets a
// reloaded page tell whether a background (mixed) purchase is still running and
// read the most recent terminal result, mirroring the autobuyer status model.
type PurchaseStatus struct {
	InProgress   bool     `json:"inProgress"`
	LastError    string   `json:"lastError"`
	TicketHashes []string `json:"ticketHashes,omitempty"`
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
	// BlocksUntilMature is the blocks an IMMATURE ticket still needs before it
	// becomes live; 0 for any other status.
	BlocksUntilMature int32 `json:"blocksUntilMature"`
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
