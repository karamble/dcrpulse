// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import "testing"

func TestIsOverLimit(t *testing.T) {
	for _, e := range []error{errPerTxExceeded, errDailyExceeded} {
		if !isOverLimit(e) {
			t.Errorf("%v should trip the tripwire", e)
		}
	}
	// Wrong-account / non-allowlisted / no-grant / expired / bad-amount are
	// denials but must NOT trip the tripwire (likelier honest mistakes).
	for _, e := range []error{errAccountNotGranted, errAddrNotAllowed, errNoGrant, errGrantExpired, errBadAmount} {
		if isOverLimit(e) {
			t.Errorf("%v should not trip the tripwire", e)
		}
	}
}
