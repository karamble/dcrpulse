// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package auth

import (
	"os"
	"testing"
)

// Logout has no session table to strike a row from: the only revocation the
// stateless cookie allows is a new HMAC secret, which is what Revoke does and
// why it signs out every device at once.

func TestRevokeInvalidatesTheSession(t *testing.T) {
	pointAt(t, tempCfg(t, enabledDoc()))
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	tok, err := MintSession()
	if err != nil {
		t.Fatal(err)
	}
	if !ValidSession(tok) {
		t.Fatal("a fresh token does not verify")
	}
	if err := Revoke(); err != nil {
		t.Fatalf("Revoke() = %v", err)
	}
	if ValidSession(tok) {
		t.Fatal("the token still verifies after logout; a captured copy would keep working")
	}
	fresh, err := MintSession()
	if err != nil {
		t.Fatal(err)
	}
	if !ValidSession(fresh) {
		t.Fatal("a token minted after logout does not verify")
	}
}

// A restart reloads the secret from disk. If the rotation was not persisted,
// the old cookie comes back to life here.
func TestRevokeSurvivesARestart(t *testing.T) {
	pointAt(t, tempCfg(t, enabledDoc()))
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	tok, err := MintSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := Revoke(); err != nil {
		t.Fatalf("Revoke() = %v", err)
	}
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	if ValidSession(tok) {
		t.Fatal("the pre-logout token verifies again after a restart; the rotated secret was not persisted")
	}
}

func TestRevokeIsANoOpWhileTheGateIsOff(t *testing.T) {
	path := tempCfg(t, "")
	pointAt(t, path)
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	requireOpen(t)
	if err := Revoke(); err != nil {
		t.Fatalf("Revoke() = %v with the gate off, want nil", err)
	}
	if b, err := os.ReadFile(path); err != nil || len(b) != 0 {
		t.Fatalf("logout wrote %q into the config while the gate is off", b)
	}
}

// Persist first, then rotate in memory. The other order would leave a cookie
// revoked until the next restart and valid again after it.
func TestRevokeKeepsTheOldSecretWhenItCannotPersist(t *testing.T) {
	path := tempCfg(t, enabledDoc())
	pointAt(t, path)
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	tok, err := MintSession()
	if err != nil {
		t.Fatal(err)
	}
	// A directory where the file was: the load inside persistLocked fails
	// before anything is written, on any account, with nothing to clean up.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Revoke(); err == nil {
		t.Fatal("Revoke() = nil although the config could not be written")
	}
	if !ValidSession(tok) {
		t.Fatal("the secret rotated in memory although it was not persisted; a restart would resurrect the old cookie")
	}
}
