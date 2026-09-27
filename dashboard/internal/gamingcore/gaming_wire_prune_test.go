package gamingcore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	pruneGCA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pruneGCB  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pruneSelf = "1111111111111111111111111111111111111111111111111111111111111111"
	prunePeer = "2222222222222222222222222222222222222222222222222222222222222222"
)

// wireFrame is testFrame with its message id varied, so each is distinct.
func wireFrame(n byte) string {
	return strings.Replace(testFrame, "mid=5", "mid="+string('0'+n), 1)
}

func mustPersist(t *testing.T, b *Bridge, game, gcid string, n byte) uint64 {
	t.Helper()
	seq, fresh, err := b.persistGamingFrame(GamingFrameEvent{Game: game, GCID: gcid, From: prunePeer, Frame: wireFrame(n)})
	if err != nil || !fresh {
		t.Fatalf("persist %s/%s/%d = %v, %v", game, gcid, n, fresh, err)
	}
	return seq
}

func inboxFrames(t *testing.T, b *Bridge, game string) []GamingFrameEvent {
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
	br := newTestBridge(t)
	mustPersist(t, br, "poker", pruneGCB, 1)
	mustPersist(t, br, "poker", pruneGCA, 2)
	mustPersist(t, br, "poker", pruneGCB, 3)
	mustPersist(t, br, "poker", pruneGCA, 4)
	mustPersist(t, br, "chess", pruneGCA, 5)

	removed, err := br.pruneGamingGroup(pruneGCA)
	if err != nil || removed != 3 {
		t.Fatalf("prune = %d, %v", removed, err)
	}
	if removed, err = br.pruneGamingGroup(pruneGCA); err != nil || removed != 0 {
		t.Fatalf("second prune = %d, %v", removed, err)
	}
	poker := inboxFrames(t, br, "poker")
	if len(poker) != 2 || poker[0].Seq != 1 || poker[1].Seq != 3 || poker[1].Frame != wireFrame(3) {
		t.Fatalf("poker after prune = %+v", poker)
	}
	if chess := inboxFrames(t, br, "chess"); len(chess) != 0 {
		t.Fatalf("chess after prune = %+v", chess)
	}

	restarted := New(br.dataDir, br.host)
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
	br := newTestBridge(t)
	mustPersist(t, br, "poker", pruneGCA, 1)
	mustPersist(t, br, "poker", pruneGCB, 2)
	if _, err := br.pruneGamingGroup(pruneGCB); err != nil {
		t.Fatal(err)
	}
	// The mark must still be above every kept frame; one below is damage.
	restarted := New(br.dataDir, br.host)
	if seq := mustPersist(t, restarted, "poker", pruneGCA, 3); seq != 3 {
		t.Fatalf("seq after mark = %d", seq)
	}
	bad := `{"seq":1,"game":"poker","gcid":"","from":"","frame":"","mark":true}` + "\n"
	appendInboxLine(t, br, bad)
	if _, _, err := New(br.dataDir, br.host).persistGamingFrame(GamingFrameEvent{Game: "poker", GCID: pruneGCA, From: prunePeer, Frame: wireFrame(4)}); err == nil {
		t.Fatal("loaded a mark below the kept frames")
	}
}

func TestRecoverHistoryDeliversJournaledFramesOfKnownGroups(t *testing.T) {
	br := newTestBridge(t)
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker"}
	if err := br.writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
	payoutLedger(t, br, "awaiting_signatures")
	mustPersist(t, br, "poker", pruneGCA, 0)
	mustJournal(t, br, pruneGCA, prunePeer, 1, true)
	mustJournal(t, br, pruneGCB, prunePeer, 2, true)
	br.RecoverHistory()
	got := inboxFrames(t, br, "poker")
	if len(got) != 2 || got[1].Frame != wireFrame(1) || got[1].From != prunePeer {
		t.Fatalf("recovered = %+v", got)
	}
}

// withGCHistory stands in for BR's own log of a group, with this client
// logging under nick and holding pruneSelf's identity.
func withGCHistory(t *testing.T, br *Bridge, nick string, entries ...map[string]any) {
	t.Helper()
	br.gamingGCHistoryFetch = func(_ context.Context, _ [32]byte, page, _ int) ([]GroupEntry, error) {
		if page > 0 {
			return nil, nil
		}
		out := make([]GroupEntry, 0, len(entries))
		for _, e := range entries {
			msg, _ := e["message"].(string)
			from, _ := e["from"].(string)
			out = append(out, GroupEntry{From: from, Message: msg})
		}
		return out, nil
	}
	br.gamingSelfNick = func(context.Context) (string, error) { return nick, nil }
	br.gamingSelfUID = func(context.Context) (string, error) { return pruneSelf, nil }
}

func TestFrameInHistoryCountsOnlyOwnSends(t *testing.T) {
	br := newTestBridge(t)
	id, _ := parseGamingGCID(pruneGCA)
	withGCHistory(t, br, "me", map[string]any{"message": wireFrame(1), "from": "peer"})
	if found, err := br.gamingFrameInHistory(context.Background(), id, wireFrame(1)); err != nil || found {
		t.Fatalf("peer copy counted as sent: %v, %v", found, err)
	}
	withGCHistory(t, br, "me", map[string]any{"message": wireFrame(2), "from": "me"})
	if found, err := br.gamingFrameInHistory(context.Background(), id, wireFrame(1)); err != nil || found {
		t.Fatalf("another own message counted as this send: %v, %v", found, err)
	}
	withGCHistory(t, br, "me", map[string]any{"message": wireFrame(1), "from": "me"})
	if found, err := br.gamingFrameInHistory(context.Background(), id, wireFrame(1)); err != nil || !found {
		t.Fatalf("own send not found: %v, %v", found, err)
	}
}

func TestPruneSettledHistoryJournalFirstAndOnce(t *testing.T) {
	br := newTestBridge(t)
	mustPersist(t, br, "poker", pruneGCA, 1)
	mustPersist(t, br, "poker", pruneGCB, 2)
	mustJournal(t, br, pruneGCA, prunePeer, 1, true)
	path := filepath.Join(br.dataDir, gamingJournalFile)
	good, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append([]byte("not json\n"), good...), 0o600); err != nil {
		t.Fatal(err)
	}
	reloadGamingJournal(br)

	br.pruneSettledGamingHistory(context.Background(), []string{pruneGCA})
	if got := inboxFrames(t, br, "poker"); len(got) != 2 {
		t.Fatalf("inbox pruned although the journal was not: %+v", got)
	}
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	br.pruneSettledGamingHistory(context.Background(), []string{pruneGCA})
	if got := inboxFrames(t, br, "poker"); len(got) != 1 || got[0].GCID != pruneGCB {
		t.Fatalf("inbox after prune = %+v", got)
	}
	if got := journalSeqs(t, br, pruneGCA); len(got) != 0 {
		t.Fatalf("journal after prune = %v", got)
	}
	// Once per group per run: a frame journaled after the prune stays.
	mustJournal(t, br, pruneGCA, prunePeer, 3, true)
	br.pruneSettledGamingHistory(context.Background(), []string{pruneGCA})
	if got := journalSeqs(t, br, pruneGCA); len(got) != 1 {
		t.Fatalf("pruned twice in one run: %v", got)
	}
}

func appendInboxLine(t *testing.T, br *Bridge, line string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(br.dataDir, gamingInboxFile), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
}
