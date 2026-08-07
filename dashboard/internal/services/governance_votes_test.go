// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// Masks and choice bits below are the real mainnet values read from
// getvoteinfo, and the expected tallies are the published results from
// voting.decred.org, not values produced by the code under test.

func mainnetChoices() []chainjson.Choice {
	return []chainjson.Choice{
		{ID: "abstain", Bits: 0, IsAbstain: true},
		{ID: "no", Bits: 2, IsNo: true},
		{ID: "yes", Bits: 4},
	}
}

// bitsFor builds a vote-bit slice holding the given per-choice counts, plus
// bit 0 set on every entry: consensus reserves it for parent-block approval, so
// real votes carry it and the mask must ignore it.
func bitsFor(yes, no, abstain int, extra ...uint16) []uint16 {
	out := make([]uint16, 0, yes+no+abstain+len(extra))
	for i := 0; i < yes; i++ {
		out = append(out, 1|4)
	}
	for i := 0; i < no; i++ {
		out = append(out, 1|2)
	}
	for i := 0; i < abstain; i++ {
		out = append(out, 1|0)
	}
	return append(out, extra...)
}

func TestCountAgendaVotesReproducesPublishedTallies(t *testing.T) {
	// The five agendas verified against voting.decred.org.
	tests := []struct {
		name             string
		yes, no, abstain int
	}{
		{"maxtreasuryspend", 24451, 3, 14188},
		{"changesubsidysplit", 23664, 61, 15621},
		{"autorevocations", 22928, 12, 16406},
		{"explicitverupgrades", 22931, 12, 16403},
		{"reverttreasurypolicy", 23082, 12, 16252},
	}

	choices := mainnetChoices()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := countAgendaVotes(bitsFor(tc.yes, tc.no, tc.abstain), 6, choices)

			if got.byChoice["yes"] != int64(tc.yes) {
				t.Errorf("yes = %d, want %d", got.byChoice["yes"], tc.yes)
			}
			if got.byChoice["no"] != int64(tc.no) {
				t.Errorf("no = %d, want %d", got.byChoice["no"], tc.no)
			}
			if got.byChoice["abstain"] != int64(tc.abstain) {
				t.Errorf("abstain = %d, want %d", got.byChoice["abstain"], tc.abstain)
			}
			if want := int64(tc.yes + tc.no + tc.abstain); got.total != want {
				t.Errorf("total = %d, want %d", got.total, want)
			}
			if got.unmatched != 0 {
				t.Errorf("unmatched = %d, want 0", got.unmatched)
			}
		})
	}
}

// Agendas of one vote version share a single 16-bit field, each isolated by its
// own mask. A vote carrying choices for several agendas at once must count
// correctly for every one of them.
func TestCountAgendaVotesIsolatesAgendasByMask(t *testing.T) {
	// One vote: yes on the mask-6 agenda, no on mask-24, abstain on mask-96.
	combined := []uint16{1 | 4 | 8}

	yesAgenda := countAgendaVotes(combined, 6, mainnetChoices())
	if yesAgenda.byChoice["yes"] != 1 {
		t.Errorf("mask 6: yes = %d, want 1", yesAgenda.byChoice["yes"])
	}

	noAgenda := countAgendaVotes(combined, 24, []chainjson.Choice{
		{ID: "abstain", Bits: 0, IsAbstain: true},
		{ID: "no", Bits: 8, IsNo: true},
		{ID: "yes", Bits: 16},
	})
	if noAgenda.byChoice["no"] != 1 {
		t.Errorf("mask 24: no = %d, want 1", noAgenda.byChoice["no"])
	}

	abstainAgenda := countAgendaVotes(combined, 96, []chainjson.Choice{
		{ID: "abstain", Bits: 0, IsAbstain: true},
		{ID: "no", Bits: 32, IsNo: true},
		{ID: "yes", Bits: 64},
	})
	if abstainAgenda.byChoice["abstain"] != 1 {
		t.Errorf("mask 96: abstain = %d, want 1", abstainAgenda.byChoice["abstain"])
	}
}

// Consensus constrains only bit 0, so a pattern matching no declared choice can
// exist on chain. dcrd's reporting path treats those as abstain; they are also
// counted separately so an unexpected number stays visible.
func TestCountAgendaVotesTreatsUnknownBitsAsAbstain(t *testing.T) {
	// 6 is the mask itself: a legal on-chain value that is not a valid choice.
	got := countAgendaVotes(bitsFor(2, 1, 1, 1|6), 6, mainnetChoices())

	if got.unmatched != 1 {
		t.Errorf("unmatched = %d, want 1", got.unmatched)
	}
	if got.byChoice["abstain"] != 2 {
		t.Errorf("abstain = %d, want 2 (1 real + 1 unmatched)", got.byChoice["abstain"])
	}
	if got.total != 5 {
		t.Errorf("total = %d, want 5", got.total)
	}
}

func TestAgendaVoteWindow(t *testing.T) {
	params := chaincfg.MainNetParams()
	rci := int64(params.RuleChangeActivationInterval) // 8064
	svh := params.StakeValidationHeight               // 4096

	tests := []struct {
		name      string
		status    string
		since     int64
		wantStart int64
		wantEnd   int64
		wantOK    bool
	}{
		{
			// maxtreasuryspend: activated at 1052416, voted 1036288-1044351.
			// Confirmed against voting.decred.org.
			name:      "active walks back two intervals",
			status:    "active",
			since:     1052416,
			wantStart: 1036288,
			wantEnd:   1044351,
			wantOK:    true,
		},
		{
			// The v9 set: activated at 657280, voted 641152-649215.
			name:      "active v9 set",
			status:    "active",
			since:     657280,
			wantStart: 641152,
			wantEnd:   649215,
			wantOK:    true,
		},
		{
			// Lock-in begins the block after voting ends, so one interval back.
			name:      "lockedin walks back one interval",
			status:    "lockedin",
			since:     1044352,
			wantStart: 1036288,
			wantEnd:   1044351,
			wantOK:    true,
		},
		{
			name:      "failed walks back one interval",
			status:    "failed",
			since:     1044352,
			wantStart: 1036288,
			wantEnd:   1044351,
			wantOK:    true,
		},
		{
			// dcrd tallies a started agenda itself; no derivation needed.
			name:   "started has no derived window",
			status: "started",
			since:  1044352,
			wantOK: false,
		},
		{
			// since is omitted from the JSON entirely for a defined agenda.
			name:   "defined has no since",
			status: "defined",
			since:  0,
			wantOK: false,
		},
		{
			// A forced choice on the test networks short-circuits since to 1.
			name:   "forced choice sentinel rejected",
			status: "active",
			since:  1,
			wantOK: false,
		},
		{
			// Off the interval grid: the state machine cannot produce this, so
			// treat it as underivable rather than reporting a wrong range.
			name:   "since off the interval grid",
			status: "active",
			since:  1052417,
			wantOK: false,
		},
		{
			name:   "window before stake validation height",
			status: "active",
			since:  svh + rci,
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := agendaVoteWindow(tc.status, tc.since, params)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (start=%d end=%d)", ok, tc.wantOK, start, end)
			}
			if !tc.wantOK {
				return
			}
			if start != tc.wantStart || end != tc.wantEnd {
				t.Errorf("window = %d-%d, want %d-%d", start, end, tc.wantStart, tc.wantEnd)
			}
			if end-start+1 != rci {
				t.Errorf("window spans %d blocks, want %d", end-start+1, rci)
			}
			if (start-svh)%rci != 0 {
				t.Errorf("start %d is not on the interval grid", start)
			}
		})
	}
}

// Every network anchors its windows on its own StakeValidationHeight and
// interval, so the derivation must track params rather than mainnet numbers.
func TestAgendaVoteWindowUsesNetworkParams(t *testing.T) {
	for _, params := range []*chaincfg.Params{
		chaincfg.MainNetParams(), chaincfg.TestNet3Params(),
		chaincfg.SimNetParams(), chaincfg.RegNetParams(),
	} {
		t.Run(params.Name, func(t *testing.T) {
			rci := int64(params.RuleChangeActivationInterval)
			svh := params.StakeValidationHeight
			// An activation three intervals past the anchor.
			since := svh + 3*rci

			start, end, ok := agendaVoteWindow("active", since, params)
			if !ok {
				t.Fatalf("no window derived for since=%d", since)
			}
			if end-start+1 != rci {
				t.Errorf("window spans %d blocks, want %d", end-start+1, rci)
			}
			if start != since-2*rci {
				t.Errorf("start = %d, want %d", start, since-2*rci)
			}
		})
	}
}
