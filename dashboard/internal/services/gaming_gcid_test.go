// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// A gcid reaches brclientd by being pasted into a URL path, so anything that is
// not a plain group chat id can leave /gc/{id}/message and reach the rest of
// that daemon's control surface. These are the shapes that do it.
func TestAGCIDThatCouldEscapeTheURLIsRefused(t *testing.T) {
	const good = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	if !gamingGCIDRe.MatchString(good) {
		t.Fatal("a real group chat id was refused")
	}

	for name, gcid := range map[string]string{
		"escapes the /gc/ prefix":    "../contacts/reset-all",
		"escapes after a valid id":   good + "/../../contacts/reset-all",
		"truncates with a fragment":  good + "#",
		"truncates with a query":     good + "?",
		"reaches a sibling route":    good + "/kill",
		"empty":                      "",
		"uppercase hex":              "0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef",
		"one character short":        good[:63],
		"one character long":         good + "a",
		"leading whitespace":         " " + good,
		"a newline after a valid id": good + "\n",
		"not hex at all":             "not-a-group-chat-id",
	} {
		if gamingGCIDRe.MatchString(gcid) {
			t.Errorf("%s: %q was accepted as a group chat id", name, gcid)
		}
	}
}
