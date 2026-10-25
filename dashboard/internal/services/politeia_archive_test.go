// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/politeia/archive"
	"dcrpulse/internal/politeia/pitest"
	"dcrpulse/internal/types"
)

const (
	arApproved = "1111111111111111"
	arRejected = "2222222222222222"
	arAbandon  = "3333333333333333"
	arMigrated = "4444444444444444"
)

// useArchive points the services at an empty archive verified with a test
// server key.
func useArchive(t *testing.T) (*archive.Store, pitest.Key) {
	t.Helper()
	server := pitest.NewKey()
	s := archive.NewStore(t.TempDir(), server.Pub)
	prev := piArchiveStore.Swap(s)
	t.Cleanup(func() { piArchiveStore.Store(prev) })
	return s, server
}

func archiveFetched(p *pitest.Proposal, status string, endBlock int64) archive.Fetched {
	return archive.Fetched{
		Record: p.Record, Comments: p.Comments, Auths: p.Auths, Vote: p.Vote, Billing: p.Billing,
		Summary: archive.Summary{
			Status:           status,
			Results:          []archive.Result{{ID: "no", VoteBit: 1, Votes: 1000}, {ID: "yes", VoteBit: 2, Votes: 9000}},
			EligibleTickets:  40000,
			QuorumPercentage: 20,
			PassPercentage:   60,
			EndBlockHeight:   endBlock,
		},
	}
}

func putArchived(t *testing.T, s *archive.Store, server pitest.Key, f archive.Fetched) {
	t.Helper()
	p, err := archive.New(f, server.Pub, archive.SourceSnapshot, 1800000000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(p); err != nil {
		t.Fatal(err)
	}
}

// seedArchive stores one proposal of each finished kind.
func seedArchive(t *testing.T) (*archive.Store, pitest.Key) {
	t.Helper()
	s, server := useArchive(t)
	putArchived(t, s, server, archiveFetched(pitest.NewProposal(server, arApproved, pitest.Options{Name: "Approved"}), "approved", 800))
	putArchived(t, s, server, archiveFetched(pitest.NewProposal(server, arRejected, pitest.Options{Name: "Rejected"}), "rejected", 900))
	putArchived(t, s, server, archiveFetched(pitest.NewProposal(server, arAbandon, pitest.Options{Name: "Abandoned"}), "ineligible", 700))
	legacy := pitest.NewProposal(server, arMigrated, pitest.Options{Name: "Migrated", LegacyToken: "27f87171d98b7923a1bd2bee6affed929fa2d2a6e178b5c80a9971a92a5c7f50"})
	putArchived(t, s, server, archiveFetched(legacy, "approved", 600))
	return s, server
}

func politeiaOff(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(pointExternalCfg(t), []byte(`{"allowed_external_requests":{"politeia":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func tokensOf(ps []types.Proposal) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Token
	}
	return out
}

func TestArchivedListsServedWhilePoliteiaIsOff(t *testing.T) {
	politeiaOff(t)
	resetPiListCache(t)
	seedArchive(t)
	ctx := context.Background()

	list, _, err := ListProposals(ctx, "finished")
	var ao *ArchiveOnlyError
	if !errors.As(err, &ao) || ao.Reason() != "disabled" {
		t.Fatalf("finished bucket: err %v", err)
	}
	if got, want := tokensOf(list), []string{arRejected, arApproved, arMigrated}; !slices.Equal(got, want) {
		t.Fatalf("finished bucket = %v, want %v newest vote first", got, want)
	}
	p := list[1]
	if !p.Archived || p.Name != "Approved" || p.Status != "finished" || p.VoteStatus != "approved" ||
		p.VoteCounts["yes"] != 9000 || p.TotalVotes != 10000 || p.QuorumMin != 8000 || p.EndBlock != 800 {
		t.Fatalf("archived row: %+v", p)
	}

	list, _, err = ListProposals(ctx, "abandoned")
	if !errors.As(err, &ao) || len(list) != 1 || list[0].Token != arAbandon ||
		list[0].Status != "abandoned" || list[0].VoteStatus != "ineligible" {
		t.Fatalf("abandoned bucket: %v %v", tokensOf(list), err)
	}

	list, _, err = ListProposals(ctx, "voting")
	if !errors.Is(err, ErrPoliteiaDisabled) || errors.As(err, &ao) || list != nil {
		t.Fatalf("voting bucket with nothing archived: %v %v", list, err)
	}
}

func TestArchivedListsServedWhenPoliteiaFails(t *testing.T) {
	pointExternalCfg(t)
	resetPiListCache(t)
	seedArchive(t)
	piRouter(t, map[string]http.HandlerFunc{
		"/api/ticketvote/v1/inventory": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		},
	})
	ctx := context.Background()

	// The second read meets the failed-fetch backoff, not Politeia.
	for range 2 {
		list, _, err := ListProposals(ctx, "abandoned")
		var ao *ArchiveOnlyError
		if !errors.As(err, &ao) || ao.Reason() != "unreachable" || len(list) != 1 {
			t.Fatalf("abandoned bucket: %v %v", tokensOf(list), err)
		}
	}
	if _, _, err := ListProposals(ctx, "voting"); err == nil || errors.As(err, new(*ArchiveOnlyError)) {
		t.Fatalf("voting bucket with nothing archived: %v", err)
	}
}

func TestLiveListMarksAndAddsArchived(t *testing.T) {
	pointExternalCfg(t)
	resetPiListCache(t)
	seedArchive(t)
	const live = "aaaaaaaaaaaaaaaa"
	piRouter(t, map[string]http.HandlerFunc{
		"/api/ticketvote/v1/inventory": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"vetted":{"approved":["` + live + `","` + arApproved + `"]},"bestblock":100}`))
		},
		"/api/ticketvote/v1/summaries": piSummariesHandler(""),
		"/api/records/v1/records":      piRecordsHandler(""),
	})
	ctx := context.Background()

	// The second read is served from the list cache.
	for range 2 {
		list, _, err := ListProposals(ctx, "finished")
		if err != nil {
			t.Fatal(err)
		}
		if got, want := tokensOf(list), []string{arRejected, arMigrated, live, arApproved}; !slices.Equal(got, want) {
			t.Fatalf("merged list = %v, want %v", got, want)
		}
		if list[2].Archived || !list[3].Archived || list[3].Username != "alice" || !list[0].Archived {
			t.Fatalf("archived marks: %+v", list)
		}
	}
}

func TestArchivedDetailCarriesItsVerification(t *testing.T) {
	politeiaOff(t)
	s, server := seedArchive(t)
	ctx := context.Background()

	const tok = "5555555555555555"
	p := pitest.NewProposal(server, tok, pitest.Options{Name: "Commented", Comments: 3})
	p.Comments[2].Comment = "Edited after signing."
	f := archiveFetched(p, "approved", 500)
	slices.Reverse(f.Summary.Results)
	putArchived(t, s, server, f)

	d, at, err := GetProposalDetail(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Description, "The proposal text.") || !strings.Contains(d.DescriptionHTML, "The proposal text.") {
		t.Fatalf("description %q html %q", d.Description, d.DescriptionHTML)
	}
	if !at.Equal(time.Unix(1800000000, 0)) || !d.Archived || d.Name != "Commented" {
		t.Fatalf("at %v archived %v name %q", at, d.Archived, d.Name)
	}
	// The signed vote start's options, not the reported summary's order.
	if len(d.VoteOptions) != 2 || d.VoteOptions[1] != (types.ProposalVoteOption{ID: "yes", Bit: 2}) {
		t.Fatalf("vote options %+v", d.VoteOptions)
	}
	if len(d.Comments) != 2 {
		t.Fatalf("%d comments, want the 2 that verified", len(d.Comments))
	}
	want := types.ProposalVerification{
		ServerKey: server.Pub, AuthorKey: p.Author.Pub, Comments: 2, CommentsLeftOut: 1,
		Authorized: true, VoteStartKey: p.Admin.Pub, BillingStatus: "completed",
		Source: archive.SourceSnapshot, ArchivedAt: 1800000000,
	}
	if d.Verification == nil || *d.Verification != want {
		t.Fatalf("verification %+v, want %+v", d.Verification, want)
	}

	d, _, err = GetProposalDetail(ctx, arMigrated)
	if err != nil {
		t.Fatal(err)
	}
	v := d.Verification
	if !v.Migrated || v.LegacyToken == "" || v.AuthorKey != "" || v.VoteStartKey != "" || v.Authorized || v.BillingStatus != "" {
		t.Fatalf("migrated verification %+v", v)
	}
	// Without a signed vote start the options come from the reported summary.
	if len(d.VoteOptions) != 2 || d.VoteOptions[0] != (types.ProposalVoteOption{ID: "no", Bit: 1}) {
		t.Fatalf("migrated vote options %+v", d.VoteOptions)
	}

	if _, _, err := GetProposalDetail(ctx, "6666666666666666"); !errors.Is(err, ErrPoliteiaDisabled) {
		t.Fatalf("unarchived proposal with Politeia off: %v", err)
	}
}

// Politeia has no abandoned vote status: abandoned proposals are ineligible,
// and "finished" is a vote status of its own.
func TestBucketStatusesRoundTrip(t *testing.T) {
	want := map[string][]string{
		"voting":    {"started"},
		"pre-vote":  {"authorized", "unauthorized"},
		"finished":  {"approved", "rejected", "finished"},
		"abandoned": {"ineligible"},
	}
	for bucket, statuses := range want {
		if got := bucketStatuses(bucket); !slices.Equal(got, statuses) {
			t.Errorf("bucket %s = %v, want %v", bucket, got, statuses)
		}
		for _, st := range statuses {
			if summaryStatusToBucket(st) != bucket {
				t.Errorf("status %s maps to %s, want %s", st, summaryStatusToBucket(st), bucket)
			}
			if _, ok := piVoteStatusEnum[st]; !ok {
				t.Errorf("status %s has no inventory enum", st)
			}
		}
	}
}
