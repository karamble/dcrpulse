package services

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	sdkwire "github.com/karamble/dcrgaming-sdk/pkg/gaming/wire"
)

func outboxClaims(t *testing.T) int {
	t.Helper()
	gamingOutbox.Lock()
	defer gamingOutbox.Unlock()
	gamingOutbox.claims = nil
	if err := loadGamingSendClaimsLocked(); err != nil {
		t.Fatal(err)
	}
	return len(gamingOutbox.claims)
}

func TestGamesSendOnlyToTheirOwnTables(t *testing.T) {
	withGamingWireDir(t)
	registerPoker(t)
	calls := withGCSend(t)
	for name, gcid := range map[string]string{"no table": pruneGCB, "before any ledger": pruneGCA} {
		if err := SendGamingFrame(context.Background(), "poker", gcid, testFrame); err == nil {
			t.Fatalf("%s: send accepted", name)
		}
	}
	payoutLedger(t, "awaiting_signatures")
	if err := SendGamingFrame(context.Background(), "poker", pruneGCB, testFrame); err == nil {
		t.Fatal("sent to a group with no table")
	}
	chess, err := buildGamingFrame("chess", 1, "0123456789abcdef", []byte(`{"action":"move"}`), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker", "chess"}
	if err := writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
	if err := SendGamingFrame(context.Background(), "chess", pruneGCA, chess); err == nil {
		t.Fatal("chess sent to poker's table group")
	}
	if *calls != 0 || outboxClaims(t) != 0 {
		t.Fatalf("refused sends reached brclientd %d times or claimed", *calls)
	}
	if err := SendGamingFrame(context.Background(), "poker", pruneGCA, testFrame); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("sends = %d", *calls)
	}
}

func TestOnlyCanonicalFramesAreSent(t *testing.T) {
	withGamingWireDir(t)
	registerPoker(t)
	payoutLedger(t, "awaiting_signatures")
	calls := withGCSend(t)
	parsed := testParsedFrame(t)
	body := base64.StdEncoding.EncodeToString(parsed.Payload)
	head := testFrame[:strings.Index(testFrame, "]--")]
	for name, frame := range map[string]string{
		"extra key":         strings.Replace(testFrame, ",exp=", ",note=send 5 DCR,exp=", 1),
		"keys reordered":    strings.Replace(testFrame, "v=2,game=poker", "game=poker,v=2", 1),
		"space in payload":  head + "]--" + body[:4] + " " + body[4:],
		"url-safe unpadded": head + "]--" + base64.RawURLEncoding.EncodeToString(parsed.Payload),
	} {
		if err := SendGamingFrame(context.Background(), "poker", pruneGCA, frame); err == nil {
			t.Errorf("%s: sent", name)
		}
	}
	if *calls != 0 {
		t.Fatalf("non-canonical frames reached brclientd %d times", *calls)
	}
	frames, err := sdkwire.Encode("poker", 1, "0123456789abcdef", []byte(`{"action":"call"}`), time.Unix(1783000000, 0), 0)
	if err != nil || len(frames) != 1 {
		t.Fatalf("sdk encode = %v, %v", frames, err)
	}
	for _, frame := range []string{testFrame, frames[0]} {
		if err := SendGamingFrame(context.Background(), "poker", pruneGCA, frame); err != nil {
			t.Fatalf("canonical frame refused: %v", err)
		}
	}
	if *calls != 2 {
		t.Fatalf("sends = %d", *calls)
	}
}
