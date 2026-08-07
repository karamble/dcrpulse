// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrd/chaincfg/v3"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// Vote bits for a whole voting window, keyed by the stake version they were
// cast under. One getstakeversions call covers an entire window, and every
// agenda of a version shares it, so caching per version means four agendas of
// the same version cost one fetch rather than four.
//
// A settled window can never change, so it is kept for the process lifetime;
// an open one is re-read after windowVotesLiveTTL so a running vote advances.
var (
	windowVotesMu   sync.Mutex
	windowVotesData = map[uint32]*windowVotes{}
)

const windowVotesLiveTTL = 30 * time.Second

type windowVotes struct {
	bits     []uint16
	start    int64
	end      int64
	complete bool
	fetched  time.Time
}

// agendaVoteBits returns every vote bit cast under voteVersion across
// [start, end]. The mutex is held across the fetch so concurrent callers for
// the same version wait rather than each pulling the same couple of megabytes,
// matching how describeGraph single-flights its heavy call.
func agendaVoteBits(ctx context.Context, voteVersion uint32, start, end int64, complete bool) (*windowVotes, error) {
	windowVotesMu.Lock()
	defer windowVotesMu.Unlock()

	if got, ok := windowVotesData[voteVersion]; ok && got.start == start && got.end == end {
		if got.complete || time.Since(got.fetched) < windowVotesLiveTTL {
			return got, nil
		}
	}

	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}

	hash, err := rpc.DcrdClient.GetBlockHash(ctx, end)
	if err != nil {
		return nil, fmt.Errorf("get block hash at %d: %w", end, err)
	}
	count := end - start + 1
	res, err := rpc.DcrdClient.GetStakeVersions(ctx, hash.String(), int32(count))
	if err != nil {
		return nil, fmt.Errorf("getstakeversions %d blocks to %d: %w", count, end, err)
	}
	// dcrd walks backwards from the given hash and silently clamps an
	// over-large count to the history it has, so the span is checked rather
	// than assumed.
	if int64(len(res.StakeVersions)) < count {
		return nil, fmt.Errorf("getstakeversions returned %d of %d blocks for window %d-%d",
			len(res.StakeVersions), count, start, end)
	}

	bits := make([]uint16, 0, len(res.StakeVersions)*int(5))
	for _, b := range res.StakeVersions {
		for _, v := range b.Votes {
			if v.Version == voteVersion {
				bits = append(bits, v.Bits)
			}
		}
	}

	w := &windowVotes{bits: bits, start: start, end: end, complete: complete, fetched: time.Now()}
	windowVotesData[voteVersion] = w
	return w, nil
}

// AgendaVoteHistory reconstructs one agenda's tally from the votes cast in its
// own voting window. For an agenda still being voted on the window is clamped
// to the tip and the result is marked incomplete.
func AgendaVoteHistory(ctx context.Context, agendaID string) (*types.AgendaVotes, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}
	params, err := chainParams(ctx)
	if err != nil {
		return nil, err
	}

	agenda, voteVersion, start, end, complete, err := locateAgenda(ctx, params, agendaID)
	if err != nil {
		return nil, err
	}

	w, err := agendaVoteBits(ctx, voteVersion, start, end, complete)
	if err != nil {
		return nil, err
	}
	// A vote that failed by expiry never opened, so its derived window holds
	// no votes of this version. Reporting zeroes would look like a rejection.
	if len(w.bits) == 0 {
		return nil, fmt.Errorf("agenda %q has no recorded votes in %d-%d", agendaID, start, end)
	}

	t := countAgendaVotes(w.bits, agenda.Mask, agenda.Choices)
	var yes, no, abstain int64
	for _, c := range agenda.Choices {
		switch {
		case c.IsAbstain:
			abstain = t.byChoice[c.ID]
		case c.IsNo:
			no = t.byChoice[c.ID]
		default:
			yes = t.byChoice[c.ID]
		}
	}

	nonAbstain := yes + no
	quorum := int64(params.RuleChangeActivationQuorum)
	out := &types.AgendaVotes{
		AgendaID:        agendaID,
		VoteVersion:     voteVersion,
		VoteStartHeight: start,
		VoteEndHeight:   end,
		Yes:             yes,
		No:              no,
		Abstain:         abstain,
		TotalVotes:      t.total,
		Unmatched:       t.unmatched,
		Quorum:          quorum,
		QuorumMet:       nonAbstain >= quorum,
		Complete:        complete,
	}
	if nonAbstain > 0 {
		out.ApprovalRating = 100 * float64(yes) / float64(nonAbstain)
	}
	if t.total > 0 {
		out.YesShare = 100 * float64(yes) / float64(t.total)
	}
	return out, nil
}

// locateAgenda finds which stake version defines an agenda and the window it
// was voted over. dcrd answers getvoteinfo per version, so the versions are
// walked until the agenda turns up.
func locateAgenda(ctx context.Context, params *chaincfg.Params, agendaID string) (
	agenda chainjson.Agenda, voteVersion uint32, start, end int64, complete bool, err error) {

	since := map[string]int64{}
	bci, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
	if err != nil {
		return agenda, 0, 0, 0, false, fmt.Errorf("getblockchaininfo: %w", err)
	}
	for id, d := range bci.Deployments {
		since[id] = d.Since
	}

	for _, v := range voteVersionsNewestFirst(params) {
		vi, verr := rpc.DcrdClient.GetVoteInfo(ctx, v)
		if verr != nil {
			// Unrecognised versions are skipped exactly as ListAgendas does.
			continue
		}
		for _, a := range vi.Agendas {
			if a.ID != agendaID {
				continue
			}
			if a.Status == "started" {
				// A live vote is counted from the window it opened in up to
				// the tip, which is as far as votes exist.
				s, _, ok := agendaVoteWindowStarted(vi, params)
				if !ok {
					return agenda, 0, 0, 0, false, fmt.Errorf("agenda %q has no derivable window", agendaID)
				}
				return a, vi.VoteVersion, s, bci.Blocks, false, nil
			}
			s, e, ok := agendaVoteWindow(a.Status, since[a.ID], params)
			if !ok {
				return agenda, 0, 0, 0, false, fmt.Errorf("agenda %q (%s) has no derivable voting window", agendaID, a.Status)
			}
			return a, vi.VoteVersion, s, e, true, nil
		}
	}
	return agenda, 0, 0, 0, false, fmt.Errorf("agenda %q not found", agendaID)
}

// agendaVoteWindowStarted returns the current rule-change interval, which is
// the window a started agenda is being voted in. getvoteinfo reports it
// directly for the tip.
func agendaVoteWindowStarted(vi *chainjson.GetVoteInfoResult, params *chaincfg.Params) (start, end int64, ok bool) {
	if vi.StartHeight <= 0 || vi.EndHeight < vi.StartHeight {
		return 0, 0, false
	}
	return vi.StartHeight, vi.EndHeight, true
}
