// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"testing"
	"time"
)

const dcrAtoms = 100_000_000 // 1 DCR

func TestGrantAuthorizeScopeAndCaps(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{
		Accounts:   []uint32{0, 2},
		PerTxAtoms: 5 * dcrAtoms,
		DailyAtoms: 8 * dcrAtoms,
		Passphrase: []byte("secret"),
	}, now)

	if _, err := s.authorize(context.Background(), "a", 1, dcrAtoms, "Dsaddr", now); err != errAccountNotGranted {
		t.Fatalf("account out of scope: want errAccountNotGranted, got %v", err)
	}
	if _, err := s.authorize(context.Background(), "a", 0, 6*dcrAtoms, "Dsaddr", now); err != errPerTxExceeded {
		t.Fatalf("over per-tx: want errPerTxExceeded, got %v", err)
	}
	pass, err := s.authorize(context.Background(), "a", 0, 5*dcrAtoms, "Dsaddr", now)
	if err != nil {
		t.Fatalf("within caps: want ok, got %v", err)
	}
	if string(pass) != "secret" {
		t.Fatalf("authorize returned wrong passphrase copy: %q", pass)
	}
	// 5 already spent today; another 5 would total 10 > 8 daily cap.
	if _, err := s.authorize(context.Background(), "a", 0, 5*dcrAtoms, "Dsaddr", now); err != errDailyExceeded {
		t.Fatalf("over daily: want errDailyExceeded, got %v", err)
	}
	// 3 remaining is allowed.
	if _, err := s.authorize(context.Background(), "a", 2, 3*dcrAtoms, "Dsaddr", now); err != nil {
		t.Fatalf("within remaining daily: want ok, got %v", err)
	}
}

func TestGrantDailyWindowResets(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, PerTxAtoms: 5 * dcrAtoms, DailyAtoms: 5 * dcrAtoms, Passphrase: []byte("p")}, now)
	if _, err := s.authorize(context.Background(), "a", 0, 5*dcrAtoms, "addr", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.authorize(context.Background(), "a", 0, dcrAtoms, "addr", now); err != errDailyExceeded {
		t.Fatalf("want daily exceeded, got %v", err)
	}
	// After the 24h window elapses, the daily allowance resets.
	if _, err := s.authorize(context.Background(), "a", 0, 5*dcrAtoms, "addr", now.Add(grantWindow+time.Minute)); err != nil {
		t.Fatalf("after window reset: want ok, got %v", err)
	}
}

func TestGrantRefund(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, PerTxAtoms: 5 * dcrAtoms, DailyAtoms: 5 * dcrAtoms, Passphrase: []byte("p")}, now)
	if _, err := s.authorize(context.Background(), "a", 0, 5*dcrAtoms, "addr", now); err != nil {
		t.Fatal(err)
	}
	s.refund("a", 5*dcrAtoms) // a failed spend frees its reserved headroom
	if _, err := s.authorize(context.Background(), "a", 0, 5*dcrAtoms, "addr", now); err != nil {
		t.Fatalf("after refund: want ok, got %v", err)
	}
}

func TestGrantAllowlist(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, PerTxAtoms: dcrAtoms, DailyAtoms: dcrAtoms, Allowlist: []string{"Dsgood"}, Passphrase: []byte("p")}, now)
	if _, err := s.authorize(context.Background(), "a", 0, 1, "Dsbad", now); err != errAddrNotAllowed {
		t.Fatalf("want addr not allowed, got %v", err)
	}
	if _, err := s.authorize(context.Background(), "a", 0, 1, "Dsgood", now); err != nil {
		t.Fatalf("allowlisted addr: want ok, got %v", err)
	}
}

func TestGrantZeroCapDeniesSpend(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	// A 0 cap is a literal limit (no unlimited): nothing is spendable.
	s.set("a", GrantSpec{Accounts: []uint32{0}, PerTxAtoms: 0, DailyAtoms: 5 * dcrAtoms, Passphrase: []byte("p")}, now)
	if _, err := s.authorize(context.Background(), "a", 0, 1, "addr", now); err != errPerTxExceeded {
		t.Fatalf("0 per-tx cap must deny any spend, got %v", err)
	}
}

func TestGrantExpiryZeroesPassphrase(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, Expiry: now.Add(-time.Minute), Passphrase: []byte("zerome")}, now)
	g := s.byAgent["a"]
	if _, err := s.authorize(context.Background(), "a", 0, 1, "addr", now); err != errGrantExpired {
		t.Fatalf("want expired, got %v", err)
	}
	for _, b := range g.passphrase {
		if b != 0 {
			t.Fatal("passphrase not zeroed on expiry")
		}
	}
	if _, ok := s.info("a"); ok {
		t.Fatal("expired grant should be removed")
	}
}

func TestGrantRevokeZeroesPassphrase(t *testing.T) {
	s := newGrantStore()
	s.set("a", GrantSpec{Accounts: []uint32{0}, Passphrase: []byte("zerome")}, time.Now())
	g := s.byAgent["a"]
	if !s.revoke("a") {
		t.Fatal("revoke of existing grant should return true")
	}
	for _, b := range g.passphrase {
		if b != 0 {
			t.Fatal("passphrase not zeroed on revoke")
		}
	}
	if s.revoke("a") {
		t.Fatal("revoke of missing grant should return false")
	}
}

func TestGrantNoGrantDenied(t *testing.T) {
	s := newGrantStore()
	if _, err := s.authorize(context.Background(), "nope", 0, 1, "addr", time.Now()); err != errNoGrant {
		t.Fatalf("want no-grant, got %v", err)
	}
}

func TestGrantVotingRequiresScope(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, Passphrase: []byte("secret")}, now)
	if _, err := s.authorizeActionPass("a", scopeGovernance, now); err == nil {
		t.Fatal("voting without scope: want denial, got nil")
	}
	s.set("a", GrantSpec{Accounts: []uint32{0}, Passphrase: []byte("secret"), WriteScopes: []string{scopeGovernance}}, now)
	pass, err := s.authorizeActionPass("a", scopeGovernance, now)
	if err != nil || string(pass) != "secret" {
		t.Fatalf("voting with scope: want passphrase copy, got %q err=%v", pass, err)
	}
}

func TestGrantLightningRequiresScopeAndCap(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{PerTxAtoms: 5 * dcrAtoms, DailyAtoms: 5 * dcrAtoms, Passphrase: []byte("p")}, now)
	if err := s.authorizeLightning(context.Background(), "a", dcrAtoms, now); err == nil {
		t.Fatal("LN without scope: want denial, got nil")
	}
	s.set("a", GrantSpec{PerTxAtoms: 5 * dcrAtoms, DailyAtoms: 5 * dcrAtoms, Passphrase: []byte("p"), WriteScopes: []string{scopeLightning}}, now)
	if err := s.authorizeLightning(context.Background(), "a", 4*dcrAtoms, now); err != nil {
		t.Fatalf("LN within cap: want ok, got %v", err)
	}
	if err := s.authorizeLightning(context.Background(), "a", 2*dcrAtoms, now); err != errDailyExceeded {
		t.Fatalf("LN over shared daily cap: want errDailyExceeded, got %v", err)
	}
	if err := s.authorizeAction("a", scopeLightning, now); err != nil {
		t.Fatalf("LN non-spend action with scope: want ok, got %v", err)
	}
}

func TestGrantDexRequiresScope(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Passphrase: []byte("p")}, now)
	if err := s.authorizeAction("a", scopeDex, now); err == nil {
		t.Fatal("DEX without scope: want denial, got nil")
	}
	s.set("a", GrantSpec{Passphrase: []byte("p"), WriteScopes: []string{scopeDex}}, now)
	if err := s.authorizeAction("a", scopeDex, now); err != nil {
		t.Fatalf("DEX with scope: want ok, got %v", err)
	}
}

func TestGrantSpendScopedCapAndScope(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{PerTxAtoms: 5 * dcrAtoms, DailyAtoms: 5 * dcrAtoms, WriteScopes: []string{scopeDexSpend}}, now)
	if err := s.authorizeSpendScoped(context.Background(), "a", scopeDexSpend, 6*dcrAtoms, now); err != errPerTxExceeded {
		t.Fatalf("over per-tx: want errPerTxExceeded, got %v", err)
	}
	if err := s.authorizeSpendScoped(context.Background(), "a", scopeDexSpend, 4*dcrAtoms, now); err != nil {
		t.Fatalf("DCR move within cap: want ok, got %v", err)
	}
	// A non-DCR move (amount 0) is scope-gated only, not cap-reserved.
	if err := s.authorizeSpendScoped(context.Background(), "a", scopeDexSpend, 0, now); err != nil {
		t.Fatalf("non-DCR scoped move: want ok, got %v", err)
	}
	// Without the scope, denied even for a non-DCR move.
	s.set("a", GrantSpec{PerTxAtoms: dcrAtoms, DailyAtoms: dcrAtoms, WriteScopes: []string{scopeDex}}, now)
	if err := s.authorizeSpendScoped(context.Background(), "a", scopeDexSpend, 0, now); err == nil {
		t.Fatal("dex.spend without scope: want denial, got nil")
	}
}
