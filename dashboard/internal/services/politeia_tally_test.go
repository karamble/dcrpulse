// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"dcrpulse/internal/types"
)

func seedVotingList(t *testing.T, rows ...types.Proposal) {
	t.Helper()
	resetPiListCache(t)
	piCacheMu.Lock()
	piCachedLists["voting"] = piListCacheEntry{list: rows, at: time.Now()}
	piCacheMu.Unlock()
}

// A list handed to a reader is encoded without the lock, so a cast that lands
// meanwhile must leave that list and its tally maps as they were and publish
// the new tally as a new list.
func TestVoteTallyBumpLeavesAPublishedListAlone(t *testing.T) {
	seedVotingList(t,
		types.Proposal{Token: "t0", VoteCounts: map[string]int64{"yes": 1}, TotalVotes: 1},
		types.Proposal{Token: "t1", VoteCounts: map[string]int64{"yes": 5, "no": 2}, TotalVotes: 7},
	)
	ctx := context.Background()
	held, _, err := ListProposals(ctx, "voting")
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}

	bumpCachedVoteTally(ctx, "t1", "no", 3)

	if held[1].VoteCounts["no"] != 2 || held[1].TotalVotes != 7 || held[1].CurrentChoice != "" {
		t.Errorf("the list a reader holds changed: %+v", held[1])
	}
	now, _, _ := ListProposals(ctx, "voting")
	if now[1].VoteCounts["no"] != 5 || now[1].VoteCounts["yes"] != 5 || now[1].TotalVotes != 10 ||
		now[1].CurrentChoice != "no" || now[1].VotedTicketCount != 3 {
		t.Errorf("the published list lacks the bump: %+v", now[1])
	}
	if now[0].VoteCounts["yes"] != 1 {
		t.Errorf("an untouched row changed: %+v", now[0])
	}
}

// The runtime kills the process on a map written while another goroutine
// iterates it; under -race this reports the race outright.
func TestVoteTallyBumpWhileEncoding(t *testing.T) {
	seedVotingList(t, types.Proposal{Token: "t1", VoteCounts: map[string]int64{"yes": 1}})
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			list, _, _ := ListProposals(ctx, "voting")
			if _, err := json.Marshal(list); err != nil {
				t.Errorf("marshal: %v", err)
				return
			}
		}
	}()
	for i := 0; i < 300; i++ {
		bumpCachedVoteTally(ctx, "t1", "yes", 1)
	}
	wg.Wait()
}
