package gamingcore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dcrpulse/internal/gamingfunds"
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

func TestAcceptRefusesSeatCountsCreateWouldRefuse(t *testing.T) {
	withGamingWireDir(t)
	for seats, refused := range map[string]bool{"1": true, "2": false, "6": false, "7": true, "13": true} {
		invite := "gaming://poker/table?fv=2&sid=a1&buyin=100000&csv=288&seats=" + seats
		err := authorizeGamingTable(t.Context(), "poker", invite, pruneGCA)
		if got := err != nil && err.Error() == "invalid seat count"; got != refused {
			t.Errorf("seats=%s: %v", seats, err)
		}
	}
}

// seatedPokerLedger writes a poker table in group A with two seated keys,
// bound or not, and the UIDs that have announced them.
func seatedPokerLedger(t *testing.T, bound bool, announced map[string]string) {
	t.Helper()
	scope := gamingfunds.Scope{Game: "poker", Network: "mainnet", Wallet: "fp"}
	key, _ := json.Marshal(struct {
		Scope gamingfunds.Scope
		Table string
	}{scope, "0123456789abcdef"})
	peers := map[string]any{}
	for uid, k := range announced {
		peers[uid] = map[string]any{"uid": uid, "key": k}
	}
	ledger := map[string]any{
		"version": gamingfunds.Version, "rosterCommits": map[string]any{}, "keys": map[string]any{},
		"deposits": map[string]any{}, "operations": map[string]any{}, "previews": map[string]any{}, "quotes": map[string]any{},
		"settlements": map[string]any{},
		"tables":      map[string]any{string(key): map[string]any{"scope": scope, "table": "0123456789abcdef", "group": pruneGCA, "seats": 2, "until": 1}},
		"peers":       map[string]any{string(key): peers},
	}
	if bound {
		ledger["seated"] = map[string]any{string(key): []string{"02aa", "02bb"}}
	}
	raw, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(GamingStateDir, "financial-authority")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "authority.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

const strangerUID = "3333333333333333333333333333333333333333333333333333333333333333"

func registerPokerAndStakeWars(t *testing.T) {
	t.Helper()
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker", "stakewars"}
	if err := writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
}

func TestFramesAreBoundToTheirTableAndSeatedPlayers(t *testing.T) {
	selfUID := strings.Repeat("11", 32)
	complete := map[string]string{prunePeer: "02aa", selfUID: "02bb"}
	for _, tc := range []struct {
		name      string
		bound     bool
		announced map[string]string
		from      string
		frame     string
		stored    bool
	}{
		{"seated player", true, complete, prunePeer, wireFrame(1), true},
		{"unseated member once seated", true, complete, strangerUID, wireFrame(2), false},
		{"any member before the seat draw", false, map[string]string{}, strangerUID, wireFrame(3), true},
		{"any member while a seat is unannounced", true, map[string]string{prunePeer: "02aa"}, strangerUID, wireFrame(4), true},
		{"another table in the group", false, map[string]string{}, prunePeer, strings.Replace(wireFrame(5), "sid=0123456789abcdef", "sid=0123456789abcdee", 1), false},
		{"another game in the group", false, map[string]string{}, prunePeer, strings.Replace(wireFrame(6), "game=poker", "game=stakewars", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withGamingWireDir(t)
			registerPokerAndStakeWars(t)
			seatedPokerLedger(t, tc.bound, tc.announced)
			b := newWireBus()
			b.deliverGamingMessage(pruneGCA, tc.from, tc.frame)
			got := len(inboxFrames(t, b, "poker")) + len(inboxFrames(t, b, "stakewars"))
			if (got == 1) != tc.stored {
				t.Fatalf("stored %d frames, want stored=%v", got, tc.stored)
			}
		})
	}
}

func TestFinancialReplayKeepsToTheGamesOwnGroups(t *testing.T) {
	withGamingWireDir(t)
	seatedPokerLedger(t, false, map[string]string{})
	b := newWireBus()
	for i, game := range []string{"poker", "stakewars"} {
		ev := GamingFrameEvent{Game: game, GCID: pruneGCA, From: prunePeer, Frame: wireFrame(byte(i)), Financial: true}
		if _, _, err := b.persistGamingFrame(ev); err != nil {
			t.Fatal(err)
		}
	}
	got := b.financialReplay()
	if len(got) != 1 || got[0].Game != "poker" {
		t.Fatalf("replay = %+v", got)
	}
}
