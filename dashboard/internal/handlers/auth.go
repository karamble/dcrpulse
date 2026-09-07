// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"net/http"

	"dcrpulse/internal/auth"
	"dcrpulse/internal/mcp"
)

// stopAgentSurface stops the agent surface and releases the wallet passphrases
// its spend grants hold. A var so a test can watch that disabling the password
// reaches it, without binding a port; mirrors rpc.SwapDcrlndClients, which
// exists for the same reason.
var stopAgentSurface = func() error { return mcp.SetEnabled(false) }

// AuthStatusHandler reports the app-password state. Unauthenticated; the
// frontend uses it to decide between the login screen, the first-run setup
// prompt, the locked screen, and the app.
func AuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"enabled":        auth.Enabled(),
		"configured":     auth.Configured(),
		"authenticated":  auth.Authenticated(r),
		"setupDismissed": auth.SetupDismissed(),
		"locked":         auth.Locked(),
		"lockReason":     auth.LockReason(),
	})
}

// AuthLoginHandler verifies the password and issues a session cookie.
func AuthLoginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !auth.Enabled() {
		http.Error(w, "app password is not enabled", http.StatusBadRequest)
		return
	}
	if !auth.Verify(req.Password) {
		http.Error(w, "incorrect password", http.StatusUnauthorized)
		return
	}
	if err := auth.SetSessionCookie(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AuthSetupHandler performs first-time password configuration. Only reachable
// before a password exists (afterwards auth is enabled and the route is gated).
func AuthSetupHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if auth.Configured() {
		http.Error(w, "a password is already configured", http.StatusConflict)
		return
	}
	if err := auth.Setup(req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := auth.SetSessionCookie(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AuthSkipSetupHandler records that the user declined the first-run prompt.
func AuthSkipSetupHandler(w http.ResponseWriter, r *http.Request) {
	if err := auth.MarkSetupDismissed(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AuthLogoutHandler clears the cookie and rotates the session secret, so the
// cookie just cleared, and any captured copy of it, stops verifying everywhere.
func AuthLogoutHandler(w http.ResponseWriter, r *http.Request) {
	auth.ClearSessionCookie(w, r)
	if err := auth.Revoke(); err != nil {
		// The browser lands on the login screen either way; this line is how
		// the operator learns the old cookie is still valid.
		settLog.Errorf("Logout could not revoke the session: %v", err)
		http.Error(w, "logout could not revoke the session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AuthChangeHandler changes the password (requires the current one). Gated by
// RequireAuth, so the caller is already authenticated.
func AuthChangeHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := auth.Change(req.Current, req.New); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Refresh the cookie so the session stays alive after the change.
	_ = auth.SetSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// AuthDisableHandler turns the gate off (requires the current password). Gated
// by RequireAuth, so the caller is already authenticated.
func AuthDisableHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := auth.Disable(req.Current); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auth.ClearSessionCookie(w, r)
	// Only once the password is actually gone: reaching this on a failed
	// attempt would let anyone who can POST a wrong password stop the agent
	// surface. The listener refuses to start without a password, so leaving it
	// serving here would keep agents holding passphrases nothing can revoke.
	if err := stopAgentSurface(); err != nil {
		settLog.Errorf("Disabling the app password could not persist the agent surface state: %v", err)
	}
	w.WriteHeader(http.StatusNoContent)
}
