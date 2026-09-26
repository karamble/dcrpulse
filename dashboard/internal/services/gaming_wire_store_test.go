package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withGamingWireDir(t *testing.T) {
	t.Helper()
	old := GamingStateDir
	GamingStateDir = t.TempDir()
	t.Cleanup(func() { GamingStateDir = old })
}

func TestGamingInboxPersistsDeduplicatesAndReplays(t *testing.T) {
	withGamingWireDir(t)
	b := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	ev := GamingFrameEvent{Game: "poker", GCID: "table", From: "alice", Frame: testFrame}
	seq, fresh, err := b.persistGamingFrame(ev)
	if err != nil || !fresh || seq != 1 {
		t.Fatalf("first persist = %d, %v, %v", seq, fresh, err)
	}
	if _, fresh, err = b.persistGamingFrame(ev); err != nil || fresh {
		t.Fatalf("duplicate persist = %v, %v", fresh, err)
	}

	// A new bus models a bridge process restart and must load the same record.
	restarted := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	ch, cancel := restarted.SubscribeFrom("poker", 0, 1)
	defer cancel()
	got := <-ch
	if got.Seq != 1 || got.Frame != testFrame || got.From != "alice" {
		t.Fatalf("replayed %+v", got)
	}
	if st, err := os.Stat(filepath.Join(GamingStateDir, gamingInboxFile)); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("inbox permissions: %v, %v", st, err)
	}
}

func TestGamingOutboxClaimsEachWirePartOnce(t *testing.T) {
	withGamingWireDir(t)
	frame, ok := parseGamingFrame(testFrame)
	if !ok {
		t.Fatal("test frame did not parse")
	}
	fresh, err := claimGamingFrameSend("poker", "table", frame, testFrame)
	if err != nil || !fresh {
		t.Fatalf("first claim = %v, %v", fresh, err)
	}
	fresh, err = claimGamingFrameSend("poker", "table", frame, testFrame)
	if !errors.Is(err, errGamingSendUncertain) || fresh {
		t.Fatalf("in-flight duplicate claim = %v, %v", fresh, err)
	}
	if err = markGamingFrameSent("poker", "table", frame, testFrame); err != nil {
		t.Fatalf("mark sent: %v", err)
	}
	fresh, err = claimGamingFrameSend("poker", "table", frame, testFrame)
	if err != nil || fresh {
		t.Fatalf("sent duplicate claim = %v, %v", fresh, err)
	}

	// Reload from disk to prove restart idempotence.
	gamingOutbox.Lock()
	gamingOutbox.dir = ""
	gamingOutbox.claims = nil
	gamingOutbox.Unlock()
	fresh, err = claimGamingFrameSend("poker", "table", frame, testFrame)
	if err != nil || fresh {
		t.Fatalf("restart claim = %v, %v", fresh, err)
	}
}

func TestSlowGameReconnectsAndReplaysInsteadOfLosingFrames(t *testing.T) {
	withGamingWireDir(t)
	b := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	base := GamingFrameEvent{Game: "poker", GCID: "table", From: "alice"}
	base.Frame = testFrame
	seq, _, err := b.persistGamingFrame(base)
	if err != nil || seq != 1 {
		t.Fatalf("persist first: %d, %v", seq, err)
	}
	ch, cancel := b.SubscribeFrom("poker", 1, 1)
	defer cancel()

	second := base
	second.Frame = strings.Replace(testFrame, "seq=1/1", "seq=1/2", 1)
	second.Seq, _, err = b.persistGamingFrame(second)
	if err != nil {
		t.Fatal(err)
	}
	b.broadcast(second) // fills the one-frame live buffer
	third := base
	third.Frame = strings.Replace(testFrame, "seq=1/1", "seq=2/2", 1)
	third.Seq, _, err = b.persistGamingFrame(third)
	if err != nil {
		t.Fatal(err)
	}
	b.broadcast(third) // closes the stalled stream; the frame stays durable
	if got := <-ch; got.Seq != 2 {
		t.Fatalf("buffered frame = %+v", got)
	}
	if _, open := <-ch; open {
		t.Fatal("stalled stream remained open after backpressure")
	}

	replayed, stop := b.SubscribeFrom("poker", 1, 1)
	defer stop()
	if got := <-replayed; got.Seq != 2 {
		t.Fatalf("first replay = %+v", got)
	}
	if got := <-replayed; got.Seq != 3 {
		t.Fatalf("second replay = %+v", got)
	}
}

func TestFinancialFramesReplayOnlyInsideBridge(t *testing.T) {
	withGamingWireDir(t)
	b := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	ev := GamingFrameEvent{
		Game: "poker", GCID: "table", From: "alice", Frame: testFrame, Financial: true,
	}
	if seq, fresh, err := b.persistGamingFrame(ev); err != nil || !fresh || seq != 1 {
		t.Fatalf("persist financial frame = %d, %v, %v", seq, fresh, err)
	}
	if got := b.financialReplay(); len(got) != 1 || !got[0].Financial {
		t.Fatalf("financial replay = %+v", got)
	}
	ch, cancel := b.SubscribeFrom("poker", 0, 1)
	defer cancel()
	select {
	case got := <-ch:
		t.Fatalf("financial authority frame escaped to game: %+v", got)
	default:
	}
}

func freshGamingBus() *GamingBus {
	return &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
}

func gamingInboxSeqs(t *testing.T, b *GamingBus, game string) []uint64 {
	t.Helper()
	ch, cancel := b.SubscribeFrom(game, 0, 1)
	defer cancel()
	var seqs []uint64
	for len(ch) > 0 {
		seqs = append(seqs, (<-ch).Seq)
	}
	return seqs
}

// Every byte of '<' is written as the six-byte <, so a 1 MiB frame is a
// line of over 6 MB. It must read back after a restart like any other.
func TestGamingInboxReadsBackAFrameThatEscapesToSixTimesItsSize(t *testing.T) {
	withGamingWireDir(t)
	b := freshGamingBus()
	for i, frame := range []string{testFrame, strings.Repeat("<", 1<<20)} {
		seq, _, err := b.persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: "table", From: "alice", Frame: frame})
		if err != nil || seq != uint64(i+1) {
			t.Fatalf("persist %d = %d, %v", i, seq, err)
		}
	}
	restarted := freshGamingBus()
	seq, _, err := restarted.persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: "table", From: "bob", Frame: testFrame})
	if err != nil || seq != 3 {
		t.Fatalf("after restart = %d, %v", seq, err)
	}
	if got := gamingInboxSeqs(t, restarted, "poker"); len(got) != 3 || got[2] != 3 {
		t.Fatalf("replay %v", got)
	}
}

func TestGamingInboxRefusesALineItCouldNotReadBack(t *testing.T) {
	withGamingWireDir(t)
	b := freshGamingBus()
	if _, _, err := b.persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: "table", From: "alice", Frame: testFrame}); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("<", gamingWireMaxLine/6+1)
	if _, _, err := b.persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: "table", From: "alice", Frame: huge}); err == nil {
		t.Fatal("wrote a line longer than the inbox reads")
	}
	if got := gamingInboxSeqs(t, freshGamingBus(), "poker"); len(got) != 1 {
		t.Fatalf("after restart %v", got)
	}
}

// A crash in the middle of an append leaves a last line without a newline;
// nothing acted on it, so the next start drops it and carries on in sequence.
func TestGamingWireJournalsDropAnUnfinishedLastRecord(t *testing.T) {
	withGamingWireDir(t)
	b := freshGamingBus()
	for _, from := range []string{"alice", "bob"} {
		if _, _, err := b.persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: "table", From: from, Frame: testFrame}); err != nil {
			t.Fatal(err)
		}
	}
	inbox := filepath.Join(GamingStateDir, gamingInboxFile)
	f, err := os.OpenFile(inbox, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"seq":3,"game":"po`)
	f.Close()

	restarted := freshGamingBus()
	seq, _, err := restarted.persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: "table", From: "carol", Frame: testFrame})
	if err != nil || seq != 3 {
		t.Fatalf("after the torn record = %d, %v", seq, err)
	}
	if got := gamingInboxSeqs(t, freshGamingBus(), "poker"); len(got) != 3 {
		t.Fatalf("replay %v", got)
	}

	frame, _ := parseGamingFrame(testFrame)
	if _, err := claimGamingFrameSend("poker", "table", frame, testFrame); err != nil {
		t.Fatal(err)
	}
	if err := markGamingFrameSent("poker", "table", frame, testFrame); err != nil {
		t.Fatal(err)
	}
	outbox := filepath.Join(GamingStateDir, gamingOutboxFile)
	f, err = os.OpenFile(outbox, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"game":"poker","gc`)
	f.Close()
	gamingOutbox.Lock()
	gamingOutbox.dir, gamingOutbox.claims = "", nil
	gamingOutbox.Unlock()
	if fresh, err := claimGamingFrameSend("poker", "table", frame, testFrame); err != nil || fresh {
		t.Fatalf("outbox after the torn record = %v, %v", fresh, err)
	}
}

// Damage anywhere but the last line is not a crash artefact. Every call must
// refuse until it is repaired, never carry on with the part that did read.
func TestGamingInboxWithDamageInsideRefusesUntilRepaired(t *testing.T) {
	withGamingWireDir(t)
	b := freshGamingBus()
	if _, _, err := b.persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: "table", From: "alice", Frame: testFrame}); err != nil {
		t.Fatal(err)
	}
	inbox := filepath.Join(GamingStateDir, gamingInboxFile)
	good, err := os.ReadFile(inbox)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inbox, append(append([]byte{}, good...), "not a record\n"...), 0o600); err != nil {
		t.Fatal(err)
	}

	restarted := freshGamingBus()
	for i := 0; i < 2; i++ {
		if _, _, err := restarted.persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: "table", From: "bob", Frame: testFrame}); err == nil {
			t.Fatalf("call %d persisted past a damaged inbox", i+1)
		}
	}
	ch, cancel := restarted.SubscribeFrom("poker", 0, 1)
	if _, open := <-ch; open {
		t.Fatal("a game was subscribed to a damaged inbox")
	}
	cancel()

	if err := os.WriteFile(inbox, good, 0o600); err != nil {
		t.Fatal(err)
	}
	if seq, _, err := restarted.persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: "table", From: "bob", Frame: testFrame}); err != nil || seq != 2 {
		t.Fatalf("after repair = %d, %v", seq, err)
	}
}
