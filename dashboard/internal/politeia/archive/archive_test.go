// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package archive_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"dcrpulse/internal/politeia/archive"
	"dcrpulse/internal/politeia/pitest"
	"dcrpulse/internal/politeia/verify"
)

const token = "0123456789abcdef"

func fetched(p *pitest.Proposal) archive.Fetched {
	return archive.Fetched{
		Record: p.Record, Comments: p.Comments, Auths: p.Auths, Vote: p.Vote, Billing: p.Billing,
		Summary: archive.Summary{Status: "approved", Results: []archive.Result{{ID: "yes", VoteBit: 2, Votes: 9000}}},
	}
}

func TestNewKeepsWhatVerifiesAndStripsTheRest(t *testing.T) {
	server := pitest.NewKey()
	p := pitest.NewProposal(server, token, pitest.Options{Name: "Marketing", Comments: 3})
	got, err := archive.New(fetched(p), server.Pub, archive.SourceRefresh, 1800000000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Comments) != 3 || len(got.Auths) != 1 || got.Vote == nil || len(got.Billing) != 1 {
		t.Fatalf("kept %d comments, %d auths, vote %v, %d billing changes",
			len(got.Comments), len(got.Auths), got.Vote != nil, len(got.Billing))
	}
	for _, f := range got.Record.Files {
		if (f.Name == "diagram.png") != (f.Payload == "") {
			t.Fatalf("file %s payload kept=%v", f.Name, f.Payload != "")
		}
	}
	if got.Vote.EligibleTickets != nil {
		t.Fatal("eligible tickets were archived")
	}
	if got.Source != archive.SourceRefresh || got.ArchivedAt != 1800000000 || got.Summary.Status != "approved" {
		t.Fatalf("source %q at %d status %q", got.Source, got.ArchivedAt, got.Summary.Status)
	}
	ck, err := archive.Verify(got, server.Pub)
	if err != nil || ck.Migrated() || ck.Author == nil || ck.Metadata.Name != "Marketing" {
		t.Fatalf("verify: %+v %v", ck, err)
	}
}

func TestNewLeavesOutFailingPartsAndRefusesFailingRecordOrVote(t *testing.T) {
	server := pitest.NewKey()
	p := pitest.NewProposal(server, token, pitest.Options{Name: "P", Comments: 3})
	p.Comments[1].Comment = "Edited after signing."
	p.Billing[0].Status = 1
	got, err := archive.New(fetched(p), server.Pub, archive.SourceSnapshot, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Comments) != 2 || len(got.DroppedComments) != 1 || got.DroppedComments[0] != 2 {
		t.Fatalf("comments %d dropped %v", len(got.Comments), got.DroppedComments)
	}
	if len(got.Billing) != 0 {
		t.Fatal("a changed billing status was kept")
	}

	bad := pitest.NewProposal(server, token, pitest.Options{Name: "P"})
	bad.Record.Files[0].Payload = base64.StdEncoding.EncodeToString([]byte("# Other text"))
	if _, err := archive.New(fetched(bad), server.Pub, archive.SourceSnapshot, 1); err == nil {
		t.Fatal("a changed text was archived")
	}
	bad = pitest.NewProposal(server, token, pitest.Options{Name: "P"})
	bad.Vote.Params.PassPercentage = 50
	if _, err := archive.New(fetched(bad), server.Pub, archive.SourceSnapshot, 1); err == nil {
		t.Fatal("a changed vote start was archived")
	}
	if _, err := archive.New(fetched(p), pitest.NewKey().Pub, archive.SourceSnapshot, 1); err == nil {
		t.Fatal("a proposal signed by another server was archived")
	}
	// The digest alone still verifies against the merkle root, but there
	// would be no text to read.
	textless := pitest.NewProposal(server, token, pitest.Options{Name: "P"})
	textless.Record.Files[0].Payload = ""
	if _, err := archive.New(fetched(textless), server.Pub, archive.SourceSnapshot, 1); err == nil {
		t.Fatal("a proposal without its text was archived")
	}
	other := pitest.NewProposal(server, "fedcba9876543210", pitest.Options{Name: "Q"})
	mixed := fetched(p)
	mixed.Vote = other.Vote
	if _, err := archive.New(mixed, server.Pub, archive.SourceSnapshot, 1); err == nil {
		t.Fatal("another proposal's vote start was archived")
	}
}

// A proposal migrated off Politeia's old backend keeps only its text, which
// today's server signed; its author signature was blanked.
func TestMigratedProposalKeepsOnlyItsText(t *testing.T) {
	server := pitest.NewKey()
	p := pitest.NewProposal(server, token, pitest.Options{Name: "Old", LegacyToken: "27f87171d98b7923a1bd2bee6affed929fa2d2a6e178b5c80a9971a92a5c7f50"})
	f := fetched(p)
	f.Comments = pitest.NewProposal(server, token, pitest.Options{Name: "Old", Comments: 2}).Comments
	got, err := archive.New(f, server.Pub, archive.SourceSnapshot, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Comments != nil || got.Auths != nil || got.Vote != nil || got.Billing != nil {
		t.Fatal("a migrated proposal kept more than its text")
	}
	ck, err := archive.Verify(got, server.Pub)
	if err != nil || !ck.Migrated() || ck.Author != nil {
		t.Fatalf("verify: %+v %v", ck, err)
	}
	got.Comments = f.Comments
	if _, err := archive.Verify(got, server.Pub); err == nil {
		t.Fatal("a migrated proposal carrying comments verified")
	}
}

func TestStoreKeepsVerifiedProposalsOnDisk(t *testing.T) {
	server := pitest.NewKey()
	dir := filepath.Join(t.TempDir(), "politeia")
	p, err := archive.New(fetched(pitest.NewProposal(server, token, pitest.Options{Name: "Kept", Comments: 1})), server.Pub, archive.SourceRefresh, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.NewStore(dir, server.Pub).Put(p); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, token+".json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("archive file: %v %v", info, err)
	}

	reopened := archive.NewStore(dir, server.Pub)
	e, ok := reopened.Get(token)
	if !ok || e.Checked.Metadata.Name != "Kept" || len(e.Proposal.Comments) != 1 {
		t.Fatalf("reloaded: %v %+v", ok, e)
	}
	if len(reopened.All()) != 1 {
		t.Fatal("All does not list the archived proposal")
	}
	if _, ok := archive.NewStore(dir, pitest.NewKey().Pub).Get(token); ok {
		t.Fatal("a file signed by another server loaded")
	}
}

func TestStoreLeavesOutTamperedAndUnreadableFiles(t *testing.T) {
	server := pitest.NewKey()
	dir := t.TempDir()
	p, err := archive.New(fetched(pitest.NewProposal(server, token, pitest.Options{Name: "Real", Comments: 1})), server.Pub, archive.SourceRefresh, 1)
	if err != nil {
		t.Fatal(err)
	}
	p.Comments[0].Comment = "Changed on disk."
	raw, _ := json.Marshal(p)
	if err := os.WriteFile(filepath.Join(dir, token+".json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fedcba9876543210.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := archive.NewStore(dir, server.Pub)
	if len(s.All()) != 0 {
		t.Fatal("a tampered or unreadable file was loaded")
	}

	good, _ := archive.New(fetched(pitest.NewProposal(server, token, pitest.Options{Name: "Real"})), server.Pub, archive.SourceRefresh, 2)
	if _, err := s.Put(good); err != nil {
		t.Fatal(err)
	}
	if e, ok := archive.NewStore(dir, server.Pub).Get(token); !ok || e.Proposal.ArchivedAt != 2 {
		t.Fatal("a verified fetch did not replace the tampered file")
	}
}

func TestStoreRefusesUnverifiedOrMisnamedProposals(t *testing.T) {
	server := pitest.NewKey()
	s := archive.NewStore(t.TempDir(), server.Pub)
	p, _ := archive.New(fetched(pitest.NewProposal(server, token, pitest.Options{Name: "P", Comments: 1})), server.Pub, archive.SourceImport, 1)
	p.Comments[0].Receipt = pitest.NewKey().Sign(p.Comments[0].Signature)
	if _, err := s.Put(p); err == nil {
		t.Fatal("a proposal with a forged receipt was stored")
	}
	q := &archive.Proposal{SchemaVersion: archive.SchemaVersion, Record: verify.Record{CensorshipRecord: verify.CensorshipRecord{Token: "../escape"}}}
	if _, err := s.Put(q); err == nil {
		t.Fatal("a token that is not hex was used as a file name")
	}
}
