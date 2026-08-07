// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/v3"
)

// Expected values below are computed by hand from dcrd's checkTSpendHasVotes
// (internal/blockchain/treasury.go), not from the implementation under test.

func TestCalcTSpendWindowMatchesNetworkParams(t *testing.T) {
	tests := []struct {
		name       string
		params     *chaincfg.Params
		wantLength uint32
	}{
		{"mainnet", chaincfg.MainNetParams(), 288 * 12},
		{"testnet3", chaincfg.TestNet3Params(), 60 * 4},
		{"simnet", chaincfg.SimNetParams(), 48 * 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tvi := tc.params.TreasuryVoteInterval
			mul := tc.params.TreasuryVoteIntervalMultiplier

			// Pick an expiry well past a single window that sits two above a TVI,
			// which is the only shape dcrd accepts.
			expiry := uint32(tvi*mul*3 + 2)
			start, end, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
			if err != nil {
				t.Fatalf("CalcTSpendWindow(%d): %v", expiry, err)
			}
			if got := end - start; got != tc.wantLength {
				t.Errorf("window length = %d, want %d", got, tc.wantLength)
			}
			if end != expiry-2 {
				t.Errorf("end = %d, want expiry-2 = %d", end, expiry-2)
			}
		})
	}
}

func TestEvalTSpendVotesMainnetThresholds(t *testing.T) {
	params := chaincfg.MainNetParams()
	// Mainnet window: 288 * 12 = 3456 blocks, 5 tickets each.
	const (
		voteStart = int64(1_000_000)
		voteEnd   = voteStart + 3456
		maxVotes  = 3456 * 5 // 17280
		quorum    = maxVotes / 5
	)

	tests := []struct {
		name           string
		countedThrough int64
		yes, no        int64
		wantRequired   int64
		wantQuorum     bool
		wantApproved   bool
	}{
		{
			// Window closed, comfortably over both bars: required degenerates to
			// 60% of the cast votes.
			name:           "closed window approved",
			countedThrough: voteEnd - 1,
			yes:            7000,
			no:             1000,
			wantRequired:   4800,
			wantQuorum:     true,
			wantApproved:   true,
		},
		{
			// Exactly 60% yes passes: the comparison is inclusive.
			name:           "closed window exactly at threshold",
			countedThrough: voteEnd - 1,
			yes:            6000,
			no:             4000,
			wantRequired:   6000,
			wantQuorum:     true,
			wantApproved:   true,
		},
		{
			name:           "closed window one yes short",
			countedThrough: voteEnd - 1,
			yes:            5999,
			no:             4001,
			wantRequired:   6000,
			wantQuorum:     true,
			wantApproved:   false,
		},
		{
			// Cast votes exactly meet quorum.
			name:           "quorum exactly met",
			countedThrough: voteEnd - 1,
			yes:            3456,
			no:             0,
			wantRequired:   2073, // 3456 * 3 / 5, truncated
			wantQuorum:     true,
			wantApproved:   true,
		},
		{
			name:           "one vote below quorum",
			countedThrough: voteEnd - 1,
			yes:            3455,
			no:             0,
			wantRequired:   2073,
			wantQuorum:     false,
			wantApproved:   false,
		},
		{
			// Mid-window: the 1000 blocks left are treated as future no votes,
			// so the bar is 60% of (cast + 5000), far above 60% of cast.
			name:           "mid window cannot short circuit",
			countedThrough: voteEnd - 1001,
			yes:            9000,
			no:             1000,
			wantRequired:   9000, // (10000 + 5000) * 3 / 5
			wantQuorum:     true,
			wantApproved:   true,
		},
		{
			name:           "mid window just short of short circuit",
			countedThrough: voteEnd - 1001,
			yes:            8999,
			no:             1000,
			wantRequired:   8999, // (9999 + 5000) * 3 / 5 = 8999 (truncated)
			wantQuorum:     true,
			wantApproved:   true,
		},
		{
			// A stale mined height past the window must not underflow the
			// remaining-block count into a huge required figure.
			name:           "counted past window end clamps remaining",
			countedThrough: voteEnd + 500,
			yes:            7000,
			no:             1000,
			wantRequired:   4800,
			wantQuorum:     true,
			wantApproved:   true,
		},
		{
			name:           "no votes at all",
			countedThrough: voteEnd - 1,
			yes:            0,
			no:             0,
			wantRequired:   0,
			wantQuorum:     false,
			wantApproved:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := evalTSpendVotes(params, voteStart, voteEnd, tc.countedThrough, tc.yes, tc.no)

			if got.MaxVotes != maxVotes {
				t.Errorf("MaxVotes = %d, want %d", got.MaxVotes, maxVotes)
			}
			if got.QuorumRequired != quorum {
				t.Errorf("QuorumRequired = %d, want %d", got.QuorumRequired, quorum)
			}
			if got.VotesCast != tc.yes+tc.no {
				t.Errorf("VotesCast = %d, want %d", got.VotesCast, tc.yes+tc.no)
			}
			if got.RequiredVotes != tc.wantRequired {
				t.Errorf("RequiredVotes = %d, want %d", got.RequiredVotes, tc.wantRequired)
			}
			if got.QuorumAchieved != tc.wantQuorum {
				t.Errorf("QuorumAchieved = %v, want %v", got.QuorumAchieved, tc.wantQuorum)
			}
			if got.Approved != tc.wantApproved {
				t.Errorf("Approved = %v, want %v", got.Approved, tc.wantApproved)
			}
		})
	}
}

func TestEvalTSpendVotesQuorumRatioPerNetwork(t *testing.T) {
	// All three networks use a 20% quorum and a 60% yes requirement; only the
	// window length differs, so the derived figures must track the params
	// rather than any hardcoded mainnet number.
	tests := []struct {
		name         string
		params       *chaincfg.Params
		wantMaxVotes int64
		wantQuorum   int64
	}{
		{"mainnet", chaincfg.MainNetParams(), 3456 * 5, 3456},
		{"testnet3", chaincfg.TestNet3Params(), 240 * 5, 240},
		{"simnet", chaincfg.SimNetParams(), 144 * 5, 144},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window := int64(tc.params.TreasuryVoteInterval * tc.params.TreasuryVoteIntervalMultiplier)
			const voteStart = int64(500_000)
			voteEnd := voteStart + window

			got := evalTSpendVotes(tc.params, voteStart, voteEnd, voteEnd-1, 0, 0)
			if got.MaxVotes != tc.wantMaxVotes {
				t.Errorf("MaxVotes = %d, want %d", got.MaxVotes, tc.wantMaxVotes)
			}
			if got.QuorumRequired != tc.wantQuorum {
				t.Errorf("QuorumRequired = %d, want %d", got.QuorumRequired, tc.wantQuorum)
			}
		})
	}
}

func TestTSpendCountHeights(t *testing.T) {
	const (
		voteEnd     = int64(1_003_456)
		minedHeight = int64(1_002_000)
	)

	tests := []struct {
		name          string
		tip           int64
		mined         bool
		wantQuery     int64
		wantExplicit  bool
		wantCountedTo int64
	}{
		{
			// Window still open: dcrd's default (best block) is already correct,
			// so no block hash lookup is needed.
			name:          "in flight uses tip implicitly",
			tip:           voteEnd - 500,
			wantQuery:     voteEnd - 500,
			wantExplicit:  false,
			wantCountedTo: voteEnd - 500,
		},
		{
			// Window closed: must name a block inside it, and never voteEnd.
			name:          "closed window names last countable block",
			tip:           voteEnd + 10_000,
			wantQuery:     voteEnd - 1,
			wantExplicit:  true,
			wantCountedTo: voteEnd - 1,
		},
		{
			name:          "tip exactly at window end still steps back",
			tip:           voteEnd,
			wantQuery:     voteEnd - 1,
			wantExplicit:  true,
			wantCountedTo: voteEnd - 1,
		},
		{
			// Mined spends are tallied only up to the block before mining,
			// regardless of which block we asked about.
			name:          "mined counts through block before mining",
			tip:           voteEnd + 10_000,
			mined:         true,
			wantQuery:     voteEnd - 1,
			wantExplicit:  true,
			wantCountedTo: minedHeight - 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query, explicit, countedTo := tspendCountHeights(tc.tip, voteEnd, minedHeight, tc.mined)
			if query != tc.wantQuery {
				t.Errorf("queryHeight = %d, want %d", query, tc.wantQuery)
			}
			if explicit != tc.wantExplicit {
				t.Errorf("explicitBlock = %v, want %v", explicit, tc.wantExplicit)
			}
			if countedTo != tc.wantCountedTo {
				t.Errorf("countedThrough = %d, want %d", countedTo, tc.wantCountedTo)
			}
			if query == voteEnd {
				t.Error("queryHeight must never be voteEnd: dcrd rejects it")
			}
		})
	}
}
