// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package auth

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSecretRotationInvalidatesSessions proves the mechanism Change relies on:
// rotating the session secret invalidates previously minted tokens.
func TestSecretRotationInvalidatesSessions(t *testing.T) {
	mu.Lock()
	prevSecret, prevEnabled, prevHash := secret, enabled, hash
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		secret, enabled, hash = prevSecret, prevEnabled, prevHash
		mu.Unlock()
	})

	mu.Lock()
	enabled = true
	secret = bytes.Repeat([]byte{0x01}, 32)
	mu.Unlock()

	tok, err := MintSession()
	if err != nil {
		t.Fatalf("MintSession() error: %v", err)
	}
	if !ValidSession(tok) {
		t.Fatal("ValidSession(tok) = false, want true before rotation")
	}

	mu.Lock()
	secret = bytes.Repeat([]byte{0x02}, 32)
	mu.Unlock()

	if ValidSession(tok) {
		t.Error("ValidSession(tok) = true, want false after secret rotation")
	}
	fresh, err := MintSession()
	if err != nil {
		t.Fatalf("MintSession() after rotation error: %v", err)
	}
	if !ValidSession(fresh) {
		t.Error("ValidSession(fresh) = false, want true after rotation")
	}
}

// pointAt aims the package at path for one test and restores both the path and
// the loaded state afterwards, so each test starts from a fresh process view.
func pointAt(t *testing.T, path string) {
	t.Helper()
	t.Cleanup(PointAtForTest(path))
}

func tempCfg(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// gate runs one request through RequireAuth; 418 from the inner handler proves
// the request got through.
func gate(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})).ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// enabledDoc is a consistent persisted state with a password set.
func enabledDoc() string {
	sec := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x07}, 32))
	return fmt.Sprintf(`{"auth_enabled":true,"auth_password_hash":"$2a$10$fixture","auth_session_secret":%q,"auth_setup_dismissed":true}`, sec)
}

func requireOpen(t *testing.T) {
	t.Helper()
	if Locked() {
		t.Fatal("the gate is locked, want open")
	}
	if Enabled() {
		t.Fatal("the gate is enabled, want off")
	}
	if rec := gate(t, http.MethodGet, "/api/wallet/status"); rec.Code != http.StatusTeapot {
		t.Fatalf("gate answered %d while off, want the request through", rec.Code)
	}
}

func requireLocked(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Init() accepted a config it cannot trust")
	}
	if !Locked() {
		t.Fatal("the gate is open after an unreadable config, want locked")
	}
	if LockReason() == "" {
		t.Fatal("a locked gate must say why")
	}
	if Enabled() {
		t.Fatal("a locked gate must not also report enabled")
	}
}

func TestInitAbsentConfigStaysOpen(t *testing.T) {
	pointAt(t, filepath.Join(t.TempDir(), "missing.json"))
	if err := Init(); err != nil {
		t.Fatalf("Init() with no config file: %v", err)
	}
	requireOpen(t)
}

func TestInitEmptyFileStaysOpen(t *testing.T) {
	pointAt(t, tempCfg(t, ""))
	if err := Init(); err != nil {
		t.Fatalf("Init() with an empty config file: %v", err)
	}
	requireOpen(t)
}

func TestInitInvalidJSONLocks(t *testing.T) {
	pointAt(t, tempCfg(t, "{not json"))
	requireLocked(t, Init())

	rec := gate(t, http.MethodGet, "/api/wallet/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("locked gate answered %d, want 503", rec.Code)
	}
	if got := rec.Header().Get("X-Dashboard-Auth"); got != "locked" {
		t.Fatalf("X-Dashboard-Auth = %q, want locked", got)
	}
	if !strings.Contains(rec.Body.String(), "restart") {
		t.Fatalf("the refusal does not tell the operator what to do: %q", rec.Body.String())
	}
	if rec := gate(t, http.MethodGet, "/api/auth/status"); rec.Code != http.StatusTeapot {
		t.Fatalf("status probe refused with %d while locked; the UI cannot explain the lock", rec.Code)
	}
	if rec := gate(t, http.MethodPost, "/api/auth/login"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("login answered %d while locked, want 503: nothing can be logged in to", rec.Code)
	}
}

func TestInitWrongTypedKeyLocks(t *testing.T) {
	pointAt(t, tempCfg(t, `{"auth_enabled":"yes"}`))
	requireLocked(t, Init())
}

func TestInitHashWithoutSecretLocks(t *testing.T) {
	for name, doc := range map[string]string{
		"secret absent":  `{"auth_enabled":true,"auth_password_hash":"$2a$10$fixture"}`,
		"secret garbage": `{"auth_enabled":true,"auth_password_hash":"$2a$10$fixture","auth_session_secret":"%%%"}`,
	} {
		t.Run(name, func(t *testing.T) {
			pointAt(t, tempCfg(t, doc))
			requireLocked(t, Init())
		})
	}
}

func TestInitUnreadableFileLocks(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 000 file")
	}
	path := tempCfg(t, enabledDoc())
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	pointAt(t, path)
	requireLocked(t, Init())
}

// A lock must also drop credentials loaded before it, or a token minted
// earlier in the process would keep working against a gate that reports locked.
func TestLockClearsCredentials(t *testing.T) {
	pointAt(t, tempCfg(t, "{not json"))
	mu.Lock()
	enabled = true
	hash = []byte("$2a$10$fixture")
	secret = bytes.Repeat([]byte{0x01}, 32)
	mu.Unlock()
	tok, err := MintSession()
	if err != nil {
		t.Fatalf("MintSession() before the lock: %v", err)
	}

	requireLocked(t, Init())
	if ValidSession(tok) {
		t.Fatal("a session minted before the lock is still valid")
	}
	if _, err := MintSession(); err == nil {
		t.Fatal("MintSession() succeeded while locked")
	}
	if Configured() {
		t.Fatal("a locked gate still reports a configured password")
	}
}

func TestRepairThenReInitUnlocks(t *testing.T) {
	path := tempCfg(t, "{not json")
	pointAt(t, path)
	requireLocked(t, Init())

	if err := os.WriteFile(path, []byte(enabledDoc()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Init(); err != nil {
		t.Fatalf("Init() after repair: %v", err)
	}
	if Locked() {
		t.Fatal("the gate stayed locked after the config was repaired")
	}
	if !Enabled() {
		t.Fatal("the repaired config has a password set, want the gate enabled")
	}
}
