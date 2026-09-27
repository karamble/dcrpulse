package gamingcore

import (
	"testing"
)

func TestRecoverHistoryReadsTablesKnownOnlyFromTheLedger(t *testing.T) {
	withGamingWireDir(t)
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker"}
	if err := writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
	payoutLedger(t, "awaiting_signatures")
	mustJournal(t, pruneGCA, prunePeer, 1, true)
	b := newWireBus()
	b.RecoverHistory()
	b.RecoverHistory()
	got := inboxFrames(t, b, "poker")
	if len(got) != 1 || got[0].GCID != pruneGCA || got[0].From != prunePeer {
		t.Fatalf("recovered %+v", got)
	}
}
