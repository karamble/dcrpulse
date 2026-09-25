// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// A cross-site page must be refused before brclientd is contacted, so it
// cannot take the call's single audio attachment. brclientd is unreachable in
// this test: a request that got past the origin check fails on its config.
func TestRTDTAudioRefusesForeignOriginBeforeBrclientd(t *testing.T) {
	rv := strings.Repeat("ab", 32)
	call := func(origin string) int {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/br/rtdt/sessions/"+rv+"/audio", nil)
		req.Header.Set("Origin", origin)
		req = mux.SetURLVars(req, map[string]string{"rv": rv})
		rec := httptest.NewRecorder()
		BisonrelayRTDTAudioHandler(rec, req)
		return rec.Code
	}
	if code := call("https://evil.example"); code != http.StatusForbidden {
		t.Errorf("foreign origin: status %d, want 403 before brclientd is dialed", code)
	}
	if code := call("http://127.0.0.1:8080"); code == http.StatusForbidden {
		t.Error("same origin refused as foreign")
	}
}
