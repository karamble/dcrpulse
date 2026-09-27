package gamingcore

import (
	"testing"
)

func TestRecoverHistoryReadsTablesKnownOnlyFromTheLedger(t *testing.T) {
	br := newTestBridge(t)
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker"}
	if err := br.writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
	payoutLedger(t, br, "awaiting_signatures")
	mustJournal(t, br, pruneGCA, prunePeer, 1, true)
	br.RecoverHistory()
	br.RecoverHistory()
	got := inboxFrames(t, br, "poker")
	if len(got) != 1 || got[0].GCID != pruneGCA || got[0].From != prunePeer {
		t.Fatalf("recovered %+v", got)
	}
}
