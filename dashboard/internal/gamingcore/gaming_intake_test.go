// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"testing"
	"time"
)

func groupMessage(gcid, uid byte, text string) GroupMessage {
	m := GroupMessage{Text: text, Time: time.UnixMilli(1700000000000)}
	for i := range m.GCID {
		m.GCID[i] = gcid
		m.From[i] = uid
	}
	return m
}

func TestReceiveGroupMessageJournalsAndDeliversFrames(t *testing.T) {
	br := newTestBridge(t)
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker"}
	if err := br.writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
	payoutLedger(t, br, "awaiting_signatures")

	if err := br.receiveGroupMessage(groupMessage(0xaa, 0x22, wireFrame(1))); err != nil {
		t.Fatalf("frame: %v", err)
	}
	recs, err := br.gamingJournalHistory(pruneGCA)
	if err != nil || len(recs) != 1 || recs[0].From != prunePeer || recs[0].TS != 1700000000 {
		t.Fatalf("journal = %+v, %v", recs, err)
	}
	if got := inboxFrames(t, br, "poker"); len(got) != 1 || got[0].From != prunePeer || got[0].GCID != pruneGCA {
		t.Fatalf("delivered = %+v", got)
	}

	// Ordinary chat is left alone.
	if err := br.receiveGroupMessage(groupMessage(0xaa, 0x22, "hello table")); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if recs, _ = br.gamingJournalHistory(pruneGCA); len(recs) != 1 {
		t.Fatalf("journal after chat = %+v", recs)
	}
}
