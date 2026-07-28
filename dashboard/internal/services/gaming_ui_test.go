// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"
	"time"

	"dcrpulse/internal/types"
)

// The two token namespaces must never meet.
//
// A game's token authorizes spending from the user's wallet account, sending
// Bison Relay messages as them, and broadcasting transactions. A panel token
// authorizes reaching one game's API through a proxy, on an allowlist, for
// fifteen minutes. If either resolver ever accepted the other's tokens, a
// browser tab would hold the first set of powers.
//
// Discipline is not enough for this, so it is a test.
func TestPanelTokensAndGameTokensAreDifferentThings(t *testing.T) {
	RevokeAllGamingUISessions()

	// The settings file lives at a compile-time path this test cannot
	// write, which is why the resolution rule is a separate function from
	// the file read. That separation is the thing being used here.
	gameToken, err := newGamingToken()
	if err != nil {
		t.Fatalf("mint a game token: %v", err)
	}
	settings := types.GamingSettings{
		Enabled:    true,
		GameTokens: map[string]string{"poker": gameToken},
	}

	panelToken, _, err := MintGamingUISession("poker", "", "Dsaddr")
	if err != nil {
		t.Fatalf("mint a panel token: %v", err)
	}
	t.Cleanup(func() { RevokeGamingUISession(panelToken) })

	// Each resolver must know its own and refuse the other's. The two are
	// built by different code from different sources on purpose - a test
	// that derived both from one place would be checking the derivation.
	if game, ok := gamingGameForToken(settings, panelToken); ok {
		t.Fatalf("a panel token resolved as the game %q, so a page could spend", game)
	}
	if _, ok := GamingUISessionFor(gameToken); ok {
		t.Fatal("a game's own token resolved as an open panel")
	}
	if game, ok := gamingGameForToken(settings, gameToken); !ok || game != "poker" {
		t.Fatalf("the game's own token stopped resolving: %q %v", game, ok)
	}
	if s, ok := GamingUISessionFor(panelToken); !ok || s.Game != "poker" {
		t.Fatalf("the panel token stopped resolving: %+v %v", s, ok)
	}
}

// Closing a panel ends its token, and so does logging out.
func TestAPanelTokenCanBeTakenBack(t *testing.T) {
	RevokeAllGamingUISessions()

	token, _, err := MintGamingUISession("poker", "abc", "Dsaddr")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if _, ok := GamingUISessionFor(token); !ok {
		t.Fatal("a freshly minted token does not resolve")
	}

	RevokeGamingUISession(token)
	if _, ok := GamingUISessionFor(token); ok {
		t.Fatal("a closed panel's token still works")
	}

	token, _, _ = MintGamingUISession("poker", "", "Dsaddr")
	RevokeAllGamingUISessions()
	if _, ok := GamingUISessionFor(token); ok {
		t.Fatal("logging out left a panel token working")
	}
}

// Uninstalling a game revokes its panels, rather than hiding them.
func TestUninstallingAGameClosesItsPanels(t *testing.T) {
	RevokeAllGamingUISessions()

	token, _, err := MintGamingUISession("poker", "", "Dsaddr")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	RevokeGamingUISessionsFor("poker")
	if _, ok := GamingUISessionFor(token); ok {
		t.Fatal("uninstalling a game left a browser tab able to reach it")
	}
}

// Rotation replaces the token rather than extending it, so one that leaked has
// a bounded life even while the panel it came from stays open.
func TestRotatingAPanelReplacesItsToken(t *testing.T) {
	RevokeAllGamingUISessions()

	first, _, err := MintGamingUISession("poker", "table-1", "Dsaddr")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	second, session, ok := RotateGamingUISession(first)
	if !ok {
		t.Fatal("rotating an open panel failed")
	}
	if second == first {
		t.Fatal("rotation handed back the same token")
	}
	if _, ok := GamingUISessionFor(first); ok {
		t.Fatal("the old token still works after rotation")
	}
	if session.TableID != "table-1" || session.PayoutAddress != "Dsaddr" {
		t.Fatalf("rotation lost what the panel was opened for: %+v", session)
	}
}

// A session that nobody touched expires on its own, without a goroutine
// watching it.
func TestAnIdlePanelExpires(t *testing.T) {
	RevokeAllGamingUISessions()

	token, _, err := MintGamingUISession("poker", "", "Dsaddr")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	uiSessionsMu.Lock()
	s := uiSessions[token]
	s.Expires = time.Now().Add(-time.Second)
	uiSessions[token] = s
	uiSessionsMu.Unlock()

	if _, ok := GamingUISessionFor(token); ok {
		t.Fatal("an expired panel token still resolves")
	}
	if n := CountGamingUISessions(); n != 0 {
		t.Fatalf("%d expired sessions were kept", n)
	}
}

// Nothing can be made to hold an unbounded number of these.
func TestOpenPanelsAreBounded(t *testing.T) {
	RevokeAllGamingUISessions()

	for range uiSessionsMax * 3 {
		if _, _, err := MintGamingUISession("poker", "", "Dsaddr"); err != nil {
			t.Fatalf("mint: %v", err)
		}
	}
	if n := CountGamingUISessions(); n > uiSessionsMax {
		t.Fatalf("%d panels are open, and the ceiling is %d", n, uiSessionsMax)
	}
}
