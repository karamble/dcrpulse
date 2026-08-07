// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
)

// The expected maxima come from chaincfg's own deployment maps. If a chaincfg
// bump adds a stake version these assertions fail, which is the point: the
// dashboard then starts querying the new version and that should be a
// deliberate, visible change rather than a silent one.
func TestVoteVersionsNewestFirst(t *testing.T) {
	tests := []struct {
		name    string
		params  *chaincfg.Params
		wantTop uint32
	}{
		{"mainnet", chaincfg.MainNetParams(), 11},
		{"testnet3", chaincfg.TestNet3Params(), 12},
		{"simnet", chaincfg.SimNetParams(), 12},
		{"regnet", chaincfg.RegNetParams(), 12},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := voteVersionsNewestFirst(tc.params)

			if len(got) == 0 {
				t.Fatal("no vote versions returned")
			}
			if len(got) != len(tc.params.Deployments) {
				t.Errorf("returned %d versions, params define %d",
					len(got), len(tc.params.Deployments))
			}
			if got[0] != tc.wantTop {
				t.Errorf("newest version = %d, want %d", got[0], tc.wantTop)
			}

			for i := 1; i < len(got); i++ {
				if got[i] >= got[i-1] {
					t.Fatalf("not strictly descending at %d: %v", i, got)
				}
			}

			// Every returned version must be a real deployment key, so the
			// step-down never wastes a round trip on a version dcrd is certain
			// to reject.
			for _, v := range got {
				if _, ok := tc.params.Deployments[v]; !ok {
					t.Errorf("version %d is not a deployment key", v)
				}
			}
		})
	}
}

// The networks do not share a key set, so the list must be built from the map
// rather than assumed to be a contiguous range from a fixed origin.
func TestVoteVersionsCoverNetworkSpecificKeys(t *testing.T) {
	mainnet := voteVersionsNewestFirst(chaincfg.MainNetParams())
	testnet := voteVersionsNewestFirst(chaincfg.TestNet3Params())

	has := func(vs []uint32, want uint32) bool {
		for _, v := range vs {
			if v == want {
				return true
			}
		}
		return false
	}

	// Mainnet defines version 4; testnet3 starts at 5.
	if !has(mainnet, 4) {
		t.Errorf("mainnet should define version 4: %v", mainnet)
	}
	if has(testnet, 4) {
		t.Errorf("testnet3 should not define version 4: %v", testnet)
	}
}
