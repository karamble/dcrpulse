// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The group id is checked here, so a malformed one never reaches brclientd.
func TestDismissBlockedReinviteRefusesAMalformedGroupID(t *testing.T) {
	for _, body := range []string{`{"gcid":""}`, `{"gcid":"zz"}`, `{"gcid":"` + strings.Repeat("ab", 31) + `/.."}`, `{}`} {
		rec := httptest.NewRecorder()
		BisonrelayGCBlockedReinviteDismissHandler(rec, httptest.NewRequest(http.MethodPost, "/api/br/gc/invites/blocked/dismiss", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400", body, rec.Code)
		}
	}
}
