// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package archive_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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
	stored(t, src, server, "fedcba9876543210", 10)
	stored(t, src, server, token, 20)

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
	if !ok || e.Proposal.Source != archive.SourceImport || e.Proposal.ArchivedAt != 20 || len(e.Proposal.Comments) != 1 {
		t.Fatalf("imported: %v %+v", ok, e.Proposal)
	}
}

func TestImportKeepsTheNewerCopy(t *testing.T) {
	server := pitest.NewKey()
	dst := archive.NewStore(t.TempDir(), server.Pub)
	stored(t, dst, server, token, 50)

	older := archive.NewStore(t.TempDir(), server.Pub)
	stored(t, older, server, token, 40)
	res, err := dst.Import(bytes.NewReader(exported(t, older)))
	if err != nil || res.Kept != 1 || res.Imported != 0 {
		t.Fatalf("older copy: %+v %v", res, err)
	}
	if e, _ := dst.Get(token); e.Proposal.ArchivedAt != 50 || e.Proposal.Source != archive.SourceRefresh {
		t.Fatalf("held copy replaced by an older one: %+v", e.Proposal)
	}

	newer := archive.NewStore(t.TempDir(), server.Pub)
	stored(t, newer, server, token, 60)
	res, err = dst.Import(bytes.NewReader(exported(t, newer)))
	if err != nil || res.Imported != 1 {
		t.Fatalf("newer copy: %+v %v", res, err)
	}
	if e, _ := dst.Get(token); e.Proposal.ArchivedAt != 60 {
		t.Fatalf("newer copy not stored: %+v", e.Proposal)
	}
}

func TestImportRefusesWhatDoesNotVerify(t *testing.T) {
	server := pitest.NewKey()
	src := archive.NewStore(t.TempDir(), server.Pub)
	stored(t, src, server, token, 1)
	stored(t, src, server, "fedcba9876543210", 1)
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
