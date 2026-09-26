package services

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func registerPoker(t *testing.T) {
	t.Helper()
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker"}
	if err := writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
}

func TestFramesOutsideAcceptedTablesAreNotStored(t *testing.T) {
	withGamingWireDir(t)
	registerPoker(t)
	b := newWireBus()
	if !b.deliverGamingMessage(pruneGCA, prunePeer, wireFrame(1)) {
		t.Fatal("a gaming frame was not recognised as one")
	}
	if got := inboxFrames(t, b, "poker"); len(got) != 0 {
		t.Fatalf("stored a frame of a group with no table: %+v", got)
	}
	payoutLedger(t, "awaiting_signatures")
	b.deliverGamingMessage(pruneGCA, prunePeer, wireFrame(1))
	b.deliverGamingMessage(pruneGCB, prunePeer, wireFrame(2))
	got := inboxFrames(t, b, "poker")
	if len(got) != 1 || got[0].GCID != pruneGCA {
		t.Fatalf("stored = %+v", got)
	}
}

func TestFinancialReplaySkipsUnknownGroupsAndAppliedFrames(t *testing.T) {
	withGamingWireDir(t)
	payoutLedger(t, "awaiting_signatures")
	b := newWireBus()
	for i, gcid := range []string{pruneGCA, pruneGCB, pruneGCA} {
		ev := GamingFrameEvent{Game: "poker", GCID: gcid, From: prunePeer, Frame: wireFrame(byte(i)), Financial: true}
		if _, _, err := b.persistGamingFrame(ev); err != nil {
			t.Fatal(err)
		}
	}
	got := b.financialReplay()
	if len(got) != 2 || got[0].GCID != pruneGCA || got[1].GCID != pruneGCA {
		t.Fatalf("replay = %+v", got)
	}
	b.markFinancialApplied(got[0])
	again := b.financialReplay()
	if len(again) != 1 || again[0].Seq != got[1].Seq {
		t.Fatalf("replay after applying one = %+v", again)
	}
}

func TestAcceptingATableReadsItsFramesBack(t *testing.T) {
	calls := 0
	old := gamingRecoverAfterAccept
	gamingRecoverAfterAccept = func() { calls++ }
	t.Cleanup(func() { gamingRecoverAfterAccept = old })
	withGamingWireDir(t)
	// A malformed invitation never reaches the ledger and recovers nothing.
	if err := authorizeGamingTable(t.Context(), "poker", "gaming://poker/nope", pruneGCA); err == nil || calls != 0 {
		t.Fatalf("bad invite = %v, recoveries %d", err, calls)
	}
}

func TestAppliedFinancialFramesLeaveTheReplay(t *testing.T) {
	withGamingWireDir(t)
	registerPoker(t)
	payoutLedger(t, "awaiting_signatures")
	b := Gaming()
	old := receiveFinancial
	t.Cleanup(func() {
		receiveFinancial = old
		b.wireMu.Lock()
		b.financialDone = nil
		b.wireMu.Unlock()
	})
	fin := strings.Replace(wireFrame(1), "--gaming[", "--gaming[authority=3,", 1)
	fin2 := strings.Replace(wireFrame(2), "--gaming[", "--gaming[authority=3,", 1)
	for _, f := range []string{fin, fin2} {
		b.deliverGamingMessage(pruneGCA, prunePeer, f)
	}
	var live []GamingFrameEvent
	for len(gamingFinancialInbox) > 0 {
		live = append(live, <-gamingFinancialInbox)
	}
	if len(live) != 2 || live[0].Seq == 0 {
		t.Fatalf("live financial events = %+v", live)
	}
	receiveFinancial = func(context.Context, GamingFrameEvent) error { return errors.New("wallet locked") }
	processFinancialFrame(context.Background(), live[1])
	receiveFinancial = func(context.Context, GamingFrameEvent) error { return nil }
	processFinancialFrame(context.Background(), live[0])
	got := b.financialReplay()
	if len(got) != 1 || got[0].Seq != live[1].Seq {
		t.Fatalf("replay = %+v", got)
	}
}
