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
