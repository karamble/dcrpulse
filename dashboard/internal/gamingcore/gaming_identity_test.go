// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package gamingcore

import (
	"os"
	"reflect"
	"testing"
)

func TestGamingCredentialPersistenceOrdering(t *testing.T) {
	br := newTestBridge(t)
	settings := DefaultGamingSettings()
	settings.RegisteredGames = []string{"poker"}
	if err := br.writeGamingSettingsLocked(settings); err != nil {
		t.Fatal(err)
	}
	old, err := br.IssueGamingCredential("poker")
	if err != nil {
		t.Fatal(err)
	}
	before := br.ReadGamingSettings()
	// Reads continue to work, but the atomic writer cannot create its temp file.
	if err := os.Mkdir(br.gamingSettingsPath()+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := br.IssueGamingCredential("poker"); err == nil {
		t.Fatal("issuance ignored persistence failure")
	}
	if game, ok := br.gamingAllow.Resolve(old.Fingerprint); !ok || game != "poker" {
		t.Fatal("failed issuance retired old credential")
	}
	if !reflect.DeepEqual(br.ReadGamingSettings(), before) {
		t.Fatal("failed issuance changed saved credential")
	}
	if err := os.Remove(br.gamingSettingsPath() + ".tmp"); err != nil {
		t.Fatal(err)
	}
	fresh, err := br.IssueGamingCredential("poker")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := br.gamingAllow.Resolve(old.Fingerprint); ok {
		t.Fatal("successful issuance retained old credential")
	}
	if _, ok := br.gamingAllow.Resolve(fresh.Fingerprint); !ok {
		t.Fatal("replacement not admitted")
	}
	// Explicit revocation intentionally takes effect even if its save fails.
	if err := os.Mkdir(br.gamingSettingsPath()+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if err := br.RevokeGamingCredential("poker"); err == nil {
		t.Fatal("revoke ignored persistence failure")
	}
	if _, ok := br.gamingAllow.Resolve(fresh.Fingerprint); ok {
		t.Fatal("failed persistence undid explicit revocation")
	}
}
