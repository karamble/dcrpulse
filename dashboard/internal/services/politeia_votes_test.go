// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"

	"dcrpulse/internal/politeia/archive"
	"dcrpulse/internal/politeia/pitest"
	"dcrpulse/internal/politeia/verify"
	"dcrpulse/internal/types"
)

func ticketHash(c byte) string { return strings.Repeat(string(c), 64) }

// onMainnet answers CurrentNetwork without a node.
func onMainnet(t *testing.T) {
	t.Helper()
	networkMu.Lock()
	old := networkVal
	networkVal = "mainnet"
	networkMu.Unlock()
	t.Cleanup(func() { networkMu.Lock(); networkVal = old; networkMu.Unlock() })
}

// commitments stands in for the node's ticket lookups.
func commitments(t *testing.T, lookup func(ticket string) (string, error)) {
	t.Helper()
	prev := ticketCommitmentAddr
	ticketCommitmentAddr = func(_ context.Context, ticket string, _ *chaincfg.Params) (string, error) {
		return lookup(ticket)
	}
	t.Cleanup(func() { ticketCommitmentAddr = prev })
}

func resetVoteChecks(t *testing.T) {
	t.Helper()
	clear := func() {
		vcMu.Lock()
		vcRunning = ""
		vcStates = map[string]*types.ProposalVoteCheckProgress{}
		vcMu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

func waitVoteCheck(t *testing.T, token string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for ProposalVoteCheckProgress(token).Running {
		if time.Now().After(deadline) {
			t.Fatal("vote check did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type voteFixture struct {
	server  pitest.Key
	store   *archive.Store
	p       *pitest.Proposal
	ballots []verify.CastVote
	owners  map[string]string
}

const vcToken = "cdcdcdcdcdcdcdcd"

// newVoteFixture archives a proposal whose tally Politeia reports as yes and
// no votes, and serves its vote start and ballots.
func newVoteFixture(t *testing.T, yes, no int64) *voteFixture {
	t.Helper()
	pointExternalCfg(t)
	resetPiDetailCache(t)
	resetVoteChecks(t)
	onMainnet(t)
	s, server := useArchive(t)
	p := pitest.NewProposal(server, vcToken, pitest.Options{Name: "Voted"})
	f := archiveFetched(p, "approved", 802016)
	f.Summary.Results = []archive.Result{{ID: "no", VoteBit: 1, Votes: no}, {ID: "yes", VoteBit: 2, Votes: yes}}
	putArchived(t, s, server, f)

	fx := &voteFixture{server: server, store: s, p: p, owners: map[string]string{}}
	p.Vote.EligibleTickets = []string{ticketHash('1'), ticketHash('2'), ticketHash('3'), ticketHash('5'), ticketHash('6'), ticketHash('7')}
	vote := func(c byte, bit string) verify.CastVote {
		v := pitest.NewVoter()
		fx.owners[ticketHash(c)] = v.Address
		return v.Ballot(server, vcToken, ticketHash(c), bit)
	}
	good1, good2 := vote('1', "2"), vote('2', "1")
	wrongAddr := vote('3', "2")
	fx.owners[ticketHash('3')] = pitest.NewVoter().Address
	notEligible := vote('4', "2")
	forged := vote('5', "1")
	forged.Receipt = pitest.NewKey().Sign(forged.Signature)
	// Signed for another proposal, with an eligible ticket.
	other := pitest.NewVoter()
	fx.owners[ticketHash('6')] = other.Address
	elsewhere := other.Ballot(server, "efefefefefefefef", ticketHash('6'), "2")
	noOption := vote('7', "4")
	fx.ballots = []verify.CastVote{good1, good2, wrongAddr, notEligible, good1, forged, elsewhere, noOption}
	commitments(t, func(ticket string) (string, error) { return fx.owners[ticket], nil })
	fx.serve(t)
	return fx
}

func (fx *voteFixture) serve(t *testing.T) {
	t.Helper()
	piRouter(t, map[string]http.HandlerFunc{
		"/api/ticketvote/v1/details": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"auths": fx.p.Auths, "vote": fx.p.Vote})
		},
		"/api/ticketvote/v1/results": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"votes": fx.ballots})
		},
	})
}

func TestVoteCheckCountsWhatVerifies(t *testing.T) {
	fx := newVoteFixture(t, 1, 1)
	if err := StartProposalVoteCheck(vcToken); err != nil {
		t.Fatal(err)
	}
	waitVoteCheck(t, vcToken)
	if st := ProposalVoteCheckProgress(vcToken); st.Error != "" || st.Done != 8 || st.Total != 8 {
		t.Fatalf("progress %+v", st)
	}

	e, _ := fx.store.Get(vcToken)
	vc := e.Proposal.VoteCheck
	if vc == nil || vc.Ballots != 8 || vc.Counted != 2 || vc.Counts["yes"] != 1 || vc.Counts["no"] != 1 || !vc.TallyMatches {
		t.Fatalf("vote check %+v", vc)
	}
	want := map[string]string{
		ticketHash('3'): "address is not the ticket's commitment address",
		ticketHash('4'): "ticket was not eligible",
		ticketHash('1'): "a second ballot for the ticket",
		ticketHash('5'): "ballot receipt",
		ticketHash('6'): "ballot is for another proposal",
		ticketHash('7'): "vote bit is not an option",
	}
	if len(vc.Problems) != len(want) {
		t.Fatalf("problems %+v", vc.Problems)
	}
	for _, p := range vc.Problems {
		if !strings.Contains(p.Problem, want[p.Ticket]) {
			t.Errorf("ticket %s: %q, want %q", p.Ticket[:4], p.Problem, want[p.Ticket])
		}
	}

	path, _ := ProposalVotesFile(vcToken)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var b votesBundle
	if err := json.Unmarshal(raw, &b); err != nil || b.ServerPublicKey != fx.server.Pub || len(b.Votes) != 8 ||
		b.Details == nil || len(b.Details.EligibleTickets) != 6 || len(b.Auths) != 1 {
		t.Fatalf("votes bundle: %v %+v", err, b)
	}

	d, _, err := GetProposalDetail(context.Background(), vcToken)
	if err != nil || d.Verification.VoteCheck == nil || d.Verification.VoteCheck.Counted != 2 ||
		!d.Verification.VoteCheck.TallyMatches || len(d.Verification.VoteCheck.Problems) != 6 {
		t.Fatalf("detail vote check: %v %+v", err, d.Verification)
	}
}

// Another spelling of a ticket hash names the same ticket on dcrd, so only
// the canonical spelling counts, even when Politeia lists them all.
func TestVoteCheckCountsEachTicketOnce(t *testing.T) {
	fx := newVoteFixture(t, 1, 0)
	h := strings.Repeat("ab", 32)
	spellings := []string{h, strings.ToUpper(h), "A" + h[1:], h[1:]}
	v := pitest.NewVoter()
	fx.p.Vote.EligibleTickets = spellings
	fx.ballots = nil
	for _, sp := range spellings {
		fx.owners[sp] = v.Address
		fx.ballots = append(fx.ballots, v.Ballot(fx.server, vcToken, sp, "2"))
	}
	if err := StartProposalVoteCheck(vcToken); err != nil {
		t.Fatal(err)
	}
	waitVoteCheck(t, vcToken)
	e, _ := fx.store.Get(vcToken)
	vc := e.Proposal.VoteCheck
	if vc == nil || vc.Counted != 1 || vc.Counts["yes"] != 1 || len(vc.Problems) != 3 {
		t.Fatalf("vote check %+v", vc)
	}
	for _, p := range vc.Problems {
		if p.Problem != "ticket is not a transaction hash" {
			t.Errorf("ticket %s: %q", p.Ticket, p.Problem)
		}
	}
}

func TestVoteCheckComparesWithPoliteiasTally(t *testing.T) {
	fx := newVoteFixture(t, 9000, 1)
	if err := StartProposalVoteCheck(vcToken); err != nil {
		t.Fatal(err)
	}
	waitVoteCheck(t, vcToken)
	if e, _ := fx.store.Get(vcToken); e.Proposal.VoteCheck == nil || e.Proposal.VoteCheck.TallyMatches {
		t.Fatalf("a tally Politeia overstated matched: %+v", e.Proposal.VoteCheck)
	}
}

// A tally that names votes no ballot was cast for does not match either.
func TestVoteCheckCatchesVotesNobodyCast(t *testing.T) {
	fx := newVoteFixture(t, 1, 1)
	e, _ := fx.store.Get(vcToken)
	p := *e.Proposal
	p.Summary.Results = append(p.Summary.Results, archive.Result{ID: "abstain", VoteBit: 4, Votes: 3})
	if _, err := fx.store.Put(&p); err != nil {
		t.Fatal(err)
	}
	if err := StartProposalVoteCheck(vcToken); err != nil {
		t.Fatal(err)
	}
	waitVoteCheck(t, vcToken)
	if e, _ := fx.store.Get(vcToken); e.Proposal.VoteCheck == nil || e.Proposal.VoteCheck.TallyMatches {
		t.Fatalf("a tally with votes nobody cast matched: %+v", e.Proposal.VoteCheck)
	}
}

// The ballots belong to the vote start the archive verified, not another.
func TestVoteCheckRefusesAnotherVoteStart(t *testing.T) {
	fx := newVoteFixture(t, 1, 1)
	fx.p.Vote = pitest.NewProposal(fx.server, vcToken, pitest.Options{Name: "Voted"}).Vote
	if err := StartProposalVoteCheck(vcToken); err != nil {
		t.Fatal(err)
	}
	waitVoteCheck(t, vcToken)
	if st := ProposalVoteCheckProgress(vcToken); !strings.Contains(st.Error, "differs from the archived one") {
		t.Fatalf("progress %+v", st)
	}
}

func TestVoteCheckStopsWhenTheNodeCannotAnswer(t *testing.T) {
	fx := newVoteFixture(t, 1, 1)
	commitments(t, func(string) (string, error) { return "", errors.New("no transaction index") })
	if err := StartProposalVoteCheck(vcToken); err != nil {
		t.Fatal(err)
	}
	waitVoteCheck(t, vcToken)
	if st := ProposalVoteCheckProgress(vcToken); !strings.Contains(st.Error, "on this node: no transaction index") {
		t.Fatalf("progress %+v", st)
	}
	if e, _ := fx.store.Get(vcToken); e.Proposal.VoteCheck != nil {
		t.Fatal("a check the node could not finish was stored")
	}
}

func TestVoteCheckRefusals(t *testing.T) {
	fx := newVoteFixture(t, 1, 1)
	if err := StartProposalVoteCheck("eeeeeeeeeeeeeeee"); !errors.Is(err, ErrNoVoteToCheck) {
		t.Fatalf("unarchived: %v", err)
	}
	putArchived(t, fx.store, fx.server, archiveFetched(
		pitest.NewProposal(fx.server, arMigrated, pitest.Options{Name: "Old", LegacyToken: "27f87171d98b7923"}), "approved", 1))
	if err := StartProposalVoteCheck(arMigrated); !errors.Is(err, ErrNoVoteToCheck) {
		t.Fatalf("migrated: %v", err)
	}

	release := make(chan struct{})
	commitments(t, func(ticket string) (string, error) { <-release; return fx.owners[ticket], nil })
	if err := StartProposalVoteCheck(vcToken); err != nil {
		t.Fatal(err)
	}
	if err := StartProposalVoteCheck(vcToken); !errors.Is(err, ErrVoteCheckRunning) {
		t.Fatalf("second check: %v", err)
	}
	close(release)
	waitVoteCheck(t, vcToken)

	politeiaOff(t)
	if err := StartProposalVoteCheck(vcToken); !errors.Is(err, ErrPoliteiaDisabled) {
		t.Fatalf("Politeia off: %v", err)
	}
}

func TestRefreshKeepsTheVoteCheck(t *testing.T) {
	fx := newVoteFixture(t, 1, 1)
	if err := StartProposalVoteCheck(vcToken); err != nil {
		t.Fatal(err)
	}
	waitVoteCheck(t, vcToken)
	var calls int
	piProposalRouter(t, fx.p, 5, &calls)
	if _, _, err := RefreshProposalDetail(context.Background(), vcToken); err != nil {
		t.Fatal(err)
	}
	if e, _ := fx.store.Get(vcToken); e.Proposal.VoteCheck == nil || e.Proposal.Source != archive.SourceRefresh {
		t.Fatalf("after refresh: %+v", e.Proposal)
	}
}

// ticketTx builds a ticket committing amounts to fresh addresses.
func ticketTx(t *testing.T, amounts ...int64) (*wire.MsgTx, []string) {
	t.Helper()
	params := chaincfg.MainNetParams()
	tx := wire.NewMsgTx()
	var addrs []string
	for i, amt := range amounts {
		a, err := stdaddr.DecodeAddress(pitest.NewVoter().Address, params)
		if err != nil {
			t.Fatal(err)
		}
		sa := a.(stdaddr.StakeAddress)
		if i == 0 {
			ver, script := sa.VotingRightsScript()
			tx.AddTxOut(&wire.TxOut{Value: 1e8, Version: ver, PkScript: script})
		}
		tx.AddTxIn(&wire.TxIn{ValueIn: amt})
		ver, script := sa.RewardCommitmentScript(amt, 0, 1<<24)
		tx.AddTxOut(&wire.TxOut{Version: ver, PkScript: script})
		ver, script = sa.StakeChangeScript()
		tx.AddTxOut(&wire.TxOut{Version: ver, PkScript: script})
		addrs = append(addrs, sa.String())
	}
	return tx, addrs
}

func TestLargestCommitmentAddr(t *testing.T) {
	params := chaincfg.MainNetParams()
	tx, addrs := ticketTx(t, 3e8, 5e8, 1e8)
	if got, err := largestCommitmentAddr(tx, params); err != nil || got != addrs[1] {
		t.Fatalf("largest = %s %v, want %s", got, err, addrs[1])
	}
	tx, addrs = ticketTx(t, 4e8, 4e8)
	if got, _ := largestCommitmentAddr(tx, params); got != addrs[0] {
		t.Fatalf("tie = %s, want the first %s", got, addrs[0])
	}
	// Commitment outputs without the ticket around them.
	tx, _ = ticketTx(t, 2e8)
	tx.TxOut = tx.TxOut[1:2]
	if _, err := largestCommitmentAddr(tx, params); err == nil {
		t.Fatal("a transaction that is not a ticket had a commitment address")
	}
}
