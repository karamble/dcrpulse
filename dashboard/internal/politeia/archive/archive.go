// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package archive keeps finished Politeia proposals on the dashboard's data
// volume, so they can be read while Politeia is unreachable. Nothing enters
// it unverified, and every file is verified again when it is loaded.
package archive

import (
	"errors"
	"fmt"

	"dcrpulse/internal/politeia/verify"
)

// SchemaVersion is the layout of an archived proposal file.
const SchemaVersion = 1

// Where an archived proposal came from.
const (
	SourceSnapshot = "snapshot"
	SourceRefresh  = "refresh"
	SourceImport   = "import"
)

// textFiles keep their payload in the archive; any other file, such as an
// image, keeps only its digest, which the signed merkle root still covers.
var textFiles = map[string]bool{
	"index.md":              true,
	"proposalmetadata.json": true,
	"votemetadata.json":     true,
}

// Summary is the vote outcome as Politeia reports it. Politeia does not sign
// it.
type Summary struct {
	Status           string   `json:"status"`
	Results          []Result `json:"results"`
	EligibleTickets  int64    `json:"eligibleTickets"`
	QuorumPercentage uint32   `json:"quorumPercentage"`
	PassPercentage   uint32   `json:"passPercentage"`
	StartBlockHeight int64    `json:"startBlockHeight"`
	EndBlockHeight   int64    `json:"endBlockHeight"`
}

// Result is one vote option's reported count.
type Result struct {
	ID      string `json:"id"`
	VoteBit uint64 `json:"voteBit"`
	Votes   int64  `json:"votes"`
}

// Proposal is one archived proposal: the signed objects as Politeia served
// them, minus image payloads and the eligible-ticket list, plus the reported
// summary.
type Proposal struct {
	SchemaVersion   int                          `json:"schemaVersion"`
	Record          verify.Record                `json:"record"`
	Comments        []verify.Comment             `json:"comments,omitempty"`
	DroppedComments []uint32                     `json:"droppedComments,omitempty"`
	Auths           []verify.AuthDetails         `json:"auths,omitempty"`
	Vote            *verify.VoteDetails          `json:"vote,omitempty"`
	Billing         []verify.BillingStatusChange `json:"billing,omitempty"`
	Summary         Summary                      `json:"summary"`
	Source          string                       `json:"source"`
	ArchivedAt      int64                        `json:"archivedAt"`
	VoteCheck       *VoteCheck                   `json:"voteCheck,omitempty"`
}

// VoteCheck is this dashboard's own check of a proposal's ballots against
// their signatures, Politeia's receipts, the eligible tickets and each
// ticket's commitment address on the chain.
type VoteCheck struct {
	Ballots      int              `json:"ballots"`
	Counted      int              `json:"counted"`
	Counts       map[string]int64 `json:"counts"`
	TallyMatches bool             `json:"tallyMatches"`
	Problems     []BallotProblem  `json:"problems,omitempty"`
	CheckedAt    int64            `json:"checkedAt"`
}

// BallotProblem is a ballot the check did not count, and why.
type BallotProblem struct {
	Ticket  string `json:"ticket"`
	Problem string `json:"problem"`
}

// Token is the proposal's censorship token.
func (p *Proposal) Token() string { return p.Record.CensorshipRecord.Token }

// Checked is what verification established about an archived proposal.
type Checked struct {
	Metadata verify.ProposalMetadata
	// Author is nil for a migrated proposal, whose original signatures were
	// blanked when Politeia moved it off its old backend in 2021.
	Author *verify.UserMetadata
}

// Migrated reports whether the proposal came from Politeia's old backend.
func (c Checked) Migrated() bool { return c.Metadata.LegacyToken != "" }

// Fetched is a proposal's data as fetched from Politeia, before it is
// verified.
type Fetched struct {
	Record   verify.Record
	Comments []verify.Comment
	Auths    []verify.AuthDetails
	Vote     *verify.VoteDetails
	Billing  []verify.BillingStatusChange
	Summary  Summary
}

// New verifies fetched data and returns it ready to store. A record or vote
// start that fails refuses the whole proposal; a comment, authorization or
// billing change that fails is left out, comments by ID. A migrated proposal
// keeps only its server-signed text.
func New(f Fetched, serverKey, source string, now int64) (*Proposal, error) {
	rec := f.Record
	if err := verify.CensorshipRecordVerify(rec, serverKey); err != nil {
		return nil, err
	}
	if err := textPresent(rec); err != nil {
		return nil, err
	}
	pm, err := verify.ProposalMetadataDecode(rec)
	if err != nil {
		return nil, err
	}
	p := &Proposal{
		SchemaVersion: SchemaVersion,
		Record:        stripped(rec),
		Summary:       f.Summary,
		Source:        source,
		ArchivedAt:    now,
	}
	if pm.LegacyToken != "" {
		return p, nil
	}
	if _, err := verify.UserMetadataVerify(rec); err != nil {
		return nil, err
	}
	if _, err := verify.StatusChangesVerify(rec); err != nil {
		return nil, err
	}
	token := rec.CensorshipRecord.Token
	for _, c := range f.Comments {
		if c.Token != token || verify.CommentVerify(c, serverKey) != nil {
			p.DroppedComments = append(p.DroppedComments, c.CommentID)
			continue
		}
		p.Comments = append(p.Comments, c)
	}
	for _, a := range f.Auths {
		if a.Token == token && verify.AuthDetailsVerify(a, serverKey) == nil {
			p.Auths = append(p.Auths, a)
		}
	}
	if f.Vote != nil {
		if f.Vote.Params.Token != token {
			return nil, errors.New("vote start is for another proposal")
		}
		if err := verify.VoteDetailsVerify(*f.Vote, serverKey); err != nil {
			return nil, err
		}
		vote := *f.Vote
		vote.EligibleTickets = nil
		p.Vote = &vote
	}
	for _, b := range f.Billing {
		if b.Token == token && verify.BillingStatusChangeVerify(b, serverKey) == nil {
			p.Billing = append(p.Billing, b)
		}
	}
	return p, nil
}

// Verify checks an archived proposal again, all of it: unlike New it leaves
// nothing out, so a file that fails anywhere is refused whole.
func Verify(p *Proposal, serverKey string) (Checked, error) {
	var ck Checked
	if p.SchemaVersion != SchemaVersion {
		return ck, fmt.Errorf("schema version %d, want %d", p.SchemaVersion, SchemaVersion)
	}
	rec := p.Record
	if err := verify.CensorshipRecordVerify(rec, serverKey); err != nil {
		return ck, err
	}
	if err := textPresent(rec); err != nil {
		return ck, err
	}
	pm, err := verify.ProposalMetadataDecode(rec)
	if err != nil {
		return ck, err
	}
	ck.Metadata = pm
	if ck.Migrated() {
		if len(p.Comments) > 0 || len(p.Auths) > 0 || p.Vote != nil || len(p.Billing) > 0 {
			return ck, errors.New("migrated proposal carries unverifiable data")
		}
		return ck, nil
	}
	um, err := verify.UserMetadataVerify(rec)
	if err != nil {
		return ck, err
	}
	ck.Author = &um
	if _, err := verify.StatusChangesVerify(rec); err != nil {
		return ck, err
	}
	token := rec.CensorshipRecord.Token
	for _, c := range p.Comments {
		if c.Token != token {
			return ck, fmt.Errorf("comment %d is for another proposal", c.CommentID)
		}
		if err := verify.CommentVerify(c, serverKey); err != nil {
			return ck, err
		}
	}
	for _, a := range p.Auths {
		if a.Token != token {
			return ck, errors.New("vote authorization is for another proposal")
		}
		if err := verify.AuthDetailsVerify(a, serverKey); err != nil {
			return ck, err
		}
	}
	if p.Vote != nil {
		if p.Vote.Params.Token != token {
			return ck, errors.New("vote start is for another proposal")
		}
		if err := verify.VoteDetailsVerify(*p.Vote, serverKey); err != nil {
			return ck, err
		}
	}
	for _, b := range p.Billing {
		if b.Token != token {
			return ck, errors.New("billing status change is for another proposal")
		}
		if err := verify.BillingStatusChangeVerify(b, serverKey); err != nil {
			return ck, err
		}
	}
	return ck, nil
}

// textPresent requires the proposal text and metadata to be there to read.
func textPresent(rec verify.Record) error {
	for _, name := range []string{"index.md", "proposalmetadata.json"} {
		found := false
		for _, f := range rec.Files {
			if f.Name == name && f.Payload != "" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s missing", name)
		}
	}
	return nil
}

// stripped drops every payload but the text files', keeping the digests.
func stripped(rec verify.Record) verify.Record {
	files := make([]verify.File, len(rec.Files))
	for i, f := range rec.Files {
		if !textFiles[f.Name] {
			f.Payload = ""
		}
		files[i] = f
	}
	rec.Files = files
	return rec
}
