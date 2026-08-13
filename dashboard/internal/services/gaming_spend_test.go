// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
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

// spendSeams points the gaming state at a directory a test may write and puts
// every staged call back the way it was.
func spendSeams(t *testing.T) {
	t.Helper()
	origDir, origAccount := GamingStateDir, spendAccount
	origConstruct, origSign, origPublish := spendConstruct, spendSign, spendPublish
	GamingStateDir = t.TempDir()
	t.Cleanup(func() {
		GamingStateDir, spendAccount = origDir, origAccount
		spendConstruct, spendSign, spendPublish = origConstruct, origSign, origPublish
	})
}

// seedPendingSpend writes one request awaiting a person.
func seedPendingSpend(t *testing.T, id string, expiresAt int64) {
	t.Helper()
	if err := writeSpendLog(spendLog{Spends: []GamingSpend{{
		ID: id, Game: "poker", Address: "Tsaddr", AmountAtoms: 1_000_000,
		State: GamingSpendPending, RequestedAt: time.Now().Unix(), ExpiresAt: expiresAt,
	}}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// A mistyped passphrase fails before anything reaches the network, so it must
// cost a retype, not the table: the request stays pending and the same
// approval works when the passphrase is right. Only a publish failure - the
// one stage that may have relayed the transaction despite the error - decides
// the request, exactly as every failure used to.
func TestAMistypedPassphraseLeavesTheRequestApprovable(t *testing.T) {
	spendSeams(t)
	seedPendingSpend(t, "aa11", time.Now().Unix()+300)

	spendAccount = func(context.Context) (uint32, error) { return 1, nil }
	spendConstruct = func(context.Context, uint32, string, int64) ([]byte, error) {
		return []byte("unsigned"), nil
	}
	wrong := true
	spendSign = func(_ context.Context, _ uint32, _ []byte, _ []byte) ([]byte, error) {
		if wrong {
			return nil, errors.New("invalid passphrase for master private key")
		}
		return []byte("signed"), nil
	}
	published := 0
	spendPublish = func(context.Context, []byte) (string, error) {
		published++
		return "txid00", nil
	}

	if _, err := ApproveGamingSpend(context.Background(), "aa11", []byte("wrogn")); err == nil {
		t.Fatal("a failed signing reported success")
	}
	if got := readSpendLog().Spends[0].State; got != GamingSpendPending {
		t.Fatalf("a pre-broadcast failure decided the request: %v", got)
	}
	if published != 0 {
		t.Fatal("published without a signature")
	}

	wrong = false
	out, err := ApproveGamingSpend(context.Background(), "aa11", []byte("right"))
	if err != nil {
		t.Fatalf("the retype was refused: %v", err)
	}
	if out.State != GamingSpendApproved || out.TxID != "txid00" {
		t.Fatalf("approved as %v txid %q", out.State, out.TxID)
	}

	// Once truly decided, deciding again is refused.
	if _, err := ApproveGamingSpend(context.Background(), "aa11", []byte("right")); !errors.Is(err, ErrGamingSpendNotPending) {
		t.Fatalf("an approved request was approvable again: %v", err)
	}
}

func TestAPublishFailureIsTerminal(t *testing.T) {
	spendSeams(t)
	seedPendingSpend(t, "bb22", time.Now().Unix()+300)

	spendAccount = func(context.Context) (uint32, error) { return 1, nil }
	spendConstruct = func(context.Context, uint32, string, int64) ([]byte, error) {
		return []byte("unsigned"), nil
	}
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		return []byte("signed"), nil
	}
	spendPublish = func(context.Context, []byte) (string, error) {
		return "", errors.New("tls handshake torn down mid-send")
	}

	out, err := ApproveGamingSpend(context.Background(), "bb22", []byte("right"))
	if err == nil {
		t.Fatal("a failed publish reported success")
	}
	if out.State != GamingSpendFailed {
		t.Fatalf("a publish failure left the request %v; it may have relayed, so it must be terminal", out.State)
	}
	if _, err := ApproveGamingSpend(context.Background(), "bb22", []byte("right")); !errors.Is(err, ErrGamingSpendNotPending) {
		t.Fatalf("a possibly-broadcast request was approvable again: %v", err)
	}
}

// Leaving failures pending makes retries ordinary, and retries make the race
// practical: the request is still pending in the log while an approval runs
// outside the lock, so without the in-flight guard a second click constructs
// a second transaction and pays the address twice.
func TestOneApprovalRunsAtATime(t *testing.T) {
	spendSeams(t)
	seedPendingSpend(t, "cc33", time.Now().Unix()+300)

	entered := make(chan struct{})
	release := make(chan struct{})
	published := 0

	spendAccount = func(context.Context) (uint32, error) { return 1, nil }
	spendConstruct = func(context.Context, uint32, string, int64) ([]byte, error) {
		return []byte("unsigned"), nil
	}
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		close(entered)
		<-release
		return []byte("signed"), nil
	}
	spendPublish = func(context.Context, []byte) (string, error) {
		published++
		return "txid00", nil
	}

	done := make(chan error, 1)
	go func() {
		_, err := ApproveGamingSpend(context.Background(), "cc33", []byte("right"))
		done <- err
	}()
	<-entered

	if _, err := ApproveGamingSpend(context.Background(), "cc33", []byte("right")); !errors.Is(err, ErrGamingSpendNotPending) {
		t.Fatalf("a second approval ran beside the first: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("the first approval failed: %v", err)
	}
	if published != 1 {
		t.Fatalf("published %d times for one request", published)
	}
}

// The retry window is the approval window. A failed attempt does not extend
// it, and a request nobody answered in time refuses even a correct
// passphrase.
func TestARequestPastItsWindowRefusesApproval(t *testing.T) {
	spendSeams(t)
	seedPendingSpend(t, "dd44", time.Now().Unix()-1)

	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		t.Fatal("an expired request reached the wallet")
		return nil, nil
	}

	if _, err := ApproveGamingSpend(context.Background(), "dd44", []byte("right")); !errors.Is(err, ErrGamingSpendNotPending) {
		t.Fatalf("an expired request was approvable: %v", err)
	}
}
