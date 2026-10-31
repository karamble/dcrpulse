// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/politeia/archive"
	"dcrpulse/internal/politeia/verify"
	"dcrpulse/internal/types"
)

// piArchiveStore holds the archive of finished proposals; set by tests.
var piArchiveStore atomic.Pointer[archive.Store]

// politeiaArchive returns the archive kept on the dashboard data volume.
func politeiaArchive() *archive.Store {
	if s := piArchiveStore.Load(); s != nil {
		return s
	}
	s := archive.NewStore(filepath.Join(config.AppDataDir, "politeia"), verify.PinnedServerKey)
	if piArchiveStore.CompareAndSwap(nil, s) {
		return s
	}
	return piArchiveStore.Load()
}

// PoliteiaArchiveCount is how many proposals the archive holds.
func PoliteiaArchiveCount() int { return len(politeiaArchive().All()) }

// ExportPoliteiaArchive returns every archived proposal.
func ExportPoliteiaArchive() archive.File { return politeiaArchive().Export() }

// ImportPoliteiaArchive stores the proposals of an exported archive that
// verify with the pinned server key.
func ImportPoliteiaArchive(r io.Reader) (archive.ImportResult, error) {
	return politeiaArchive().Import(r)
}

// ArchiveOnlyError carries a list served from the archive alone because
// Politeia is switched off or did not answer.
type ArchiveOnlyError struct {
	Err error
}

func (e *ArchiveOnlyError) Error() string { return "served from the archive: " + e.Err.Error() }
func (e *ArchiveOnlyError) Unwrap() error { return e.Err }

// Reason is "disabled" when Politeia is switched off, else "unreachable".
func (e *ArchiveOnlyError) Reason() string {
	if errors.Is(e.Err, ErrPoliteiaDisabled) {
		return "disabled"
	}
	return "unreachable"
}

// billingStatusNames are the pi v1 billing statuses.
var billingStatusNames = map[uint32]string{1: "active", 2: "closed", 3: "completed"}

// archiveVoteStatuses are the vote statuses a proposal is archived in.
var archiveVoteStatuses = map[string]bool{"approved": true, "rejected": true, "finished": true, "ineligible": true}

// stillOpen reports whether an archived proposal can still change on
// Politeia: an approved one takes author updates and replies until its
// billing is closed or completed.
func stillOpen(e archive.Entry) bool {
	p := e.Proposal
	if p.Summary.Status != "approved" || e.Checked.Migrated() {
		return false
	}
	n := len(p.Billing)
	return n == 0 || p.Billing[n-1].Status == 1
}

// errCensored is a censored record: Politeia deleted its files, so there is
// nothing to archive.
var errCensored = errors.New("censored record")

// notVerifiedError carries Politeia data whose signatures did not verify.
type notVerifiedError struct{ err error }

func (e *notVerifiedError) Error() string { return "signatures did not verify: " + e.err.Error() }

type piDetailsResp struct {
	Record verify.Record `json:"record"`
}

type piVoteRecordResp struct {
	Auths []verify.AuthDetails `json:"auths"`
	Vote  *verify.VoteDetails  `json:"vote"`
}

type piBillingResp struct {
	BillingStatusChanges map[string][]verify.BillingStatusChange `json:"billingstatuschanges"`
}

// fetchArchivable fetches every part of a proposal the archive keeps. The
// record comes whole, images included, since the merkle root covers them.
func fetchArchivable(ctx context.Context, token string, sum piSummary) (archive.Fetched, error) {
	req := struct {
		Token string `json:"token"`
	}{token}
	var rec piDetailsResp
	if err := piPost(ctx, "/records/v1/details", req, &rec, politeiaTimeout); err != nil {
		return archive.Fetched{}, err
	}
	if rec.Record.Status == verify.RecordStatusCensored {
		return archive.Fetched{}, errCensored
	}
	cmts, err := piComments(ctx, token)
	if err != nil {
		return archive.Fetched{}, err
	}
	var vote piVoteRecordResp
	if err := piPost(ctx, "/ticketvote/v1/details", req, &vote, politeiaTimeout); err != nil {
		return archive.Fetched{}, err
	}
	var bill piBillingResp
	breq := struct {
		Tokens []string `json:"tokens"`
	}{[]string{token}}
	if err := piPost(ctx, "/pi/v1/billingstatuschanges", breq, &bill, politeiaTimeout); err != nil {
		return archive.Fetched{}, err
	}
	return archive.Fetched{
		Record:   rec.Record,
		Comments: cmts,
		Auths:    vote.Auths,
		Vote:     vote.Vote,
		Billing:  bill.BillingStatusChanges[token],
		Summary:  archiveSummary(sum),
	}, nil
}

func archiveSummary(sum piSummary) archive.Summary {
	out := archive.Summary{
		Status:           piStatusName[sum.Status],
		EligibleTickets:  sum.EligibleTicketCount,
		QuorumPercentage: uint32(sum.QuorumPercentage),
		PassPercentage:   uint32(sum.PassPercentage),
		StartBlockHeight: sum.StartBlockHeight,
		EndBlockHeight:   sum.EndBlockHeight,
	}
	for _, r := range sum.Results {
		out.Results = append(out.Results, archive.Result{ID: r.ID, VoteBit: uint64(r.VoteBit), Votes: r.Votes})
	}
	return out
}

// archiveFromPoliteia fetches a finished proposal, verifies it and stores it.
// Data that fails verification comes back as a notVerifiedError.
func archiveFromPoliteia(ctx context.Context, token string, sum piSummary, source string) (archive.Entry, error) {
	f, err := fetchArchivable(ctx, token, sum)
	if err != nil {
		return archive.Entry{}, err
	}
	if f.Record.CensorshipRecord.Token != token {
		return archive.Entry{}, &notVerifiedError{errors.New("record is for another proposal")}
	}
	s := politeiaArchive()
	p, err := archive.New(f, s.ServerKey(), source, time.Now().Unix())
	if err != nil {
		return archive.Entry{}, &notVerifiedError{err}
	}
	// A finished vote's ballots do not change, but the reported tally may.
	if old, ok := s.Get(token); ok && old.Proposal.VoteCheck != nil {
		vc := *old.Proposal.VoteCheck
		vc.TallyMatches = tallyMatches(vc.Counts, p.Summary.Results)
		p.VoteCheck = &vc
	}
	e, err := s.Put(p)
	if err != nil {
		// Verified but not written: still serve it.
		govnLog.Warnf("Politeia proposal %s not stored: %v", token, err)
		ck, err := archive.Verify(p, s.ServerKey())
		if err != nil {
			return archive.Entry{}, &notVerifiedError{err}
		}
		return archive.Entry{Proposal: p, Checked: ck}, nil
	}
	return e, nil
}

// archivedProposals lists the archived proposals in a status bucket, newest
// vote first.
func archivedProposals(ctx context.Context, bucket string) []types.Proposal {
	var out []types.Proposal
	for _, e := range politeiaArchive().All() {
		p := proposalFromArchive(e)
		if p.Status == bucket {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return out
	}
	votes, counts := loadLocalPoliteiaVotes(ctx), loadLocalPoliteiaVoteCounts(ctx)
	for i := range out {
		out[i].CurrentChoice = votes[out[i].Token]
		out[i].VotedTicketCount = counts[out[i].Token]
	}
	sortByEndBlock(out)
	return out
}

// withArchived adds the bucket's archived proposals that the live list does
// not already hold, and marks the ones it does.
func withArchived(ctx context.Context, live []types.Proposal, bucket string) []types.Proposal {
	archived := archivedProposals(ctx, bucket)
	if len(archived) == 0 {
		return live
	}
	have := map[string]bool{}
	for _, p := range archived {
		have[p.Token] = true
	}
	out := make([]types.Proposal, 0, len(live)+len(archived))
	for _, p := range live {
		if have[p.Token] {
			p.Archived = true
			delete(have, p.Token)
		}
		out = append(out, p)
	}
	for _, p := range archived {
		if have[p.Token] {
			out = append(out, p)
		}
	}
	sortByEndBlock(out)
	return out
}

func sortByEndBlock(ps []types.Proposal) {
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].EndBlock > ps[j].EndBlock })
}

// proposalFromArchive is the list row of an archived proposal; its vote
// figures are Politeia's unsigned summary.
func proposalFromArchive(e archive.Entry) types.Proposal {
	p, sum := e.Proposal, e.Proposal.Summary
	out := types.Proposal{
		Token:           p.Token(),
		Name:            e.Checked.Metadata.Name,
		Username:        p.Record.Username,
		Status:          summaryStatusToBucket(sum.Status),
		VoteStatus:      sum.Status,
		VoteCounts:      map[string]int64{},
		EligibleTickets: sum.EligibleTickets,
		EndBlock:        sum.EndBlockHeight,
		Archived:        true,
	}
	if out.Name == "" {
		out.Name = p.Token()
	}
	for _, r := range sum.Results {
		out.VoteCounts[r.ID] = r.Votes
		out.TotalVotes += r.Votes
	}
	if sum.EligibleTickets > 0 && sum.QuorumPercentage > 0 {
		out.QuorumMin = sum.EligibleTickets * int64(sum.QuorumPercentage) / 100
	}
	return out
}

// detailFromArchive is the detail view of an archived proposal, with what
// its signatures cover.
func detailFromArchive(e archive.Entry, serverKey string) *types.ProposalDetail {
	p := e.Proposal
	d := &types.ProposalDetail{
		Proposal:    proposalFromArchive(e),
		SubmittedAt: p.Record.Timestamp,
	}
	for _, f := range p.Record.Files {
		if f.Name == "index.md" {
			if raw, err := base64.StdEncoding.DecodeString(f.Payload); err == nil {
				d.Description = string(raw)
				d.DescriptionHTML = renderProposalMarkdown(d.Description)
			}
		}
	}
	// The started vote's options are signed; the summary's are not.
	if p.Vote != nil {
		for _, o := range p.Vote.Params.Options {
			d.VoteOptions = append(d.VoteOptions, types.ProposalVoteOption{ID: o.ID, Bit: uint32(o.Bit)})
		}
	} else {
		for _, r := range p.Summary.Results {
			d.VoteOptions = append(d.VoteOptions, types.ProposalVoteOption{ID: r.ID, Bit: uint32(r.VoteBit)})
		}
	}
	d.Comments = commentsFromPi(p.Comments)

	v := &types.ProposalVerification{
		ServerKey:       serverKey,
		Migrated:        e.Checked.Migrated(),
		LegacyToken:     e.Checked.Metadata.LegacyToken,
		Comments:        len(p.Comments),
		CommentsLeftOut: len(p.DroppedComments),
		Authorized:      len(p.Auths) > 0,
		Source:          p.Source,
		ArchivedAt:      p.ArchivedAt,
	}
	if e.Checked.Author != nil {
		v.AuthorKey = e.Checked.Author.PublicKey
	}
	if p.Vote != nil {
		v.VoteStartKey = p.Vote.PublicKey
	}
	if n := len(p.Billing); n > 0 {
		v.BillingStatus = billingStatusNames[p.Billing[n-1].Status]
	}
	if vc := p.VoteCheck; vc != nil {
		v.VoteCheck = &types.ProposalVoteCheck{
			Ballots: vc.Ballots, Counted: vc.Counted, Counts: vc.Counts,
			// Against the tally shown, which may differ from the one checked.
			TallyMatches: tallyMatches(vc.Counts, p.Summary.Results), CheckedAt: vc.CheckedAt,
		}
		for _, b := range vc.Problems {
			v.VoteCheck.Problems = append(v.VoteCheck.Problems, types.ProposalBallotProblem(b))
		}
	}
	d.Verification = v
	return d
}

// archivedDetail returns the archived detail of a proposal, with the
// wallet's own vote applied.
func archivedDetail(ctx context.Context, e archive.Entry) (*types.ProposalDetail, time.Time, error) {
	d := detailFromArchive(e, politeiaArchive().ServerKey())
	d.CurrentChoice = loadLocalPoliteiaVotes(ctx)[e.Proposal.Token()]
	d.VotedTicketCount = loadLocalPoliteiaVoteCounts(ctx)[e.Proposal.Token()]
	return d, time.Unix(e.Proposal.ArchivedAt, 0), nil
}
