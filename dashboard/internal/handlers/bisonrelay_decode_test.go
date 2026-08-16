// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// A body the proxy could not parse must be refused here, never forwarded as a
// zero-valued request.
func TestAMalformedBodyNeverReachesBrclientd(t *testing.T) {
	t.Run("reset-all", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/br/contacts/reset-all", strings.NewReader(`{"age_days":`))
		rec := httptest.NewRecorder()
		BisonrelayContactResetAllHandler(rec, req)
		if rec.Code != 400 {
			t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
		}
		if !strings.HasPrefix(rec.Body.String(), "decode body:") {
			t.Errorf("body = %q, want prefix %q", rec.Body.String(), "decode body:")
		}
	})
	t.Run("resend-list", func(t *testing.T) {
		gcid := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		req := httptest.NewRequest("POST", "/br/gc/"+gcid+"/resend-list", strings.NewReader(`{"uid":`))
		req = mux.SetURLVars(req, map[string]string{"gcid": gcid})
		rec := httptest.NewRecorder()
		BisonrelayGCResendListHandler(rec, req)
		if rec.Code != 400 {
			t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
		}
		if !strings.HasPrefix(rec.Body.String(), "decode body:") {
			t.Errorf("body = %q, want prefix %q", rec.Body.String(), "decode body:")
		}
	})
}
