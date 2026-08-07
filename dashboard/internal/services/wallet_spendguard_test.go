// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
)

// A ticket purchase pauses the mixer for its own duration, so a guard that only
// asks IsMixerRunning reports "idle" for the whole run and lets a send race the
// purchase for the UTXOs it is spending. These pin all three conditions.

func setMixerRunning(running bool) func() {
	mixerMu.Lock()
	prev := mixerCancel
	if running {
		mixerCancel = func() {}
	} else {
		mixerCancel = nil
	}
	mixerMu.Unlock()
	return func() {
		mixerMu.Lock()
		mixerCancel = prev
		mixerMu.Unlock()
	}
}

func setAutobuyerRunning(running bool) func() {
	autobuyerMu.Lock()
	prev := autobuyerCancel
	if running {
		autobuyerCancel = func() {}
	} else {
		autobuyerCancel = nil
	}
	autobuyerMu.Unlock()
	return func() {
		autobuyerMu.Lock()
		autobuyerCancel = prev
		autobuyerMu.Unlock()
	}
}

func TestSpendGuard(t *testing.T) {
	tests := []struct {
		name      string
		mixer     bool
		autobuyer bool
		purchase  bool
		want      error
	}{
		{"idle", false, false, false, nil},
		{"mixer running", true, false, false, ErrSpendWhileMixing},
		{"autobuyer running", false, true, false, ErrSpendWhileMixing},

		// The case this fix exists for: a purchase has paused the mixer, so the
		// first two conditions both read false for the whole ~10 minute run.
		{"ticket purchase in progress", false, false, true, ErrSpendWhilePurchasing},

		// When both apply, report the one the user can actually act on.
		{"mixer wins over purchase", true, false, true, ErrSpendWhileMixing},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer setMixerRunning(tc.mixer)()
			defer setAutobuyerRunning(tc.autobuyer)()
			if tc.purchase {
				if !tryBeginTicketPurchase() {
					t.Fatal("a purchase was already marked active")
				}
				defer endTicketPurchase()
			}

			got := spendGuard()
			if !errors.Is(got, tc.want) {
				t.Errorf("spendGuard() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The guard has to run before the RPC-client check, or a busy wallet reports
// "client not initialized" instead of the real reason.
func TestSpendPathsRejectDuringPurchase(t *testing.T) {
	if !tryBeginTicketPurchase() {
		t.Fatal("a purchase was already marked active")
	}
	defer endTicketPurchase()

	if _, err := SignAndPublishTransaction(context.Background(), 0, nil, nil); !errors.Is(err, ErrSpendWhilePurchasing) {
		t.Errorf("SignAndPublishTransaction = %v, want ErrSpendWhilePurchasing", err)
	}
	if _, err := SignMsigTransaction(context.Background(), "", nil, 0, nil); !errors.Is(err, ErrSpendWhilePurchasing) {
		t.Errorf("SignMsigTransaction = %v, want ErrSpendWhilePurchasing", err)
	}
}
