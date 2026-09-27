// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// reloadGamingJournal makes the next call read the file again, as a restart
// would.
func reloadGamingJournal() {
	gamingFrameJournal.Lock()
	gamingFrameJournal.seen = nil
	gamingFrameJournal.Unlock()
}

func mustJournal(t *testing.T, gcid, from string, n byte, wantFresh bool) {
	t.Helper()
	fresh, err := appendGamingJournal(gcid, from, wireFrame(n), time.Unix(1700000000, 0))
	if err != nil || fresh != wantFresh {
		t.Fatalf("journal %s/%d = %v, %v; want fresh %v", gcid, n, fresh, err, wantFresh)
	}
}

func journalSeqs(t *testing.T, gcid string) []uint64 {
	t.Helper()
	recs, err := gamingJournalHistory(gcid)
	if err != nil {
		t.Fatal(err)
	}
	var out []uint64
	for _, rec := range recs {
		out = append(out, rec.Seq)
	}
	return out
}

func TestGamingJournalKeepsSendersAndSeqAcrossReload(t *testing.T) {
	withGamingWireDir(t)
	mustJournal(t, pruneGCA, prunePeer, 1, true)
	mustJournal(t, pruneGCB, pruneSelf, 2, true)
	mustJournal(t, pruneGCA, prunePeer, 1, false)

	reloadGamingJournal()
	mustJournal(t, pruneGCA, prunePeer, 1, false)
	mustJournal(t, pruneGCA, prunePeer, 3, true)

	recs, err := gamingJournalHistory(pruneGCA)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].Seq != 1 || recs[1].Seq != 3 ||
		recs[0].From != prunePeer || recs[1].Message != wireFrame(3) || recs[0].TS != 1700000000 {
		t.Fatalf("history = %+v", recs)
	}
}

func TestGamingJournalDropsTornTail(t *testing.T) {
	withGamingWireDir(t)
	mustJournal(t, pruneGCA, prunePeer, 1, true)
	path := filepath.Join(GamingStateDir, gamingJournalFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"seq":2,"gcid":"` + pruneGCA); err != nil {
		t.Fatal(err)
	}
	f.Close()

	reloadGamingJournal()
	mustJournal(t, pruneGCA, prunePeer, 2, true)
	if got := journalSeqs(t, pruneGCA); len(got) != 2 || got[1] != 2 {
		t.Fatalf("seqs after torn tail = %v", got)
	}
	raw, _ := os.ReadFile(path)
	if strings.Count(string(raw), "\n") != 2 || !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("journal after heal = %q", raw)
	}
}

func TestGamingJournalRefusesDamageUntilRepaired(t *testing.T) {
	withGamingWireDir(t)
	mustJournal(t, pruneGCA, prunePeer, 1, true)
	path := filepath.Join(GamingStateDir, gamingJournalFile)
	good, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append([]byte("not json\n"), good...), 0o600); err != nil {
		t.Fatal(err)
	}
	reloadGamingJournal()
	if _, err := appendGamingJournal(pruneGCA, prunePeer, wireFrame(2), time.Now()); err == nil {
		t.Fatal("appended to a damaged journal")
	}
	if _, err := gamingJournalHistory(pruneGCA); err == nil {
		t.Fatal("read a damaged journal")
	}
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	mustJournal(t, pruneGCA, prunePeer, 2, true)

	// Records out of order are damage too.
	ordered, _ := os.ReadFile(path)
	lines := strings.SplitAfter(string(ordered), "\n")
	if err := os.WriteFile(path, []byte(lines[1]+lines[0]), 0o600); err != nil {
		t.Fatal(err)
	}
	reloadGamingJournal()
	if _, err := gamingJournalHistory(pruneGCA); err == nil {
		t.Fatal("read a journal whose sequence goes backwards")
	}
}

func TestGamingJournalPruneKeepsOthersAndSeq(t *testing.T) {
	withGamingWireDir(t)
	mustJournal(t, pruneGCA, prunePeer, 1, true)
	mustJournal(t, pruneGCB, prunePeer, 2, true)
	mustJournal(t, pruneGCA, prunePeer, 3, true)

	removed, err := pruneGamingJournal(pruneGCA)
	if err != nil || removed != 2 {
		t.Fatalf("prune = %d, %v", removed, err)
	}
	if removed, err = pruneGamingJournal(pruneGCA); err != nil || removed != 0 {
		t.Fatalf("second prune = %d, %v", removed, err)
	}
	if got := journalSeqs(t, pruneGCB); len(got) != 1 || got[0] != 2 {
		t.Fatalf("other group after prune = %v", got)
	}
	// A pruned frame is no longer a duplicate, in this run or the next.
	mustJournal(t, pruneGCA, prunePeer, 1, true)
	if _, err := pruneGamingJournal(pruneGCA); err != nil {
		t.Fatal(err)
	}

	reloadGamingJournal()
	mustJournal(t, pruneGCA, prunePeer, 1, true)
	mustJournal(t, pruneGCB, prunePeer, 4, true)
	if got := journalSeqs(t, pruneGCB); len(got) != 2 || got[1] != 6 {
		t.Fatalf("seq after prune and reload = %v", got)
	}
}
