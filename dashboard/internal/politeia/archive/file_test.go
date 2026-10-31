// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package archive_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/politeia/archive"
	"dcrpulse/internal/politeia/pitest"
)

func stored(t *testing.T, s *archive.Store, server pitest.Key, token string, at int64) *archive.Proposal {
	t.Helper()
	p, err := archive.New(fetched(pitest.NewProposal(server, token, pitest.Options{Name: "P " + token, Comments: 1})), server.Pub, archive.SourceRefresh, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func exported(t *testing.T, s *archive.Store) []byte {
	t.Helper()
	raw, err := json.Marshal(s.Export())
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestExportImportRoundTrip(t *testing.T) {
	server := pitest.NewKey()
	src := archive.NewStore(t.TempDir(), server.Pub)
	now := time.Now().Unix()
	stored(t, src, server, "fedcba9876543210", now-20)
	stored(t, src, server, token, now-10)

	f := src.Export()
	if f.Format != archive.FileFormat || f.SchemaVersion != archive.SchemaVersion || f.ServerKey != server.Pub ||
		len(f.Proposals) != 2 || f.Proposals[0].Token() != token {
		t.Fatalf("export: %s v%d key %s, %d proposals", f.Format, f.SchemaVersion, f.ServerKey, len(f.Proposals))
	}

	dir := t.TempDir()
	res, err := archive.NewStore(dir, server.Pub).Import(bytes.NewReader(exported(t, src)))
	if err != nil || res.Imported != 2 || res.Kept != 0 || len(res.Refused) != 0 {
		t.Fatalf("import: %+v %v", res, err)
	}
	// Reopened from disk.
	e, ok := archive.NewStore(dir, server.Pub).Get(token)
	if !ok || e.Proposal.Source != archive.SourceImport || e.Proposal.ArchivedAt != now-10 || len(e.Proposal.Comments) != 1 {
		t.Fatalf("imported: %v %+v", ok, e.Proposal)
	}
}

// An import never replaces a copy this dashboard fetched itself, whatever
// date the file claims; among imports the newer one wins.
func TestImportNeverReplacesTheDashboardsOwnCopy(t *testing.T) {
	server := pitest.NewKey()
	now := time.Now().Unix()
	dst := archive.NewStore(t.TempDir(), server.Pub)
	stored(t, dst, server, token, now-3600)

	newer := archive.NewStore(t.TempDir(), server.Pub)
	stored(t, newer, server, token, now)
	res, err := dst.Import(bytes.NewReader(exported(t, newer)))
	if err != nil || res.Kept != 1 || res.Imported != 0 {
		t.Fatalf("newer import over the dashboard's copy: %+v %v", res, err)
	}
	if e, _ := dst.Get(token); e.Proposal.ArchivedAt != now-3600 || e.Proposal.Source != archive.SourceRefresh {
		t.Fatalf("the dashboard's copy was replaced: %+v", e.Proposal)
	}
}

func TestImportReplacesOnlyAnOlderImport(t *testing.T) {
	server := pitest.NewKey()
	now := time.Now().Unix()
	dst := archive.NewStore(t.TempDir(), server.Pub)
	at := func(when int64) []byte {
		src := archive.NewStore(t.TempDir(), server.Pub)
		stored(t, src, server, token, when)
		return exported(t, src)
	}
	if res, err := dst.Import(bytes.NewReader(at(now - 7200))); err != nil || res.Imported != 1 {
		t.Fatalf("first import: %+v %v", res, err)
	}
	if res, err := dst.Import(bytes.NewReader(at(now - 3600))); err != nil || res.Imported != 1 {
		t.Fatalf("newer import: %+v %v", res, err)
	}
	if res, err := dst.Import(bytes.NewReader(at(now - 5400))); err != nil || res.Kept != 1 {
		t.Fatalf("older import: %+v %v", res, err)
	}
	if e, _ := dst.Get(token); e.Proposal.ArchivedAt != now-3600 || e.Proposal.Source != archive.SourceImport {
		t.Fatalf("held import: %+v", e.Proposal)
	}
}

// A file cannot date its copy after the import, which would keep every later
// import out.
func TestImportClampsAFutureDate(t *testing.T) {
	server := pitest.NewKey()
	src := archive.NewStore(t.TempDir(), server.Pub)
	stored(t, src, server, token, time.Now().AddDate(1, 0, 0).Unix())
	dst := archive.NewStore(t.TempDir(), server.Pub)
	if res, err := dst.Import(bytes.NewReader(exported(t, src))); err != nil || res.Imported != 1 {
		t.Fatalf("import: %+v %v", res, err)
	}
	if e, _ := dst.Get(token); e.Proposal.ArchivedAt > time.Now().Unix() {
		t.Fatalf("future date kept: %d", e.Proposal.ArchivedAt)
	}
}

func TestImportRefusesWhatDoesNotVerify(t *testing.T) {
	server := pitest.NewKey()
	src := archive.NewStore(t.TempDir(), server.Pub)
	now := time.Now().Unix()
	stored(t, src, server, token, now)
	stored(t, src, server, "fedcba9876543210", now)
	f := src.Export()
	f.Proposals[0].Comments[0].Comment = "Changed in the file."
	raw, _ := json.Marshal(f)

	dst := archive.NewStore(t.TempDir(), server.Pub)
	res, err := dst.Import(bytes.NewReader(raw))
	if err != nil || res.Imported != 1 || len(res.Refused) != 1 || res.Refused[0].Token != token || res.Refused[0].Reason == "" {
		t.Fatalf("import: %+v %v", res, err)
	}
	if _, ok := dst.Get(token); ok {
		t.Fatal("a tampered proposal was imported")
	}

	// Signed by another server: nothing verifies with the pinned key.
	other := archive.NewStore(t.TempDir(), pitest.NewKey().Pub)
	if res, err := other.Import(bytes.NewReader(exported(t, src))); err != nil || res.Imported != 0 || len(res.Refused) != 2 {
		t.Fatalf("other server: %+v %v", res, err)
	}
}

func TestImportRefusesOtherFiles(t *testing.T) {
	s := archive.NewStore(t.TempDir(), pitest.NewKey().Pub)
	for name, doc := range map[string]string{
		"not json":          `{not json`,
		"array":             `[]`,
		"other format":      `{"format":"something-else","proposals":[]}`,
		"no format":         `{"proposals":[]}`,
		"format after list": `{"proposals":[],"format":"dcrpulse-politeia-archive"}`,
		"empty object":      `{}`,
	} {
		if _, err := s.Import(strings.NewReader(doc)); err == nil {
			t.Errorf("%s: imported", name)
		}
	}
	if res, err := s.Import(strings.NewReader(`{"format":"dcrpulse-politeia-archive","schemaVersion":1,"proposals":[]}`)); err != nil || res.Imported != 0 {
		t.Fatalf("empty archive: %+v %v", res, err)
	}
}

func TestVoteCheckIsStoredButNeverImported(t *testing.T) {
	server := pitest.NewKey()
	dir := t.TempDir()
	s := archive.NewStore(dir, server.Pub)
	now := time.Now().Unix()
	held := stored(t, s, server, token, now-60)
	vc := archive.VoteCheck{Ballots: 3, Counted: 2, Counts: map[string]int64{"yes": 2}, CheckedAt: now,
		Problems: []archive.BallotProblem{{Ticket: "aa", Problem: "ticket was not eligible"}}}
	if err := s.SetVoteCheck(token, held, vc, []byte(`{"votes":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetVoteCheck("fedcba9876543210", held, vc, nil); err == nil {
		t.Fatal("a vote check was recorded on a proposal not archived")
	}
	// The copy was replaced after the check began: no result is recorded.
	other := archive.NewStore(t.TempDir(), server.Pub)
	stale := stored(t, other, server, token, now-60)
	current := stored(t, other, server, token, now)
	if err := other.SetVoteCheck(token, stale, vc, nil); err == nil {
		t.Fatal("a vote check was recorded on a copy replaced during the check")
	}
	if err := other.SetVoteCheck(token, current, vc, nil); err != nil {
		t.Fatal(err)
	}

	reopened := archive.NewStore(dir, server.Pub)
	e, _ := reopened.Get(token)
	if e.Proposal.VoteCheck == nil || e.Proposal.VoteCheck.Counted != 2 || len(e.Proposal.VoteCheck.Problems) != 1 {
		t.Fatalf("reloaded vote check: %+v", e.Proposal.VoteCheck)
	}
	path, err := reopened.VotesFile(token)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if raw, _ := os.ReadFile(path); err != nil || info.Mode().Perm() != 0o600 || string(raw) != `{"votes":[]}` {
		t.Fatalf("votes file: %v %v", info, err)
	}
	if _, err := reopened.VotesFile("../" + token); err == nil {
		t.Fatal("a path that is not a token named a votes file")
	}

	dst := archive.NewStore(t.TempDir(), server.Pub)
	if res, err := dst.Import(bytes.NewReader(exported(t, reopened))); err != nil || res.Imported != 1 {
		t.Fatalf("import: %+v %v", res, err)
	}
	if e, _ := dst.Get(token); e.Proposal.VoteCheck != nil {
		t.Fatal("an imported file's vote check was taken on trust")
	}
}
