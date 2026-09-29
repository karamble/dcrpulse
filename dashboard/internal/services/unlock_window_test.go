// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// The window opens every account, runs the signing step with the window lock
// held, then re-locks what it opened before another window can start.
func TestWithUnlockedAccountsRunsBetweenUnlockAndRelock(t *testing.T) {
	f := &fakeVSPWallet{}
	withFakeVSPWallet(t, f)
	opsBefore := unlockedOps.Load()

	err := withUnlockedAccounts(context.Background(), []byte("hunter2"), func() error {
		if len(vspSignSem) != 1 {
			t.Error("the window lock is not held while signing")
		}
		if unlockedOps.Load() <= opsBefore {
			t.Error("the lock sweep is not held off while signing")
		}
		f.record("sign")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"unlock:0", "unlock:1", "sign", "lock:0", "lock:1"}
	if !reflect.DeepEqual(f.events, want) {
		t.Fatalf("events %v, want %v", f.events, want)
	}
	if len(vspSignSem) != 0 || unlockedOps.Load() != opsBefore {
		t.Fatal("the window was not closed")
	}
}

func TestWithUnlockedAccountsWrongPassphrase(t *testing.T) {
	f := &fakeVSPWallet{}
	withFakeVSPWallet(t, f)
	opsBefore := unlockedOps.Load()

	ran := false
	err := withUnlockedAccounts(context.Background(), []byte("wrong"), func() error {
		ran = true
		return nil
	})
	if !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("err = %v, want ErrWrongPassphrase", err)
	}
	if ran {
		t.Fatal("the signing step ran without unlocked accounts")
	}
	if len(vspSignSem) != 0 || unlockedOps.Load() != opsBefore {
		t.Fatal("a failed unlock left the window open")
	}
}

func TestWithUnlockedAccountsRelocksWhenSigningFails(t *testing.T) {
	f := &fakeVSPWallet{}
	withFakeVSPWallet(t, f)
	signErr := errors.New("SignMessages: unavailable")

	err := withUnlockedAccounts(context.Background(), []byte("hunter2"), func() error {
		return signErr
	})
	if err != signErr {
		t.Fatalf("err = %v, want the signing error unchanged", err)
	}
	if _, last := phaseIndexes(f.events, "lock:"); last == -1 {
		t.Fatalf("accounts left unlocked after a failed signing: %v", f.events)
	}
}

// A second window waits for the first and gives up with its own deadline.
func TestWithUnlockedAccountsWaitsForTheOpenWindow(t *testing.T) {
	f := &fakeVSPWallet{}
	withFakeVSPWallet(t, f)
	vspSignSem <- struct{}{}
	t.Cleanup(func() { <-vspSignSem })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := withUnlockedAccounts(ctx, []byte("hunter2"), func() error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if len(f.events) != 0 {
		t.Fatalf("a waiting window touched the accounts: %v", f.events)
	}
}
