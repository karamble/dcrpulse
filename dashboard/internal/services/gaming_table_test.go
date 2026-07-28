// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"strings"
	"testing"
)

// The link has to be the one a game's own parser accepts, and the two live in
// different repositories that cannot import each other. Pinned against a
// literal so a change here is a deliberate change to a published format.
func TestGamingInviteLinkIsTheAgreedShape(t *testing.T) {
	got := gamingInviteLink("poker", "0123456789abcdef", 10_000_000, 6, 288, 900017)
	want := "gaming://poker/table?buyin=10000000&csv=288&seats=6&sid=0123456789abcdef&until=900017"
	if got != want {
		t.Fatalf("invite link is\n got %s\nwant %s", got, want)
	}
}

// The session id becomes the routing key on the wire, which is 1 to 32
// lowercase hex. Anything else is refused when the invitation is read, so it
// must not be produced here.
func TestGamingInviteSessionIsRoutable(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		table, err := gamingSessionID()
		if err != nil {
			t.Fatalf("mint a session: %v", err)
		}
		if len(table) == 0 || len(table) > 32 {
			t.Fatalf("session %q is not 1 to 32 characters", table)
		}
		if strings.Trim(table, "0123456789abcdef") != "" {
			t.Fatalf("session %q is not lowercase hex", table)
		}
		if seen[table] {
			t.Fatalf("session %q was minted twice", table)
		}
		seen[table] = true
	}
}

// A table is 2 to 6 seats and the buy-in is not optional. Refusing here means
// an unusable invitation is never sent to a group chat.
func TestCreatingRefusesTermsNobodyCanPlay(t *testing.T) {
	for _, c := range []struct {
		seats uint32
		buyin uint64
		why   string
	}{
		{1, 10_000_000, "one seat is not a table"},
		{7, 10_000_000, "seven seats is beyond the escrow"},
		{2, 0, "no buy-in is no stake"},
	} {
		_, err := CreateGamingTable(t.Context(), "poker", strings.Repeat("ab", 32), c.buyin, c.seats)
		if err == nil {
			t.Errorf("%d seats at %d atoms was accepted, and %s", c.seats, c.buyin, c.why)
		}
	}
}
