// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// MsigInvitee names one Bison Relay contact to invite into a shared
// wallet round.
type MsigInvitee struct {
	UID  string `json:"uid"`
	Nick string `json:"nick"`
}

// MsigInviteRequest starts a shared wallet round. The invitees plus this
// wallet form the n keys of an m-of-n scheme.
type MsigInviteRequest struct {
	Label    string        `json:"label"`
	M        int           `json:"m"`
	Account  uint32        `json:"account"`
	Invitees []MsigInvitee `json:"invitees"`
}

// MsigActionRequest targets one shared wallet round by tempId or address.
type MsigActionRequest struct {
	ID      string `json:"id"`
	Account uint32 `json:"account,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// MsigRecipient is one payment output of a proposal.
type MsigRecipient struct {
	Address     string `json:"address"`
	AmountAtoms int64  `json:"amountAtoms"`
}

// MsigProposeRequest builds, self-signs and dispatches a shared wallet
// payment. QueueUIDs lists the cosigners to ask, in order. SendAll sweeps
// the wallet's whole spendable balance to a single recipient, whose
// amount is then computed server-side.
type MsigProposeRequest struct {
	WalletID   string          `json:"walletId"`
	Recipients []MsigRecipient `json:"recipients"`
	SendAll    bool            `json:"sendAll,omitempty"`
	QueueUIDs  []string        `json:"queueUids"`
	Note       string          `json:"note,omitempty"`
	HopTTLSecs int64           `json:"hopTtlSecs,omitempty"`
	Account    uint32          `json:"account"`
	Passphrase string          `json:"passphrase"`
}

// MsigProposalActionRequest targets one proposal of a shared wallet.
type MsigProposalActionRequest struct {
	WalletID   string `json:"walletId"`
	TxID       string `json:"txid"`
	Account    uint32 `json:"account,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}
