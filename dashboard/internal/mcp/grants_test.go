// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
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

	if _, err := s.authorize("a", 1, dcrAtoms, "Dsaddr", now); err != errAccountNotGranted {
		t.Fatalf("account out of scope: want errAccountNotGranted, got %v", err)
	}
	if _, err := s.authorize("a", 0, 6*dcrAtoms, "Dsaddr", now); err != errPerTxExceeded {
		t.Fatalf("over per-tx: want errPerTxExceeded, got %v", err)
	}
	pass, err := s.authorize("a", 0, 5*dcrAtoms, "Dsaddr", now)
	if err != nil {
		t.Fatalf("within caps: want ok, got %v", err)
	}
	if string(pass) != "secret" {
		t.Fatalf("authorize returned wrong passphrase copy: %q", pass)
	}
	// 5 already spent today; another 5 would total 10 > 8 daily cap.
	if _, err := s.authorize("a", 0, 5*dcrAtoms, "Dsaddr", now); err != errDailyExceeded {
		t.Fatalf("over daily: want errDailyExceeded, got %v", err)
	}
	// 3 remaining is allowed.
	if _, err := s.authorize("a", 2, 3*dcrAtoms, "Dsaddr", now); err != nil {
		t.Fatalf("within remaining daily: want ok, got %v", err)
	}
}

func TestGrantDailyWindowResets(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, DailyAtoms: 5 * dcrAtoms, Passphrase: []byte("p")}, now)
	if _, err := s.authorize("a", 0, 5*dcrAtoms, "addr", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.authorize("a", 0, dcrAtoms, "addr", now); err != errDailyExceeded {
		t.Fatalf("want daily exceeded, got %v", err)
	}
	// After the 24h window elapses, the daily allowance resets.
	if _, err := s.authorize("a", 0, 5*dcrAtoms, "addr", now.Add(grantWindow+time.Minute)); err != nil {
		t.Fatalf("after window reset: want ok, got %v", err)
	}
}

func TestGrantRefund(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, DailyAtoms: 5 * dcrAtoms, Passphrase: []byte("p")}, now)
	if _, err := s.authorize("a", 0, 5*dcrAtoms, "addr", now); err != nil {
		t.Fatal(err)
	}
	s.refund("a", 5*dcrAtoms) // a failed spend frees its reserved headroom
	if _, err := s.authorize("a", 0, 5*dcrAtoms, "addr", now); err != nil {
		t.Fatalf("after refund: want ok, got %v", err)
	}
}

func TestGrantAllowlist(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, Allowlist: []string{"Dsgood"}, Passphrase: []byte("p")}, now)
	if _, err := s.authorize("a", 0, 1, "Dsbad", now); err != errAddrNotAllowed {
		t.Fatalf("want addr not allowed, got %v", err)
	}
	if _, err := s.authorize("a", 0, 1, "Dsgood", now); err != nil {
		t.Fatalf("allowlisted addr: want ok, got %v", err)
	}
}

func TestGrantExpiryZeroesPassphrase(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, Expiry: now.Add(-time.Minute), Passphrase: []byte("zerome")}, now)
	g := s.byAgent["a"]
	if _, err := s.authorize("a", 0, 1, "addr", now); err != errGrantExpired {
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
	if _, err := s.authorize("nope", 0, 1, "addr", time.Now()); err != errNoGrant {
		t.Fatalf("want no-grant, got %v", err)
	}
}
