// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
)

// A gcid becomes a path segment in a brclientd URL, so anything that is not a
// plain group chat id could reach another route. These are the shapes that try.
//
// Driven through SendGamingFrame rather than against the pattern, so it also
// covers the frame never being sent. Kills: removing the check, or moving it
// after the send.
func TestAGCIDThatCouldEscapeTheURLIsRefused(t *testing.T) {
	inviteSeams(t)

	const good = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	if !ValidGamingGCID(good) {
		t.Fatal("a real group chat id was refused")
	}

	bad := map[string]string{
		"escapes the /gc/ prefix":    "../contacts/reset-all",
		"escapes after a valid id":   good + "/../../contacts/reset-all",
		"truncates with a fragment":  good + "#",
		"truncates with a query":     good + "?",
		"reaches a sibling route":    good + "/kill",
		"reaches a two-part action":  good + "/history/clear",
		"pre-escaped separator":      good + "%2fkill",
		"empty":                      "",
		"uppercase hex":              "0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef",
		"one character short":        good[:63],
		"one character long":         good + "a",
		"leading whitespace":         " " + good,
		"a newline after a valid id": good + "\n",
		"not hex at all":             "not-a-group-chat-id",
	}
	if len(bad) < 14 {
		t.Fatalf("the table shrank to %d shapes", len(bad))
	}

	for name, gcid := range bad {
		if ValidGamingGCID(gcid) {
			t.Errorf("%s: %q was accepted as a group chat id", name, gcid)
		}
		// testFrame names poker, which spendPolicy registers.
		err := SendGamingFrame(context.Background(), "poker", gcid, testFrame)
		if !errors.Is(err, ErrGamingBadGCID) {
			t.Errorf("%s: SendGamingFrame returned %v, want ErrGamingBadGCID", name, err)
		}
	}
}
