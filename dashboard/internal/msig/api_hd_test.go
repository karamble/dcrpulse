// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"strings"
	"testing"
)

// SharedAccounts must name every dedicated account of the active
// wallet's registry so spend selectors can hide the key reservoirs.
func TestSharedAccountsListsDedicatedAccounts(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")

	hd.as("alice")
	if got := SharedAccounts(hd.ctx); len(got) != 0 {
		t.Fatalf("accounts before any wallet: %v", got)
	}

	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	hd.pump()

	// The harness hands each node account number 1 for its first
	// dedicated account.
	hd.as("alice")
	if got := SharedAccounts(hd.ctx); !got[1] || len(got) != 1 {
		t.Fatalf("alice shared accounts: %v", got)
	}
	hd.as("bob")
	if got := SharedAccounts(hd.ctx); !got[1] || len(got) != 1 {
		t.Fatalf("bob shared accounts: %v", got)
	}
}

// The restore matrix: a backup card lands on a fresh wallet built from
// the same seed. The dedicated account is gone, so the first attempt
// asks for the passphrase, the second recreates the account, proves it
// by xpub equality and re-imports the ladder windows.
func TestRestoreBackupCardOnFreshSeed(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "restorer")
	// The restorer is alice's seed in a fresh wallet.
	hd.masters[hd.nodeByNick("restorer").uid] = hd.masters[hd.nodeByNick("alice").uid]

	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	hd.settle(t, tempID, "alice", "bob")

	hd.as("alice")
	card, err := ExportBackupCard(hd.ctx, tempID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if card.CardVersion != backupCardVersion || card.DerivationScheme != DerivationScheme {
		t.Fatalf("card metadata: %+v", card)
	}

	origRescan := rescanSeam
	t.Cleanup(func() { rescanSeam = origRescan })
	var rescans []int64
	rescanSeam = func(beginHeight int64) { rescans = append(rescans, beginHeight) }

	// Version and scheme gates.
	hd.as("restorer")
	tooNew := *card
	tooNew.CardVersion = backupCardVersion + 1
	if _, err := ImportBackupCard(hd.ctx, &tooNew, nil); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("future card accepted: %v", err)
	}
	wrongScheme := *card
	wrongScheme.DerivationScheme = "someone-elses-scheme"
	if _, err := ImportBackupCard(hd.ctx, &wrongScheme, nil); err == nil || !strings.Contains(err.Error(), "derivation scheme") {
		t.Fatalf("foreign scheme accepted: %v", err)
	}

	// No account holds the xpub and no passphrase was given.
	if _, err := ImportBackupCard(hd.ctx, card, nil); err != ErrRestoreNeedsPassphrase {
		t.Fatalf("expected passphrase demand, got %v", err)
	}

	// With the passphrase the account is recreated and proven.
	before := len(hd.nodeByNick("restorer").imports)
	rec, err := ImportBackupCard(hd.ctx, card, []byte("wallet-pass"))
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if rec.Status != StatusActive || rec.Address != card.Record.Address {
		t.Fatalf("restored record: %s %s", rec.Status, rec.Address)
	}
	if got := len(hd.nodeByNick("restorer").imports) - before; got != int(GapExt+GapInt) {
		t.Fatalf("restore imported %d scripts, want %d", got, GapExt+GapInt)
	}
	if len(rescans) != 1 || rescans[0] != card.Record.CreatedHeight-1 {
		t.Fatalf("deferred rescan: %v (created height %d)", rescans, card.Record.CreatedHeight)
	}
	if len(hd.accounts[hd.nodeByNick("restorer").uid]) == 0 {
		t.Fatalf("restore did not recreate the account")
	}

	// A second restore of the same card is refused.
	if _, err := ImportBackupCard(hd.ctx, card, []byte("wallet-pass")); err == nil {
		t.Fatalf("duplicate restore accepted")
	}
}

// A wallet whose seed did not produce the card's xpub must be refused
// even with a passphrase.
func TestRestoreBackupCardWrongSeed(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "stranger")

	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	hd.settle(t, tempID, "alice", "bob")

	hd.as("alice")
	card, err := ExportBackupCard(hd.ctx, tempID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	origRescan := rescanSeam
	t.Cleanup(func() { rescanSeam = origRescan })
	rescanSeam = func(int64) {}

	hd.as("stranger")
	_, err = ImportBackupCard(hd.ctx, card, []byte("wallet-pass"))
	if err == nil || !strings.Contains(err.Error(), "seed") {
		t.Fatalf("wrong seed accepted: %v", err)
	}
}

// A restore into a wallet where the dedicated account already exists —
// even under a different name — locates it by xpub without a passphrase.
func TestRestoreBackupCardLocatesExistingAccount(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "twin")
	twin := hd.nodeByNick("twin")
	hd.masters[twin.uid] = hd.masters[hd.nodeByNick("alice").uid]

	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	hd.settle(t, tempID, "alice", "bob")

	hd.as("alice")
	card, err := ExportBackupCard(hd.ctx, tempID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// The twin wallet (same seed) already carries account 1 under an
	// unrelated, renamed label; the registry record is absent.
	hd.accounts[twin.uid] = []string{"renamed-by-hand"}

	origRescan := rescanSeam
	t.Cleanup(func() { rescanSeam = origRescan })
	rescanSeam = func(int64) {}

	hd.as("twin")
	rec, err := ImportBackupCard(hd.ctx, card, nil)
	if err != nil {
		t.Fatalf("restore without passphrase: %v", err)
	}
	if rec.OwnHD.Account != card.Record.OwnHD.Account {
		t.Fatalf("account relocation mismatch: %d != %d", rec.OwnHD.Account, card.Record.OwnHD.Account)
	}
}
