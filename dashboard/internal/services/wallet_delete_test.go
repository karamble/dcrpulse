// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A wallet's Lightning node, Bison Relay identity and DEX profile are filed
// under its name. Deleting the wallet must take them along, and a new wallet
// with that name must never inherit them.

func fakeWalletServiceDirs(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	oldDirs, oldPurge := walletServiceDirs, walletPurgeDir
	walletServiceDirs = func(name string) []walletServiceDir {
		return []walletServiceDir{
			{lightningDataLabel, filepath.Join(root, "dcrlnd", "wallets", name), true},
			{bisonRelayDataLabel, filepath.Join(root, "brclientd", "wallets", name), false},
			{dexDataLabel, filepath.Join(root, "dcrdex", "wallets", name), false},
		}
	}
	walletPurgeDir = func() string { return filepath.Join(root, "control", "purge") }
	t.Cleanup(func() { walletServiceDirs, walletPurgeDir = oldDirs, oldPurge })
	return root
}

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(d, "data"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDeleteRemovesLightningAndMarksTheRest(t *testing.T) {
	root := fakeWalletServiceDirs(t)
	ln := filepath.Join(root, "dcrlnd", "wallets", "alice")
	br := filepath.Join(root, "brclientd", "wallets", "alice")
	mkdirs(t, ln, br)

	if err := removeWalletServiceData("alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ln); !os.IsNotExist(err) {
		t.Errorf("lightning tree still there: %v", err)
	}
	if _, err := os.Stat(br); err != nil {
		t.Errorf("dashboard removed the read-only bison relay tree itself: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "control", "purge", "alice")); err != nil {
		t.Errorf("no removal marker for the supervisors: %v", err)
	}
}

func TestCreateRefusedWhileDataRemains(t *testing.T) {
	root := fakeWalletServiceDirs(t)
	mkdirs(t, filepath.Join(root, "brclientd", "wallets", "bob"), filepath.Join(root, "dcrdex", "wallets", "bob"))

	err := claimWalletServiceData("bob")
	if err == nil || !strings.Contains(err.Error(), "Bison Relay, DCRDEX data of an earlier wallet") {
		t.Fatalf("orphaned data: got %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "control", "purge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "control", "purge", "bob"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err = claimWalletServiceData("bob")
	if err == nil || !strings.Contains(err.Error(), "still being removed (Bison Relay, DCRDEX data remain)") {
		t.Fatalf("pending removal: got %v", err)
	}
}

func TestCreateClearsStaleMarker(t *testing.T) {
	root := fakeWalletServiceDirs(t)
	marker := filepath.Join(root, "control", "purge", "carol")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := claimWalletServiceData("carol"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("stale marker survived, a supervisor would remove the new wallet's data: %v", err)
	}
	if err := claimWalletServiceData("carol"); err != nil {
		t.Errorf("no marker at all: %v", err)
	}
}

func TestLeftoverWalletData(t *testing.T) {
	root := fakeWalletServiceDirs(t)
	mkdirs(t, filepath.Join(root, "dcrlnd", "wallets", "dave"), filepath.Join(root, "dcrdex", "wallets", "dave"))

	if got := strings.Join(leftoverWalletData("dave"), ","); got != "Lightning,DCRDEX" {
		t.Errorf("leftoverWalletData = %q, want Lightning,DCRDEX", got)
	}
	// The default wallet's trees are the service roots; they are never its own.
	mkdirs(t, filepath.Join(root, "dcrlnd", "wallets", "default-wallet"))
	if got := leftoverWalletData("default-wallet"); got != nil {
		t.Errorf("default wallet reported %v", got)
	}
}

// A delete cut short must leave the wallet listed, so it can be deleted again,
// with its removal already marked.
func TestDeleteRemovesTheWalletLast(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission this test relies on")
	}
	root := fakeWalletServiceDirs(t)
	lnParent := filepath.Join(root, "dcrlnd", "wallets")
	walletData := filepath.Join(root, "dcrwallet", "wallets", "erin")
	mkdirs(t, filepath.Join(lnParent, "erin"), walletData)
	if err := os.Chmod(lnParent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lnParent, 0o755) })

	if err := removeWalletFiles("erin", walletData, filepath.Join(root, "config", "erin")); err == nil {
		t.Fatal("the Lightning tree could not be removed, yet the delete reported success")
	}
	if _, err := os.Stat(walletData); err != nil {
		t.Errorf("wallet data removed before the rest, the wallet can no longer be deleted again: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "control", "purge", "erin")); err != nil {
		t.Errorf("no removal marker: %v", err)
	}
}
