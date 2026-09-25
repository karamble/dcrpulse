// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// setPassword points the package at a scratch config and sets a password, so
// each test starts from a clean gate and a zeroed backoff counter.
func setPassword(t *testing.T, password string) {
	t.Helper()
	t.Cleanup(PointAtForTest(filepath.Join(t.TempDir(), "config.json")))
	if err := Setup(password); err != nil {
		t.Fatalf("Setup() = %v, want nil", err)
	}
}

func TestBackoffDelayCurve(t *testing.T) {
	tests := []struct {
		failures int
		want     time.Duration
	}{
		{0, 0},
		{1, 0},
		{5, 0}, // the free tries absorb honest typos
		{6, 2 * time.Second},
		{7, 4 * time.Second},
		{8, 8 * time.Second},
		{9, 16 * time.Second},
		{10, 32 * time.Second},
		{14, 512 * time.Second},
		{15, backoffCap}, // 1024s would exceed the cap
		{40, backoffCap},
		{1 << 20, backoffCap}, // the shift guard, not an overflow to a tiny value
	}
	for _, tt := range tests {
		if got := backoffDelay(tt.failures); got != tt.want {
			t.Errorf("backoffDelay(%d) = %v, want %v", tt.failures, got, tt.want)
		}
	}
}

// The free tries must not wait, or a operator who fat-fingers once is punished.
func TestVerifyFreeTriesDoNotBlock(t *testing.T) {
	setPassword(t, "correct horse")
	for i := 1; i <= backoffFreeTries; i++ {
		ok, wait := Verify("wrong")
		if ok {
			t.Fatalf("attempt %d: Verify(wrong) ok = true, want false", i)
		}
		if wait != 0 {
			t.Fatalf("attempt %d: wait = %v, want 0 within the free tries", i, wait)
		}
	}
}

// The attempt AFTER the free tries is refused without checking the password.
func TestVerifyBlocksAfterFreeTries(t *testing.T) {
	setPassword(t, "correct horse")
	for i := 0; i < backoffFreeTries; i++ {
		Verify("wrong")
	}
	// This one consumes a free try and arms the backoff.
	if ok, wait := Verify("wrong"); ok || wait != 0 {
		t.Fatalf("arming attempt: ok = %v, wait = %v, want false, 0", ok, wait)
	}
	ok, wait := Verify("wrong")
	if ok {
		t.Fatal("blocked attempt returned ok = true")
	}
	if wait <= 0 {
		t.Fatalf("blocked attempt: wait = %v, want > 0", wait)
	}
	if wait > backoffDelay(backoffFreeTries+1) {
		t.Fatalf("wait = %v, want <= %v", wait, backoffDelay(backoffFreeTries+1))
	}
	// The correct password is refused too while blocked: the point is that no
	// password is CHECKED, not that the wrong one is rejected.
	if ok, wait := Verify("correct horse"); ok || wait <= 0 {
		t.Fatalf("correct password while blocked: ok = %v, wait = %v, want false and a wait", ok, wait)
	}
}

// A success inside the free tries clears the counter, so honest use never
// accumulates toward a block.
func TestVerifySuccessResetsCounter(t *testing.T) {
	setPassword(t, "correct horse")
	for i := 0; i < backoffFreeTries; i++ {
		Verify("wrong")
	}
	if ok, wait := Verify("correct horse"); !ok || wait != 0 {
		t.Fatalf("Verify(correct) = %v, %v, want true, 0", ok, wait)
	}
	mu.RLock()
	gotFailures := loginBackoff.failures
	gotBlocked := loginBackoff.blockedUntil
	mu.RUnlock()
	if gotFailures != 0 {
		t.Errorf("failures = %d after success, want 0", gotFailures)
	}
	if !gotBlocked.IsZero() {
		t.Errorf("blockedUntil = %v after success, want zero", gotBlocked)
	}
	// And the free tries are available again from scratch.
	for i := 1; i <= backoffFreeTries; i++ {
		if _, wait := Verify("wrong"); wait != 0 {
			t.Fatalf("attempt %d after reset: wait = %v, want 0", i, wait)
		}
	}
}

// Change and Disable share Verify, so they inherit the backoff and must report
// it as ErrTooManyAttempts rather than "current password is incorrect".
func TestChangeAndDisableReportBackoff(t *testing.T) {
	for _, tt := range []struct {
		name string
		call func() error
	}{
		{"Change", func() error { return Change("wrong", "next password") }},
		{"Disable", func() error { return Disable("wrong") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setPassword(t, "correct horse")
			var err error
			for i := 0; i <= backoffFreeTries; i++ {
				err = tt.call()
				if errors.Is(err, ErrTooManyAttempts) {
					t.Fatalf("blocked after %d attempts, want the free tries to pass first", i+1)
				}
			}
			if err = tt.call(); !errors.Is(err, ErrTooManyAttempts) {
				t.Fatalf("%s() = %v, want ErrTooManyAttempts", tt.name, err)
			}
		})
	}
}

// PointAtForTest must zero the counter, or one test's failures decide whether
// the next one is allowed to check a password at all.
func TestPointAtForTestClearsBackoff(t *testing.T) {
	setPassword(t, "correct horse")
	for i := 0; i <= backoffFreeTries; i++ {
		Verify("wrong")
	}
	if _, wait := Verify("wrong"); wait <= 0 {
		t.Fatal("expected the backoff to be armed before the restore")
	}
	restore := PointAtForTest(filepath.Join(t.TempDir(), "other.json"))
	mu.RLock()
	gotFailures := loginBackoff.failures
	gotBlocked := loginBackoff.blockedUntil
	mu.RUnlock()
	restore()
	if gotFailures != 0 || !gotBlocked.IsZero() {
		t.Errorf("PointAtForTest left failures = %d, blockedUntil = %v, want 0 and zero", gotFailures, gotBlocked)
	}
}

// Guesses sent at the same moment must not all be checked: the first one holds
// the block the others would have earned (EA-4).
func TestConcurrentGuessesGetOneCheck(t *testing.T) {
	setPassword(t, "correct horse")
	mu.Lock()
	loginBackoff = backoff{failures: backoffFreeTries + 1} // blocked before, now expired
	mu.Unlock()

	start := make(chan struct{})
	results := make(chan time.Duration, 5)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, wait := Verify("wrong")
			results <- wait
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	checked := 0
	for wait := range results {
		if wait == 0 {
			checked++
		}
	}
	if checked != 1 {
		t.Fatalf("%d of 5 concurrent guesses were checked, want 1", checked)
	}
	mu.Lock()
	got := loginBackoff.failures
	mu.Unlock()
	if got != backoffFreeTries+2 {
		t.Fatalf("failures = %d, want %d", got, backoffFreeTries+2)
	}
}

// Wrong guesses at the login page do not stop a signed-in device from
// changing or disabling the password (EA-5).
func TestLoginGuessesDoNotBlockSignedInChecks(t *testing.T) {
	setPassword(t, "correct horse")
	for i := 0; i < backoffFreeTries+3; i++ {
		Verify("wrong")
	}
	if _, wait := Verify("correct horse"); wait == 0 {
		t.Fatal("login not blocked after repeated wrong guesses")
	}
	if err := Change("correct horse", "battery staple"); err != nil {
		t.Fatalf("Change while logins are blocked = %v, want nil", err)
	}
	if err := Disable("battery staple"); err != nil {
		t.Fatalf("Disable while logins are blocked = %v, want nil", err)
	}
}

// A signed-in caller guessing the current password still backs off.
func TestSignedInGuessesBackOff(t *testing.T) {
	setPassword(t, "correct horse")
	for i := 0; i <= backoffFreeTries; i++ {
		if err := Disable("wrong"); errors.Is(err, ErrTooManyAttempts) {
			t.Fatalf("attempt %d blocked within the free tries", i+1)
		}
	}
	if err := Disable("wrong"); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("Disable after %d wrong tries = %v, want ErrTooManyAttempts", backoffFreeTries+1, err)
	}
}
