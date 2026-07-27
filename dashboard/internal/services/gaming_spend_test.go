// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/types"
)

func spendPolicy() types.GamingSettings {
	return types.GamingSettings{
		Enabled:             true,
		Account:             "gaming",
		Mode:                gamingModeApproval,
		PerTableCapAtoms:    100_000_000,
		PerDayCapAtoms:      500_000_000,
		ApprovalTimeoutSecs: 120,
	}
}

func TestASpendWithinPolicyIsCarried(t *testing.T) {
	if err := checkSpendRequest(spendPolicy(), true, "Tsaddr", 10_000_000); err != nil {
		t.Fatalf("a spend inside every cap was refused: %v", err)
	}
}

func TestPolicyRefusesWhatItWasWrittenTo(t *testing.T) {
	for _, tc := range []struct {
		name      string
		settings  func(s types.GamingSettings) types.GamingSettings
		installed bool
		address   string
		amount    int64
		want      error
	}{
		{"switched off", func(s types.GamingSettings) types.GamingSettings {
			s.Enabled = false
			return s
		}, true, "Tsaddr", 10_000_000, ErrGamingSpendRefused},
		{"no account bound", func(s types.GamingSettings) types.GamingSettings {
			s.Account = " "
			return s
		}, true, "Tsaddr", 10_000_000, ErrGamingSpendRefused},
		{"game not added", func(s types.GamingSettings) types.GamingSettings {
			return s
		}, false, "Tsaddr", 10_000_000, ErrGamingGameNotInstalled},
		{"nowhere to pay", func(s types.GamingSettings) types.GamingSettings {
			return s
		}, true, "  ", 10_000_000, ErrGamingSpendRefused},
		{"nothing to pay", func(s types.GamingSettings) types.GamingSettings {
			return s
		}, true, "Tsaddr", 0, ErrGamingSpendRefused},
		{"over the table cap", func(s types.GamingSettings) types.GamingSettings {
			return s
		}, true, "Tsaddr", 100_000_001, ErrGamingSpendRefused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkSpendRequest(tc.settings(spendPolicy()), tc.installed, tc.address, tc.amount)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// Nothing here holds a wallet passphrase - every send route takes one from the
// user and wipes it - so there is no way to pay without asking. Saying so beats
// quietly behaving like approval, which would make the setting a lie.
func TestAutomaticPaymentIsRefusedRatherThanFaked(t *testing.T) {
	s := spendPolicy()
	s.Mode = gamingModeAutopay

	err := checkSpendRequest(s, true, "Tsaddr", 10_000_000)
	if !errors.Is(err, ErrGamingSpendRefused) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("the refusal should say why: %v", err)
	}
}

// A cap that only counted what had already been paid could be walked past by
// asking several times before anyone answered.
func TestPendingRequestsCountTowardTheDailyCap(t *testing.T) {
	now := time.Now().Unix()
	log := spendLog{Spends: []GamingSpend{
		{State: GamingSpendApproved, AmountAtoms: 100, DecidedAt: now - 60},
		{State: GamingSpendPending, AmountAtoms: 50},
		{State: GamingSpendDenied, AmountAtoms: 900, DecidedAt: now - 60},
		{State: GamingSpendExpired, AmountAtoms: 900, DecidedAt: now - 60},
		{State: GamingSpendFailed, AmountAtoms: 900, DecidedAt: now - 60},
	}}
	if got, want := spentInDayLocked(log, now), int64(150); got != want {
		t.Fatalf("counted %d against the day, want %d", got, want)
	}
}

// The day is a rolling one, so yesterday's spending does not hold today's
// hostage.
func TestSpendingOlderThanADayNoLongerCounts(t *testing.T) {
	now := time.Now().Unix()
	day := int64((24 * time.Hour).Seconds())
	log := spendLog{Spends: []GamingSpend{
		{State: GamingSpendApproved, AmountAtoms: 100, DecidedAt: now - day - 1},
		{State: GamingSpendApproved, AmountAtoms: 7, DecidedAt: now - 10},
	}}
	if got, want := spentInDayLocked(log, now), int64(7); got != want {
		t.Fatalf("counted %d against the day, want %d", got, want)
	}
}

func TestTheDailyCapRefusesWhatWouldPassIt(t *testing.T) {
	s := spendPolicy()
	if err := checkSpendAgainstDay(s, 100, s.PerDayCapAtoms-100); err != nil {
		t.Fatalf("a spend that exactly reaches the cap was refused: %v", err)
	}
	if err := checkSpendAgainstDay(s, 101, s.PerDayCapAtoms-100); !errors.Is(err, ErrGamingSpendRefused) {
		t.Fatalf("got %v, want a refusal", err)
	}
	s.PerDayCapAtoms = 0
	if err := checkSpendAgainstDay(s, 1<<40, 1<<40); err != nil {
		t.Fatalf("no cap should mean no refusal: %v", err)
	}
}

// A request nobody answers must stop being answerable, or a game waits forever
// on a person who has gone to bed.
func TestAnUnansweredRequestExpires(t *testing.T) {
	now := time.Now().Unix()
	log := spendLog{Spends: []GamingSpend{
		{ID: "a", State: GamingSpendPending, ExpiresAt: now - 1},
		{ID: "b", State: GamingSpendPending, ExpiresAt: now + 60},
		{ID: "c", State: GamingSpendApproved, ExpiresAt: now - 1},
	}}
	if !expireLocked(&log, now) {
		t.Fatal("an overdue request was not expired")
	}
	if log.Spends[0].State != GamingSpendExpired {
		t.Fatalf("overdue request is %s", log.Spends[0].State)
	}
	if log.Spends[1].State != GamingSpendPending {
		t.Fatalf("a request still in time is %s", log.Spends[1].State)
	}
	if log.Spends[2].State != GamingSpendApproved {
		t.Fatal("a decided request was reopened by expiry")
	}
	if expireLocked(&log, now) {
		t.Fatal("expiring twice reported a change the second time")
	}
}
