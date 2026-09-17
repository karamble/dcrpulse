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
