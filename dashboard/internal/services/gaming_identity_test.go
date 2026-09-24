// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"os"
	"reflect"
	"testing"

	"dcrpulse/internal/gamingbridge"
)

func TestGamingCredentialPersistenceOrdering(t *testing.T) {
	oldDir, oldAllow := GamingStateDir, gamingAllow
	GamingStateDir = t.TempDir()
	gamingAllow = gamingbridge.NewAllowlist()
	t.Cleanup(func() { GamingStateDir = oldDir; gamingAllow = oldAllow })
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker"}
	if err := writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
	old, err := IssueGamingCredential("poker")
	if err != nil {
		t.Fatal(err)
	}
	before := ReadGamingSettings()
	// Reads continue to work, but the atomic writer cannot create its temp file.
	if err := os.Mkdir(gamingSettingsPath()+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := IssueGamingCredential("poker"); err == nil {
		t.Fatal("issuance ignored persistence failure")
	}
	if game, ok := gamingAllow.Resolve(old.Fingerprint); !ok || game != "poker" {
		t.Fatal("failed issuance retired old credential")
	}
	if !reflect.DeepEqual(ReadGamingSettings(), before) {
		t.Fatal("failed issuance changed saved credential")
	}
	if err := os.Remove(gamingSettingsPath() + ".tmp"); err != nil {
		t.Fatal(err)
	}
	fresh, err := IssueGamingCredential("poker")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gamingAllow.Resolve(old.Fingerprint); ok {
		t.Fatal("successful issuance retained old credential")
	}
	if _, ok := gamingAllow.Resolve(fresh.Fingerprint); !ok {
		t.Fatal("replacement not admitted")
	}
	// Explicit revocation intentionally takes effect even if its save fails.
	if err := os.Mkdir(gamingSettingsPath()+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if err := RevokeGamingCredential("poker"); err == nil {
		t.Fatal("revoke ignored persistence failure")
	}
	if _, ok := gamingAllow.Resolve(fresh.Fingerprint); ok {
		t.Fatal("failed persistence undid explicit revocation")
	}
}
