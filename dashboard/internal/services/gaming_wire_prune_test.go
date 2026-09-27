package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dcrpulse/internal/rpc"
)

const (
	pruneGCA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pruneGCB  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pruneSelf = "1111111111111111111111111111111111111111111111111111111111111111"
	prunePeer = "2222222222222222222222222222222222222222222222222222222222222222"
)

func newWireBus() *GamingBus { return &GamingBus{subs: make(map[*gamingSubscriber]struct{})} }

// wireFrame is testFrame with its message id varied, so each is distinct.
func wireFrame(n byte) string {
	return strings.Replace(testFrame, "mid=5", "mid="+string('0'+n), 1)
}

func mustPersist(t *testing.T, b *GamingBus, game, gcid string, n byte) uint64 {
	t.Helper()
	seq, fresh, err := b.persistGamingFrame(GamingFrameEvent{Game: game, GCID: gcid, From: prunePeer, Frame: wireFrame(n)})
	if err != nil || !fresh {
		t.Fatalf("persist %s/%s/%d = %v, %v", game, gcid, n, fresh, err)
	}
	return seq
}

func inboxFrames(t *testing.T, b *GamingBus, game string) []GamingFrameEvent {
	t.Helper()
	b.wireMu.Lock()
	defer b.wireMu.Unlock()
	got, err := b.loadGamingFramesLocked(game, 0)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestGamingInboxPruneKeepsOtherGroupsAndSeq(t *testing.T) {
	withGamingWireDir(t)
	b := newWireBus()
	mustPersist(t, b, "poker", pruneGCB, 1)
	mustPersist(t, b, "poker", pruneGCA, 2)
	mustPersist(t, b, "poker", pruneGCB, 3)
	mustPersist(t, b, "poker", pruneGCA, 4)
	mustPersist(t, b, "chess", pruneGCA, 5)

	removed, err := b.pruneGamingGroup(pruneGCA)
	if err != nil || removed != 3 {
		t.Fatalf("prune = %d, %v", removed, err)
	}
	if removed, err = b.pruneGamingGroup(pruneGCA); err != nil || removed != 0 {
		t.Fatalf("second prune = %d, %v", removed, err)
	}
	poker := inboxFrames(t, b, "poker")
	if len(poker) != 2 || poker[0].Seq != 1 || poker[1].Seq != 3 || poker[1].Frame != wireFrame(3) {
		t.Fatalf("poker after prune = %+v", poker)
	}
	if chess := inboxFrames(t, b, "chess"); len(chess) != 0 {
		t.Fatalf("chess after prune = %+v", chess)
	}

	restarted := newWireBus()
	if seq := mustPersist(t, restarted, "poker", pruneGCB, 6); seq != 5 {
		t.Fatalf("poker seq after restart = %d", seq)
	}
	if seq := mustPersist(t, restarted, "chess", pruneGCB, 7); seq != 2 {
		t.Fatalf("chess seq after restart = %d", seq)
	}
	// A pruned frame is no longer a duplicate, which is why brclientd goes first.
	if seq := mustPersist(t, restarted, "poker", pruneGCA, 2); seq != 6 {
		t.Fatalf("re-persisted pruned frame seq = %d", seq)
	}
}

func TestGamingInboxRejectsMarkOutOfOrder(t *testing.T) {
	withGamingWireDir(t)
	b := newWireBus()
	mustPersist(t, b, "poker", pruneGCA, 1)
	mustPersist(t, b, "poker", pruneGCB, 2)
	if _, err := b.pruneGamingGroup(pruneGCB); err != nil {
		t.Fatal(err)
	}
	// The mark must still be above every kept frame; one below is damage.
	restarted := newWireBus()
	if seq := mustPersist(t, restarted, "poker", pruneGCA, 3); seq != 3 {
		t.Fatalf("seq after mark = %d", seq)
	}
	bad := `{"seq":1,"game":"poker","gcid":"","from":"","frame":"","mark":true}` + "\n"
	appendInboxLine(t, bad)
	if _, _, err := newWireBus().persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: pruneGCA, From: prunePeer, Frame: wireFrame(4)}); err == nil {
		t.Fatal("loaded a mark below the kept frames")
	}
}

func TestRecoverHistoryDeliversJournaledFramesOfKnownGroups(t *testing.T) {
	withGamingWireDir(t)
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker"}
	if err := writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
	payoutLedger(t, "awaiting_signatures")
	b := newWireBus()
	mustPersist(t, b, "poker", pruneGCA, 0)
	mustJournal(t, pruneGCA, prunePeer, 1, true)
	mustJournal(t, pruneGCB, prunePeer, 2, true)
	b.RecoverHistory()
	got := inboxFrames(t, b, "poker")
	if len(got) != 2 || got[1].Frame != wireFrame(1) || got[1].From != prunePeer {
		t.Fatalf("recovered = %+v", got)
	}
}

// withGCHistory stands in for BR's own log of a group, with this client
// logging under nick and holding pruneSelf's identity.
func withGCHistory(t *testing.T, nick string, entries ...map[string]any) {
	t.Helper()
	oldFetch, oldNick, oldUID := gamingGCHistoryFetch, gamingSelfNick, gamingSelfUID
	gamingGCHistoryFetch = func(_ context.Context, _ rpc.ShortIDHex, page, _ int) (json.RawMessage, error) {
		if page > 0 {
			return json.RawMessage(`{"entries":[]}`), nil
		}
		return json.Marshal(map[string]any{"entries": entries})
	}
	gamingSelfNick = func(context.Context) (string, error) { return nick, nil }
	gamingSelfUID = func(context.Context) (string, error) { return pruneSelf, nil }
	t.Cleanup(func() { gamingGCHistoryFetch, gamingSelfNick, gamingSelfUID = oldFetch, oldNick, oldUID })
}

func TestFrameInHistoryCountsOnlyOwnSends(t *testing.T) {
	id, _ := parseGamingGCID(pruneGCA)
	withGCHistory(t, "me", map[string]any{"message": wireFrame(1), "from": "peer"})
	if found, err := gamingFrameInHistory(context.Background(), id, wireFrame(1)); err != nil || found {
		t.Fatalf("peer copy counted as sent: %v, %v", found, err)
	}
	withGCHistory(t, "me", map[string]any{"message": wireFrame(2), "from": "me"})
	if found, err := gamingFrameInHistory(context.Background(), id, wireFrame(1)); err != nil || found {
		t.Fatalf("another own message counted as this send: %v, %v", found, err)
	}
	withGCHistory(t, "me", map[string]any{"message": wireFrame(1), "from": "me"})
	if found, err := gamingFrameInHistory(context.Background(), id, wireFrame(1)); err != nil || !found {
		t.Fatalf("own send not found: %v, %v", found, err)
	}
}

func TestPruneSettledHistoryJournalFirstAndOnce(t *testing.T) {
	withGamingWireDir(t)
	b := newWireBus()
	mustPersist(t, b, "poker", pruneGCA, 1)
	mustPersist(t, b, "poker", pruneGCB, 2)
	mustJournal(t, pruneGCA, prunePeer, 1, true)
	path := filepath.Join(GamingStateDir, gamingJournalFile)
	good, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append([]byte("not json\n"), good...), 0o600); err != nil {
		t.Fatal(err)
	}
	reloadGamingJournal()

	b.pruneSettledGamingHistory(context.Background(), []string{pruneGCA})
	if got := inboxFrames(t, b, "poker"); len(got) != 2 {
		t.Fatalf("inbox pruned although the journal was not: %+v", got)
	}
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	b.pruneSettledGamingHistory(context.Background(), []string{pruneGCA})
	if got := inboxFrames(t, b, "poker"); len(got) != 1 || got[0].GCID != pruneGCB {
		t.Fatalf("inbox after prune = %+v", got)
	}
	if got := journalSeqs(t, pruneGCA); len(got) != 0 {
		t.Fatalf("journal after prune = %v", got)
	}
	// Once per group per run: a frame journaled after the prune stays.
	mustJournal(t, pruneGCA, prunePeer, 3, true)
	b.pruneSettledGamingHistory(context.Background(), []string{pruneGCA})
	if got := journalSeqs(t, pruneGCA); len(got) != 1 {
		t.Fatalf("pruned twice in one run: %v", got)
	}
}

func appendInboxLine(t *testing.T, line string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(GamingStateDir, gamingInboxFile), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
}
