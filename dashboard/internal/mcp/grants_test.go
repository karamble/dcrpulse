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

// TestGrantScopedRejectsNegative covers the overflow path: an unsigned tool
// amount above MaxInt64 wraps negative, and a negative must not slip past the
// caps the way a legitimate zero (non-DCR) does.
func TestGrantScopedRejectsNegative(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{PerTxAtoms: dcrAtoms, DailyAtoms: dcrAtoms, WriteScopes: []string{scopeDexSpend}}, now)
	if err := s.authorizeSpendScoped(context.Background(), "a", scopeDexSpend, -1, "", now); err != errBadAmount {
		t.Fatalf("negative scoped amount: want errBadAmount, got %v", err)
	}
	// The reservation must not have run.
	if got := s.byAgent["a"].spentAtoms; got != 0 {
		t.Fatalf("negative amount moved the spend counter: got %d, want 0", got)
	}
}

// TestGrantEditKeepsSpentWindow pins that editing a grant does not refill the
// day's allowance: the window belongs to the agent's spending, not to the grant.
func TestGrantEditKeepsSpentWindow(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	spec := GrantSpec{Accounts: []uint32{0}, PerTxAtoms: 5 * dcrAtoms, DailyAtoms: 5 * dcrAtoms}
	s.set("a", spec, now)
	if _, err := s.authorize(context.Background(), "a", 0, 5*dcrAtoms, "", now); err != nil {
		t.Fatalf("spend to the cap: %v", err)
	}
	// Re-grant with an extra allowlist entry - an edit that touches nothing
	// about the caps.
	spec.Allowlist = []string{"Dsomething"}
	s.set("a", spec, now)
	if _, err := s.authorize(context.Background(), "a", 0, dcrAtoms, "Dsomething", now); err != errDailyExceeded {
		t.Fatalf("after an edit the day should still be spent: want errDailyExceeded, got %v", err)
	}
	// A fresh window after 24h still resets.
	later := now.Add(grantWindow + time.Minute)
	s.set("a", spec, later)
	if _, err := s.authorize(context.Background(), "a", 0, dcrAtoms, "Dsomething", later); err != nil {
		t.Fatalf("new window: want ok, got %v", err)
	}
}

// TestGrantRefundIgnoresNonPositive pins the counter-reset fix: a negative
// refund would otherwise raise spentAtoms and the <0 clamp would then zero the
// whole window's usage.
func TestGrantRefundIgnoresNonPositive(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{Accounts: []uint32{0}, PerTxAtoms: 5 * dcrAtoms, DailyAtoms: 5 * dcrAtoms}, now)
	if _, err := s.authorize(context.Background(), "a", 0, 4*dcrAtoms, "", now); err != nil {
		t.Fatalf("spend within cap: %v", err)
	}
	s.refund("a", -9_000_000_000_000_000_000)
	s.refund("a", 0)
	if got := s.byAgent["a"].spentAtoms; got != 4*dcrAtoms {
		t.Fatalf("non-positive refund changed the spend counter: got %d, want %d", got, 4*dcrAtoms)
	}
	// The remaining headroom is still only 1 DCR.
	if _, err := s.authorize(context.Background(), "a", 0, 2*dcrAtoms, "", now); err != errDailyExceeded {
		t.Fatalf("after bogus refunds: want errDailyExceeded, got %v", err)
	}
}

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
	if err := s.authorizeSpendScoped(context.Background(), "a", scopeDexSpend, 6*dcrAtoms, "", now); err != errPerTxExceeded {
		t.Fatalf("over per-tx: want errPerTxExceeded, got %v", err)
	}
	if err := s.authorizeSpendScoped(context.Background(), "a", scopeDexSpend, 4*dcrAtoms, "", now); err != nil {
		t.Fatalf("DCR move within cap: want ok, got %v", err)
	}
	// A non-DCR move (amount 0) is scope-gated only, not cap-reserved.
	if err := s.authorizeSpendScoped(context.Background(), "a", scopeDexSpend, 0, "", now); err != nil {
		t.Fatalf("non-DCR scoped move: want ok, got %v", err)
	}
	// Without the scope, denied even for a non-DCR move.
	s.set("a", GrantSpec{PerTxAtoms: dcrAtoms, DailyAtoms: dcrAtoms, WriteScopes: []string{scopeDex}}, now)
	if err := s.authorizeSpendScoped(context.Background(), "a", scopeDexSpend, 0, "", now); err == nil {
		t.Fatal("dex.spend without scope: want denial, got nil")
	}
}

func TestGrantActionGated(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	s.set("a", GrantSpec{WriteScopes: []string{scopeDex}}, now)
	if err := s.authorizeActionGated(context.Background(), "a", scopeDexSpend, "arm it", now); err == nil {
		t.Fatal("arming without scope: want denial, got nil")
	}
	s.set("a", GrantSpec{WriteScopes: []string{scopeDexSpend}}, now)
	if err := s.authorizeActionGated(context.Background(), "a", scopeDexSpend, "arm it", now); err != nil {
		t.Fatalf("arming with scope: want ok, got %v", err)
	}
}

func TestGrantVSPFees(t *testing.T) {
	s := newGrantStore()
	now := time.Now()
	full := GrantSpec{
		Accounts:    []uint32{0, 1},
		PerTxAtoms:  5 * dcrAtoms,
		DailyAtoms:  5 * dcrAtoms,
		Passphrase:  []byte("secret"),
		WriteScopes: []string{scopeStaking},
	}

	// The scope alone is not enough: the fee comes out of an account.
	s.set("a", GrantSpec{PerTxAtoms: 5 * dcrAtoms, DailyAtoms: 5 * dcrAtoms, Passphrase: []byte("secret"), WriteScopes: []string{scopeStaking}}, now)
	if _, err := s.authorizeVSPFees(context.Background(), "a", 0, 0, dcrAtoms, "", now); err != errAccountNotGranted {
		t.Fatalf("ungranted fee account: want errAccountNotGranted, got %v", err)
	}

	// An account the grant covers is not enough if the change account escapes it.
	s.set("a", full, now)
	if _, err := s.authorizeVSPFees(context.Background(), "a", 0, 7, dcrAtoms, "", now); err != errAccountNotGranted {
		t.Fatalf("ungranted change account: want errAccountNotGranted, got %v", err)
	}

	// Without the staking scope the accounts do not help.
	s.set("a", GrantSpec{Accounts: []uint32{0, 1}, PerTxAtoms: 5 * dcrAtoms, DailyAtoms: 5 * dcrAtoms, Passphrase: []byte("secret")}, now)
	if _, err := s.authorizeVSPFees(context.Background(), "a", 0, 1, dcrAtoms, "", now); err == nil {
		t.Fatal("VSP fees without scope: want denial, got nil")
	}

	// A ceiling over the per-transaction cap is refused and reserves nothing.
	s.set("a", full, now)
	if _, err := s.authorizeVSPFees(context.Background(), "a", 0, 1, 6*dcrAtoms, "", now); err != errPerTxExceeded {
		t.Fatalf("ceiling over per-tx: want errPerTxExceeded, got %v", err)
	}
	if got := s.byAgent["a"].spentAtoms; got != 0 {
		t.Fatalf("refused ceiling reserved %d atoms, want 0", got)
	}

	// Within the caps: the passphrase comes back and the ceiling is reserved.
	pass, err := s.authorizeVSPFees(context.Background(), "a", 0, 1, 4*dcrAtoms, "", now)
	if err != nil || string(pass) != "secret" {
		t.Fatalf("VSP fees within caps: want passphrase copy, got %q err=%v", pass, err)
	}
	if got := s.byAgent["a"].spentAtoms; got != 4*dcrAtoms {
		t.Fatalf("reserved %d atoms, want %d", got, 4*dcrAtoms)
	}

	// A run with nothing to pay for is refused rather than authorized: the callee
	// settles whatever the VSP still asks for, so "no local candidates" is not
	// "no payment", and a zero ceiling would hand over the passphrase with no cap
	// behind it. A fresh store because re-granting carries the spend window over.
	s2 := newGrantStore()
	s2.set("a", full, now)
	if _, err := s2.authorizeVSPFees(context.Background(), "a", 0, 1, 0, "", now); err != errBadAmount {
		t.Fatalf("zero ceiling: want errBadAmount, got %v", err)
	}
	if got := s2.byAgent["a"].spentAtoms; got != 0 {
		t.Fatalf("refused run reserved %d atoms, want 0", got)
	}
}

// Switching agent access off has to release the wallet passphrases the grants
// hold. The teardown path already ends listen streams, stops the bridge feed
// and cancels parked approvals; the held secret was the one thing left behind,
// and an operator who turns the surface off expects it gone.
func TestDisablingMCPZeroesHeldPassphrases(t *testing.T) {
	prev := grants
	grants = newGrantStore()
	t.Cleanup(func() { grants = prev })

	grants.set("a", GrantSpec{Accounts: []uint32{0}, Passphrase: []byte("zerome")}, time.Now())
	g := grants.byAgent["a"]

	// persistEnabled writes the dashboard config, which is absent under test;
	// the release happens before it, so its error is not what is under test.
	_ = SetEnabled(false)

	for _, b := range g.passphrase {
		if b != 0 {
			t.Fatal("turning MCP off left a wallet passphrase readable in memory")
		}
	}
	if _, ok := grants.info("a"); ok {
		t.Fatal("the grant survived the surface being turned off")
	}
}
