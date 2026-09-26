package services

import (
	"context"
	"encoding/json"
	"testing"

	"dcrpulse/internal/rpc"
)

func TestFirstContactOncePerProcess(t *testing.T) {
	var s notifGapState
	if !s.firstContact() {
		t.Fatal("first event was not first contact")
	}
	s.observe(5, "abc", 0)
	s.observe(9, "def", 0)
	if s.firstContact() {
		t.Fatal("first contact reported twice")
	}
}

func TestRecoverHistoryReadsTablesKnownOnlyFromTheLedger(t *testing.T) {
	withGamingWireDir(t)
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker"}
	if err := writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
	payoutLedger(t, "awaiting_signatures")
	var asked []string
	pages := historyPages(map[string]any{"message": wireFrame(1), "from": prunePeer})
	withHistorySeams(t, func(ctx context.Context, id rpc.ShortIDHex, page, size int) (json.RawMessage, error) {
		asked = append(asked, id.String())
		return pages(ctx, id, page, size)
	})
	b := newWireBus()
	b.RecoverHistory()
	b.RecoverHistory()
	got := inboxFrames(t, b, "poker")
	if len(asked) == 0 || asked[0] != pruneGCA || len(got) != 1 || got[0].GCID != pruneGCA || got[0].From != prunePeer {
		t.Fatalf("asked %v, recovered %+v", asked, got)
	}
}
