// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"dcrpulse/internal/auth"
)

// Logout must do more than tell the browser to drop the cookie: the cookie, and
// any captured copy of it, has to stop verifying on the server.
func TestLogoutRevokesTheSession(t *testing.T) {
	withPassword(t, "hunter2")
	tok, err := auth.MintSession()
	if err != nil {
		t.Fatal(err)
	}
	if !auth.ValidSession(tok) {
		t.Fatal("a fresh token does not verify")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "dcrpulse_session", Value: tok})
	w := httptest.NewRecorder()
	// Through the gate, so the cookie is what lets the request in.
	auth.RequireAuth(http.HandlerFunc(AuthLogoutHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout = %d %s", w.Code, w.Body.String())
	}

	cleared := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "dcrpulse_session" && c.Value == "" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout did not expire the session cookie")
	}
	if auth.ValidSession(tok) {
		t.Fatal("the cookie still verifies after logout; a captured copy would keep working")
	}
}
