// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"

	"dcrpulse/internal/politeia/archive"
	"dcrpulse/internal/politeia/verify"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
)

var (
	// ErrVoteCheckRunning refuses a second vote check while one runs.
	ErrVoteCheckRunning = errors.New("a vote check is already running")
	// ErrNoVoteToCheck refuses a proposal that is not archived with a
	// started vote.
	ErrNoVoteToCheck = errors.New("only an archived proposal whose vote started can have its votes checked")
)

var (
	vcMu      sync.Mutex
	vcRunning string
	vcStates  = map[string]*types.ProposalVoteCheckProgress{}
)

// votesBundle is politeiaverify's votes bundle, the file politeiagui offers
// as [token]-votes.json.
type votesBundle struct {
	Auths           []verify.AuthDetails `json:"auths,omitempty"`
	Details         *verify.VoteDetails  `json:"details,omitempty"`
	Votes           []verify.CastVote    `json:"votes,omitempty"`
	ServerPublicKey string               `json:"serverpublickey"`
}

// ticketCommitmentAddr returns the ticket's largest commitment address, which
// Politeia requires a ballot to be signed with. A variable for tests.
var ticketCommitmentAddr = func(ctx context.Context, ticket string, params *chaincfg.Params) (string, error) {
	if rpc.DcrdClient == nil {
		return "", errors.New("dcrd client not available")
	}
	hash, err := chainhash.NewHashFromStr(ticket)
	if err != nil {
		return "", err
	}
	tx, err := rpc.DcrdClient.GetRawTransaction(ctx, hash)
	if err != nil {
		return "", err
	}
	return largestCommitmentAddr(tx.MsgTx(), params)
}

// largestCommitmentAddr picks the commitment with the largest amount, the
// first of equals, as politeiad does.
func largestCommitmentAddr(tx *wire.MsgTx, params *chaincfg.Params) (string, error) {
	if !stake.IsSStx(tx) {
		return "", errors.New("not a ticket")
	}
	best, bestAmt := "", dcrutil.Amount(0)
	for _, out := range tx.TxOut {
		amt, err := stake.AmountFromSStxPkScrCommitment(out.PkScript)
		if err != nil {
			continue
		}
		addr, err := stake.AddrFromSStxPkScrCommitment(out.PkScript, params)
		if err != nil {
			continue
		}
		if amt > bestAmt {
			best, bestAmt = addr.String(), amt
		}
	}
	if best == "" {
		return "", errors.New("no commitment address")
	}
	return best, nil
}

// StartProposalVoteCheck starts checking an archived proposal's ballots in
// the background. One check runs at a time.
func StartProposalVoteCheck(token string) error {
	if ShuttingDown() {
		return ErrShuttingDown
	}
	if !PoliteiaEnabled() {
		return ErrPoliteiaDisabled
	}
	e, ok := politeiaArchive().Get(token)
	if !ok || e.Proposal.Vote == nil {
		return ErrNoVoteToCheck
	}
	vcMu.Lock()
	defer vcMu.Unlock()
	if vcRunning != "" {
		return ErrVoteCheckRunning
	}
	vcRunning = token
	vcStates[token] = &types.ProposalVoteCheckProgress{Running: true}
	go func() {
		err := checkVotes(context.Background(), token, e)
		if err != nil {
			govnLog.Warnf("Vote check of proposal %s stopped: %v", token, err)
		}
		vcMu.Lock()
		defer vcMu.Unlock()
		st := vcStates[token]
		st.Running = false
		if err != nil {
			st.Error = err.Error()
		}
		vcRunning = ""
	}()
	return nil
}

// ProposalVoteCheckProgress reports the proposal's vote check.
func ProposalVoteCheckProgress(token string) types.ProposalVoteCheckProgress {
	vcMu.Lock()
	defer vcMu.Unlock()
	if st, ok := vcStates[token]; ok {
		return *st
	}
	return types.ProposalVoteCheckProgress{}
}

func vcProgress(token string, done, total int) {
	vcMu.Lock()
	defer vcMu.Unlock()
	st := vcStates[token]
	st.Done, st.Total = done, total
}

// checkVotes fetches the ballots and checks each as politeiaverify does, plus
// the ticket's commitment address on this node, then compares the count with
// Politeia's tally. A ballot that fails is listed; a node that cannot answer
// stops the check.
func checkVotes(ctx context.Context, token string, e archive.Entry) error {
	params, err := chainParams(ctx)
	if err != nil {
		return err
	}
	key := politeiaArchive().ServerKey()
	req := struct {
		Token string `json:"token"`
	}{token}
	var vote piVoteRecordResp
	if err := piPost(ctx, "/ticketvote/v1/details", req, &vote, ProposalsFetchTimeout); err != nil {
		return err
	}
	if vote.Vote == nil || vote.Vote.Signature != e.Proposal.Vote.Signature {
		return errors.New("Politeia's vote start differs from the archived one")
	}
	if err := verify.VoteDetailsVerify(*vote.Vote, key); err != nil {
		return err
	}
	var res struct {
		Votes []verify.CastVote `json:"votes"`
	}
	if err := piPost(ctx, "/ticketvote/v1/results", req, &res, ProposalsFetchTimeout); err != nil {
		return err
	}
	vcProgress(token, 0, len(res.Votes))

	eligible := make(map[string]bool, len(vote.Vote.EligibleTickets))
	for _, t := range vote.Vote.EligibleTickets {
		eligible[t] = true
	}
	options := map[uint64]string{}
	for _, o := range e.Proposal.Vote.Params.Options {
		options[o.Bit] = o.ID
	}
	vc := archive.VoteCheck{Ballots: len(res.Votes), Counts: map[string]int64{}}
	seen := make(map[string]bool, len(res.Votes))
	for i, b := range res.Votes {
		bit, bitErr := parseVoteBit(b.VoteBit)
		option, isOption := options[bit]
		problem := ""
		switch {
		case b.Token != token:
			problem = "ballot is for another proposal"
		case !canonicalTicket(b.Ticket):
			problem = "ticket is not a transaction hash"
		case seen[b.Ticket]:
			problem = "a second ballot for the ticket"
		case !eligible[b.Ticket]:
			problem = "ticket was not eligible"
		case bitErr != nil || !isOption:
			problem = "vote bit is not an option"
		default:
			if err := verify.CastVoteVerify(b, key, params); err != nil {
				problem = err.Error()
			}
		}
		seen[b.Ticket] = true
		if problem == "" {
			addr, err := ticketCommitmentAddr(ctx, b.Ticket, params)
			if err != nil {
				return fmt.Errorf("ticket %s on this node: %w", b.Ticket, err)
			}
			if addr != b.Address {
				problem = "address is not the ticket's commitment address"
			}
		}
		if problem != "" {
			vc.Problems = append(vc.Problems, archive.BallotProblem{Ticket: b.Ticket, Problem: problem})
		} else {
			vc.Counted++
			vc.Counts[option]++
		}
		vcProgress(token, i+1, len(res.Votes))
	}
	vc.TallyMatches = tallyMatches(vc.Counts, e.Proposal.Summary.Results)
	vc.CheckedAt = time.Now().Unix()

	bundle, err := json.Marshal(votesBundle{
		Auths: e.Proposal.Auths, Details: vote.Vote, Votes: res.Votes, ServerPublicKey: key,
	})
	if err != nil {
		return err
	}
	return politeiaArchive().SetVoteCheck(token, e.Proposal, vc, bundle)
}

// canonicalTicket reports whether t is a transaction hash spelled as dcrd
// writes it, so each ticket has a single spelling to count.
func canonicalTicket(t string) bool {
	h, err := chainhash.NewHashFromStr(t)
	return err == nil && h.String() == t
}

// tallyMatches reports whether the counted ballots equal Politeia's tally,
// option by option.
func tallyMatches(counts map[string]int64, results []archive.Result) bool {
	reported := map[string]int64{}
	for _, r := range results {
		reported[r.ID] = r.Votes
	}
	for id, n := range counts {
		if reported[id] != n {
			return false
		}
	}
	for id, n := range reported {
		if counts[id] != n {
			return false
		}
	}
	return true
}

// ProposalVotesFile is where the ballots of a checked proposal are kept.
func ProposalVotesFile(token string) (string, error) {
	return politeiaArchive().VotesFile(token)
}
