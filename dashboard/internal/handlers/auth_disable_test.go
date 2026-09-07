// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"dcrpulse/internal/auth"
)

// The agent surface refuses to START without an app password, in two places.
// The same rule has to hold when the password goes away: a listener left
// serving keeps agents holding wallet passphrases, while every route that could
// revoke them answers 401. The ordering is the delicate half - stopping before
// the password is verified would hand anyone who can POST a wrong guess a way
// to switch the agent surface off.

// withPassword points auth at a scratch config and sets a password on it.
func withPassword(t *testing.T, password string) {
	t.Helper()
	restore := auth.PointAtForTest(filepath.Join(t.TempDir(), "config.json"))
	t.Cleanup(restore)
	if err := auth.Setup(password); err != nil {
		t.Fatalf("auth.Setup() = %v, want nil", err)
	}
}

// watchAgentSurface swaps the stop seam for a recorder.
func watchAgentSurface(t *testing.T, err error) *int {
	t.Helper()
	calls := 0
	prev := stopAgentSurface
	stopAgentSurface = func() error {
		calls++
		return err
	}
	t.Cleanup(func() { stopAgentSurface = prev })
	return &calls
}

func disableRequest(t *testing.T, password string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/auth/disable",
		strings.NewReader(`{"current":"`+password+`"}`))
	w := httptest.NewRecorder()
	AuthDisableHandler(w, r)
	return w
}

func TestDisablePasswordStopsTheAgentSurface(t *testing.T) {
	withPassword(t, "correct horse")
	calls := watchAgentSurface(t, nil)

	if got := disableRequest(t, "correct horse").Code; got != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", got, http.StatusNoContent)
	}
	if auth.Enabled() {
		t.Fatal("the password is still enabled")
	}
	if *calls != 1 {
		t.Fatalf("the agent surface was stopped %d times, want once", *calls)
	}
}

// The ordering trap: a wrong guess must not reach the stop.
func TestFailedDisableLeavesTheAgentSurfaceAlone(t *testing.T) {
	withPassword(t, "correct horse")
	calls := watchAgentSurface(t, nil)

	if got := disableRequest(t, "wrong guess").Code; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
	if !auth.Enabled() {
		t.Fatal("a wrong password disabled the gate")
	}
	if *calls != 0 {
		t.Fatalf("a failed attempt stopped the agent surface %d times, want none", *calls)
	}
}

// The password is already gone by the time the surface is stopped, so a failure
// there is reported in the log, not to the caller: the listener is down in this
// process either way, and it refuses to start again without a password.
func TestDisableSucceedsEvenIfTheSurfaceCannotPersist(t *testing.T) {
	withPassword(t, "correct horse")
	watchAgentSurface(t, errors.New("disk full"))

	if got := disableRequest(t, "correct horse").Code; got != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", got, http.StatusNoContent)
	}
	if auth.Enabled() {
		t.Fatal("the password is still enabled")
	}
}
