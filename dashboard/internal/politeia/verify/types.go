// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package verify

// The wire types below carry the signed fields of politeiawww's records,
// comments, ticketvote and pi v1 APIs, field for field and in upstream's
// order: the vote start signature covers Go's JSON encoding of VoteParams.

// File is a record file. Digest is the SHA256 of the decoded payload.
type File struct {
	Name    string `json:"name"`
	MIME    string `json:"mime"`
	Digest  string `json:"digest"`
	Payload string `json:"payload"`
}

// MetadataStream is a record metadata stream; Payload is JSON.
type MetadataStream struct {
	PluginID string `json:"pluginid"`
	StreamID uint32 `json:"streamid"`
	Payload  string `json:"payload"`
}

// CensorshipRecord is the server's signature over the record's merkle root
// and token.
type CensorshipRecord struct {
	Token     string `json:"token"`
	Merkle    string `json:"merkle"`
	Signature string `json:"signature"`
}

// Record is a records v1 record.
type Record struct {
	State            uint32           `json:"state"`
	Status           uint32           `json:"status"`
	Version          uint32           `json:"version"`
	Timestamp        int64            `json:"timestamp"`
	Username         string           `json:"username"`
	Metadata         []MetadataStream `json:"metadata"`
	Files            []File           `json:"files"`
	CensorshipRecord CensorshipRecord `json:"censorshiprecord"`
}

// UserMetadata is the author's signature over the record's merkle root.
type UserMetadata struct {
	UserID    string `json:"userid"`
	PublicKey string `json:"publickey"`
	Signature string `json:"signature"`
}

// StatusChange is a signed record status change.
type StatusChange struct {
	Token     string `json:"token"`
	Version   uint32 `json:"version"`
	Status    uint32 `json:"status"`
	Reason    string `json:"reason,omitempty"`
	PublicKey string `json:"publickey"`
	Signature string `json:"signature"`
	Timestamp int64  `json:"timestamp"`
}

// ProposalMetadata is the proposal's proposalmetadata.json file.
type ProposalMetadata struct {
	Name        string `json:"name"`
	Amount      uint64 `json:"amount"`
	StartDate   int64  `json:"startdate"`
	EndDate     int64  `json:"enddate"`
	Domain      string `json:"domain"`
	LegacyToken string `json:"legacytoken,omitempty"`
}

// Comment is a comments v1 comment.
type Comment struct {
	UserID        string `json:"userid"`
	Username      string `json:"username"`
	State         uint32 `json:"state"`
	Token         string `json:"token"`
	ParentID      uint32 `json:"parentid"`
	Comment       string `json:"comment"`
	PublicKey     string `json:"publickey"`
	Signature     string `json:"signature"`
	CommentID     uint32 `json:"commentid"`
	Version       uint32 `json:"version"`
	CreatedAt     int64  `json:"createdat"`
	Timestamp     int64  `json:"timestamp"`
	Receipt       string `json:"receipt"`
	Downvotes     uint64 `json:"downvotes"`
	Upvotes       uint64 `json:"upvotes"`
	Deleted       bool   `json:"deleted,omitempty"`
	Reason        string `json:"reason,omitempty"`
	ExtraData     string `json:"extradata,omitempty"`
	ExtraDataHint string `json:"extradatahint,omitempty"`
}

// AuthDetails is a signed vote authorization or revocation.
type AuthDetails struct {
	Token     string `json:"token"`
	Version   uint32 `json:"version"`
	Action    string `json:"action"`
	PublicKey string `json:"publickey"`
	Signature string `json:"signature"`
	Timestamp int64  `json:"timestamp"`
	Receipt   string `json:"receipt"`
}

// VoteOption is one choice of a vote.
type VoteOption struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Bit         uint64 `json:"bit"`
}

// VoteParams are the parameters an admin signs to start a vote.
type VoteParams struct {
	Token            string       `json:"token"`
	Version          uint32       `json:"version"`
	Type             uint32       `json:"type"`
	Mask             uint64       `json:"mask"`
	Duration         uint32       `json:"duration"`
	QuorumPercentage uint32       `json:"quorumpercentage"`
	PassPercentage   uint32       `json:"passpercentage"`
	Options          []VoteOption `json:"options"`
	Parent           string       `json:"parent,omitempty"`
}

// VoteDetails is a started vote: the signed parameters and the server's
// receipt over the signature and start block.
type VoteDetails struct {
	Params           VoteParams `json:"params"`
	PublicKey        string     `json:"publickey"`
	Signature        string     `json:"signature"`
	Receipt          string     `json:"receipt"`
	StartBlockHeight uint32     `json:"startblockheight"`
	StartBlockHash   string     `json:"startblockhash"`
	EndBlockHeight   uint32     `json:"endblockheight"`
	EligibleTickets  []string   `json:"eligibletickets"`
}

// BillingStatusChange is a signed change of an approved proposal's billing
// status.
type BillingStatusChange struct {
	Token     string `json:"token"`
	Status    uint32 `json:"status"`
	Reason    string `json:"reason,omitempty"`
	PublicKey string `json:"publickey"`
	Signature string `json:"signature"`
	Receipt   string `json:"receipt"`
	Timestamp int64  `json:"timestamp"`
}

// CastVote is a ballot as Politeia records it.
type CastVote struct {
	Token     string `json:"token"`
	Ticket    string `json:"ticket"`
	VoteBit   string `json:"votebit"`
	Address   string `json:"address"`
	Signature string `json:"signature"`
	Receipt   string `json:"receipt"`
	Timestamp int64  `json:"timestamp"`
}
