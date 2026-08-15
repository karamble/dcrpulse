// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dcrpulse/internal/types"
)

// spendPolicy is one registered, funded game.
func spendPolicy() types.GamingSettings {
	return types.GamingSettings{
		Enabled:         true,
		RegisteredGames: []string{"poker"},
		Policies: map[string]types.GamePolicy{"poker": {
			Account:             "gaming",
			PerTableCapAtoms:    100_000_000,
			PerDayCapAtoms:      500_000_000,
			ApprovalTimeoutSecs: 120,
		}},
	}
}

// withPokerPolicy returns settings whose poker policy has been edited.
func withPokerPolicy(s types.GamingSettings, edit func(p *types.GamePolicy)) types.GamingSettings {
	p := s.Policies["poker"]
	edit(&p)
	s.Policies = map[string]types.GamePolicy{"poker": p}
	return s
}

func TestASpendWithinPolicyIsCarried(t *testing.T) {
	if _, err := checkSpendRequest(spendPolicy(), "poker", "Tsaddr", 10_000_000); err != nil {
		t.Fatalf("a spend inside every cap was refused: %v", err)
	}
}

func TestPolicyRefusesWhatItWasWrittenTo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings func(s types.GamingSettings) types.GamingSettings
		game     string
		address  string
		amount   int64
		want     error
	}{
		{"switched off", func(s types.GamingSettings) types.GamingSettings {
			s.Enabled = false
			return s
		}, "poker", "Tsaddr", 10_000_000, ErrGamingSpendRefused},
		{"no account bound", func(s types.GamingSettings) types.GamingSettings {
			return withPokerPolicy(s, func(p *types.GamePolicy) { p.Account = " " })
		}, "poker", "Tsaddr", 10_000_000, ErrGamingSpendRefused},
		{"game not registered", func(s types.GamingSettings) types.GamingSettings {
			return s
		}, "chess", "Tsaddr", 10_000_000, ErrGamingGameNotRegistered},
		{"nowhere to pay", func(s types.GamingSettings) types.GamingSettings {
			return s
		}, "poker", "  ", 10_000_000, ErrGamingSpendRefused},
		{"nothing to pay", func(s types.GamingSettings) types.GamingSettings {
			return s
		}, "poker", "Tsaddr", 0, ErrGamingSpendRefused},
		{"over the table cap", func(s types.GamingSettings) types.GamingSettings {
			return s
		}, "poker", "Tsaddr", 100_000_001, ErrGamingSpendRefused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := checkSpendRequest(tc.settings(spendPolicy()), tc.game, tc.address, tc.amount)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// Asking for money gets a person asked, and nothing else.
//
// This process holds no wallet passphrase, so a request that came back anything
// but pending - or that reached the wallet at all - would mean a path existed
// that pays a game on its own. There used to be a setting offering exactly
// that; it could be chosen, it was stored, and then every spend was refused
// with a message only the game saw. This asserts the property the setting
// pretended to configure, which is the one that has to hold however the policy
// is written.
func TestNothingIsPaidWithoutAPerson(t *testing.T) {
	spendSeams(t)
	spendAccount = func(context.Context, string) (uint32, error) {
		t.Fatal("asking to spend resolved an account on its own")
		return 0, nil
	}
	spendConstruct = func(context.Context, uint32, string, int64) ([]byte, error) {
		t.Fatal("asking to spend built a transaction on its own")
		return nil, nil
	}
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		t.Fatal("asking to spend reached the wallet on its own")
		return nil, nil
	}
	spendPublish = func(context.Context, []byte) (string, error) {
		t.Fatal("asking to spend put a transaction on the network on its own")
		return "", nil
	}

	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}

	spend, err := RequestGamingSpend("poker", "Tsaddr", 10_000_000, "a seat")
	if err != nil {
		t.Fatalf("a spend inside every cap was refused: %v", err)
	}
	if spend.State != GamingSpendPending {
		t.Fatalf("a request came back %s without anybody being asked", spend.State)
	}
	if spend.TxID != "" {
		t.Fatalf("a request nobody approved has a transaction: %s", spend.TxID)
	}
}

// A cap that only counted what had already been paid could be walked past by
// asking several times before anyone answered.
func TestPendingRequestsCountTowardTheDailyCap(t *testing.T) {
	now := time.Now().Unix()
	log := spendLog{Spends: []GamingSpend{
		{Game: "poker", State: GamingSpendApproved, AmountAtoms: 100, DecidedAt: now - 60},
		{Game: "poker", State: GamingSpendPending, AmountAtoms: 50},
		{Game: "poker", State: GamingSpendDenied, AmountAtoms: 900, DecidedAt: now - 60},
		{Game: "poker", State: GamingSpendExpired, AmountAtoms: 900, DecidedAt: now - 60},
		{Game: "poker", State: GamingSpendFailed, AmountAtoms: 900, DecidedAt: now - 60},
	}}
	if got, want := spentInDayLocked(log, "poker", now, ""), int64(150); got != want {
		t.Fatalf("counted %d against the day, want %d", got, want)
	}
}

// The day is a rolling one, so yesterday's spending does not hold today's
// hostage.
func TestSpendingOlderThanADayNoLongerCounts(t *testing.T) {
	now := time.Now().Unix()
	day := int64((24 * time.Hour).Seconds())
	log := spendLog{Spends: []GamingSpend{
		{Game: "poker", State: GamingSpendApproved, AmountAtoms: 100, DecidedAt: now - day - 1},
		{Game: "poker", State: GamingSpendApproved, AmountAtoms: 7, DecidedAt: now - 10},
	}}
	if got, want := spentInDayLocked(log, "poker", now, ""), int64(7); got != want {
		t.Fatalf("counted %d against the day, want %d", got, want)
	}
}

// The day total is one game's budget, not the section's.
//
// Summed across every game, a busy table spends the allowance of a game that
// asked for nothing, and that game is then refused for money it never moved -
// which is a limit falling on whoever happened to play second.
func TestOneGamesSpendingDoesNotConsumeAnothersAllowance(t *testing.T) {
	now := time.Now().Unix()
	log := spendLog{Spends: []GamingSpend{
		{Game: "poker", State: GamingSpendApproved, AmountAtoms: 100, DecidedAt: now - 60},
		{Game: "poker", State: GamingSpendPending, AmountAtoms: 50},
		{Game: "chess", State: GamingSpendApproved, AmountAtoms: 900, DecidedAt: now - 60},
	}}
	if got, want := spentInDayLocked(log, "poker", now, ""), int64(150); got != want {
		t.Errorf("poker was charged %d, want %d", got, want)
	}
	if got, want := spentInDayLocked(log, "chess", now, ""), int64(900); got != want {
		t.Errorf("chess was charged %d, want %d", got, want)
	}
	// A game nobody has spent for owes nothing, however busy the others are.
	if got := spentInDayLocked(log, "backgammon", now, ""); got != 0 {
		t.Errorf("a game that has spent nothing was charged %d", got)
	}
}

func TestTheDailyCapRefusesWhatWouldPassIt(t *testing.T) {
	p := spendPolicy().Policies["poker"]
	if err := checkSpendAgainstDay(p, 100, p.PerDayCapAtoms-100); err != nil {
		t.Fatalf("a spend that exactly reaches the cap was refused: %v", err)
	}
	if err := checkSpendAgainstDay(p, 101, p.PerDayCapAtoms-100); !errors.Is(err, ErrGamingSpendRefused) {
		t.Fatalf("got %v, want a refusal", err)
	}
	p.PerDayCapAtoms = 0
	if err := checkSpendAgainstDay(p, 1<<40, 1<<40); err != nil {
		t.Fatalf("no cap should mean no refusal: %v", err)
	}
}

// More atoms than exist is not a cap decision: no operator wrote it, and no
// game asking for it is asking to play. It is refused before the table cap
// is consulted, so an absurd amount never comes back described as merely
// over a limit a game could wait out.
func TestARequestForMoreAtomsThanExistIsRefused(t *testing.T) {
	_, err := checkSpendRequest(spendPolicy(), "poker", "Tsaddr", maxSpendAtoms+1)
	if !errors.Is(err, ErrGamingSpendRefused) {
		t.Fatalf("a request for more than the supply was carried: %v", err)
	}
	if errors.Is(err, ErrGamingSpendOverCap) {
		t.Fatalf("more than the supply came back as merely over a cap: %v", err)
	}

	uncapped := withPokerPolicy(spendPolicy(), func(p *types.GamePolicy) {
		p.PerTableCapAtoms, p.PerDayCapAtoms = 0, 0
	})
	if _, err := checkSpendRequest(uncapped, "poker", "Tsaddr", maxSpendAtoms); err != nil {
		t.Fatalf("the whole supply under no caps was refused: %v", err)
	}
}

// The day check is a bound on the request, not a sum: a sum's operands are
// a caller's to choose, and one absurd request wrapping the arithmetic
// negative would read as under any cap - the daily limit switched off for
// everything asked after it.
func TestTheDailyCapCannotBeWrappedPast(t *testing.T) {
	p := spendPolicy().Policies["poker"]
	for _, tc := range []struct {
		what   string
		amount int64
		used   int64
		refuse bool
	}{
		{"an amount that would wrap the sum", math.MaxInt64, 100, true},
		{"the whole cap with some already used", p.PerDayCapAtoms, 1, true},
		{"exactly the headroom left", p.PerDayCapAtoms - 100, 100, false},
		{"one atom when the day is spent", 1, p.PerDayCapAtoms, true},
		{"one atom against an untouched day", 1, 0, false},
		{"one atom on a saturated total", 1, math.MaxInt64, true},
	} {
		err := checkSpendAgainstDay(p, tc.amount, tc.used)
		if tc.refuse && !errors.Is(err, ErrGamingSpendOverCap) {
			t.Errorf("%s was carried: %v", tc.what, err)
		}
		if !tc.refuse && err != nil {
			t.Errorf("%s was refused: %v", tc.what, err)
		}
	}
}

// A negative day total is a log this code never wrote. Granting it as
// headroom would turn corruption into allowance, so it refuses instead.
func TestANegativeDayTotalRefusesRatherThanGrantsHeadroom(t *testing.T) {
	p := spendPolicy().Policies["poker"]
	if err := checkSpendAgainstDay(p, 1, -1); !errors.Is(err, ErrGamingSpendOverCap) {
		t.Fatalf("a poisoned total granted headroom: %v", err)
	}
}

// Entries too large to sum must read as a day already full, never as a
// number that wrapped: MaxInt64 refuses everything after it, where a
// wrapped negative would have allowed anything.
func TestAHugeDayTotalSaturatesInsteadOfWrapping(t *testing.T) {
	now := time.Now().Unix()
	for _, tc := range []struct {
		what string
		log  spendLog
		want int64
	}{
		{"two entries no int64 can hold", spendLog{Spends: []GamingSpend{
			{Game: "poker", State: GamingSpendApproved, AmountAtoms: math.MaxInt64 - 5, DecidedAt: now - 60},
			{Game: "poker", State: GamingSpendPending, AmountAtoms: 100},
		}}, math.MaxInt64},
		{"ordinary entries still sum exactly", spendLog{Spends: []GamingSpend{
			{Game: "poker", State: GamingSpendApproved, AmountAtoms: 100, DecidedAt: now - 60},
			{Game: "poker", State: GamingSpendPending, AmountAtoms: 50},
		}}, 150},
		{"an amount nobody could have requested", spendLog{Spends: []GamingSpend{
			{Game: "poker", State: GamingSpendApproved, AmountAtoms: -5, DecidedAt: now - 60},
		}}, math.MaxInt64},
	} {
		if got := spentInDayLocked(tc.log, "poker", now, ""); got != tc.want {
			t.Errorf("%s: counted %d against the day, want %d", tc.what, got, tc.want)
		}
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

// mustReadSpendLog reads the log for an assertion, failing the test rather
// than returning an error nothing here expects.
func mustReadSpendLog(t *testing.T) spendLog {
	t.Helper()
	log, err := readSpendLog()
	if err != nil {
		t.Fatalf("read spend log: %v", err)
	}
	return log
}

// seedPendingSpend writes one request awaiting a person.
func seedPendingSpend(t *testing.T, id string, expiresAt int64) {
	t.Helper()
	if err := writeSpendLog(spendLog{Spends: []GamingSpend{{
		ID: id, Game: "poker", Address: "Tsaddr", AmountAtoms: 1_000_000,
		State: GamingSpendPending, RequestedAt: time.Now().Unix(), ExpiresAt: expiresAt,
	}}}, time.Now().Unix()); err != nil {
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
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	seedPendingSpend(t, "aa11", time.Now().Unix()+300)

	spendAccount = func(context.Context, string) (uint32, error) { return 1, nil }
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
	if got := mustReadSpendLog(t).Spends[0].State; got != GamingSpendPending {
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
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	seedPendingSpend(t, "bb22", time.Now().Unix()+300)

	spendAccount = func(context.Context, string) (uint32, error) { return 1, nil }
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
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	seedPendingSpend(t, "cc33", time.Now().Unix()+300)

	entered := make(chan struct{})
	release := make(chan struct{})
	published := 0

	spendAccount = func(context.Context, string) (uint32, error) { return 1, nil }
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

// The audit file is also where the day total is counted from, so a trim
// that dropped what the cap still counts would let a flood of worthless
// requests scroll the day's real spending out of the file and reset the
// cap in the middle of the day.
func TestTheDaysSpendingCannotBeScrolledOffTheLog(t *testing.T) {
	spendSeams(t)
	now := time.Now().Unix()
	// The window is pinned by its own literal, not the constant under test,
	// so a window that quietly shrank cannot drag this fixture with it.
	spends := []GamingSpend{
		{ID: "paid", Game: "poker", State: GamingSpendApproved, AmountAtoms: 7, DecidedAt: now - 86400 + 2},
		{ID: "open", Game: "poker", State: GamingSpendPending, AmountAtoms: 50, ExpiresAt: now + 300},
	}
	for range maxSpendLog {
		spends = append(spends, GamingSpend{
			Game: "poker", State: GamingSpendDenied, AmountAtoms: 1, DecidedAt: now - 30,
		})
	}
	if err := writeSpendLog(spendLog{Spends: spends}, now); err != nil {
		t.Fatalf("write: %v", err)
	}
	log := mustReadSpendLog(t)
	if len(log.Spends) != maxSpendLog {
		t.Fatalf("the log holds %d entries, want the bound of %d", len(log.Spends), maxSpendLog)
	}
	if log.Spends[0].ID != "paid" || log.Spends[1].ID != "open" {
		t.Fatalf("the entries the cap counts were trimmed or reordered: the first two are %q and %q",
			log.Spends[0].ID, log.Spends[1].ID)
	}
	if got, want := spentInDayLocked(log, "poker", now, ""), int64(57); got != want {
		t.Fatalf("after the trim the day counts %d, want %d - a flood reset the cap", got, want)
	}
}

// When everything in an oversized log still counts, the bound yields:
// dropping live accounting to honour a file-size number would be the same
// bug the trim exists to prevent. And exactly at the bound, nothing is
// dropped at all, droppable or not.
func TestAFullLogOfLiveEntriesIsKeptWholeRatherThanTrimmed(t *testing.T) {
	spendSeams(t)
	now := time.Now().Unix()
	var spends []GamingSpend
	for range maxSpendLog + 1 {
		spends = append(spends, GamingSpend{
			Game: "poker", State: GamingSpendPending, AmountAtoms: 1, ExpiresAt: now + 300,
		})
	}
	if err := writeSpendLog(spendLog{Spends: spends}, now); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := len(mustReadSpendLog(t).Spends); got != maxSpendLog+1 {
		t.Fatalf("a log of %d live entries was trimmed to %d", maxSpendLog+1, got)
	}

	atBound := append(spends[:maxSpendLog-1:maxSpendLog-1], GamingSpend{
		Game: "poker", State: GamingSpendDenied, AmountAtoms: 1, DecidedAt: now - 60,
	})
	if err := writeSpendLog(spendLog{Spends: atBound}, now); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := len(mustReadSpendLog(t).Spends); got != maxSpendLog {
		t.Fatalf("a log exactly at the bound was cut to %d", got)
	}
}

// A person can only be waiting on so much at once. The ninth unanswered
// request is refused - as over a cap, the refusal that tells a game to
// wait - until any one of the eight is answered. Eight is pinned by its
// own literal here: poker's worst honest case is documented against it,
// so the ceiling does not get to drift without this sentence changing.
func TestANinthUnansweredRequestIsRefusedUntilOneIsAnswered(t *testing.T) {
	spendSeams(t)
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	var last GamingSpend
	for i := range 8 {
		out, err := RequestGamingSpend("poker", "Tsaddr", 1_000_000, "a seat")
		if err != nil {
			t.Fatalf("request %d of 8 was refused: %v", i+1, err)
		}
		last = out
	}
	if _, err := RequestGamingSpend("poker", "Tsaddr", 1_000_000, "one too many"); !errors.Is(err, ErrGamingSpendOverCap) {
		t.Fatalf("an unanswered backlog past the ceiling was carried: %v", err)
	}
	if _, err := DenyGamingSpend(last.ID); err != nil {
		t.Fatalf("deny: %v", err)
	}
	if _, err := RequestGamingSpend("poker", "Tsaddr", 1_000_000, "after an answer"); err != nil {
		t.Fatalf("answering one request did not make room for another: %v", err)
	}
}

// The ceiling is per game for the same reason the day total is: a bound
// that fell on whoever asked second would not be a bound on anybody.
func TestOneGamesBacklogDoesNotBlockAnother(t *testing.T) {
	spendSeams(t)
	s := spendPolicy()
	s.RegisteredGames = []string{"poker", "chess"}
	s.Policies["chess"] = s.Policies["poker"]
	if _, err := WriteGamingSettings(s, true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	for i := range maxPendingSpendsPerGame {
		if _, err := RequestGamingSpend("poker", "Tsaddr", 1_000_000, "a seat"); err != nil {
			t.Fatalf("request %d was refused: %v", i+1, err)
		}
	}
	if _, err := RequestGamingSpend("chess", "Tsaddr", 1_000_000, "a first ask"); err != nil {
		t.Fatalf("poker's backlog blocked chess: %v", err)
	}
}

// approvalStubs wires the happy wallet path: account 1, a fixed unsigned and
// signed blob, and a publish that counts itself and returns txid00.
func approvalStubs(t *testing.T, published *int) {
	t.Helper()
	spendAccount = func(context.Context, string) (uint32, error) { return 1, nil }
	spendConstruct = func(context.Context, uint32, string, int64) ([]byte, error) {
		return []byte("unsigned"), nil
	}
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		return []byte("signed"), nil
	}
	spendPublish = func(context.Context, []byte) (string, error) {
		*published++
		return "txid00", nil
	}
}

// The one fact that must never be lost is that a signed transaction is about
// to reach the network. If the broadcast came first, a crash between it and
// the record would leave a paid request pending - approvable a second time,
// by a person who has no way to know.
func TestTheLogSaysPublishingBeforeAnythingIsBroadcast(t *testing.T) {
	spendSeams(t)
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	seedPendingSpend(t, "aa11", time.Now().Unix()+300)

	published := 0
	approvalStubs(t, &published)
	spendPublish = func(context.Context, []byte) (string, error) {
		if got := mustReadSpendLog(t).Spends[0].State; got != "publishing" {
			t.Fatalf("at broadcast time the log says %q, want publishing", got)
		}
		published++
		return "txid00", nil
	}

	out, err := ApproveGamingSpend(context.Background(), "aa11", []byte("right"))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if out.State != GamingSpendApproved || out.TxID != "txid00" || published != 1 {
		t.Fatalf("approved as %v txid %q published %d", out.State, out.TxID, published)
	}
}

// The window is checked one last time on the way to the network. An approval
// that outlives it must stop before the broadcast, not record an expiry over
// money that moved.
func TestAnApprovalThatOutlivedItsWindowNeverBroadcasts(t *testing.T) {
	spendSeams(t)
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	expiresAt := time.Now().Unix() + 1
	seedPendingSpend(t, "bb22", expiresAt)

	published := 0
	approvalStubs(t, &published)
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		for time.Now().Unix() <= expiresAt {
			time.Sleep(100 * time.Millisecond)
		}
		return []byte("signed"), nil
	}
	spendPublish = func(context.Context, []byte) (string, error) {
		t.Fatal("an approval past its window reached the network")
		return "", nil
	}

	if _, err := ApproveGamingSpend(context.Background(), "bb22", []byte("right")); !errors.Is(err, ErrGamingSpendNotPending) {
		t.Fatalf("an approval past its window came back %v, want not-pending", err)
	}
	if got := mustReadSpendLog(t).Spends[0].State; got != GamingSpendExpired {
		t.Fatalf("the entry is %q, want expired", got)
	}
}

// An outcome is final the moment it is written. Only a payment on its way to
// the network may take one; every other state refuses, so a decided request
// can never be rewritten however the paths interleave.
func TestADecidedRequestCannotBeRewritten(t *testing.T) {
	spendSeams(t)
	now := time.Now().Unix()
	for _, state := range []GamingSpendState{
		GamingSpendPending, GamingSpendApproved, GamingSpendDenied,
		GamingSpendExpired, GamingSpendFailed,
	} {
		if err := writeSpendLog(spendLog{Spends: []GamingSpend{{
			ID: "cc33", Game: "poker", Address: "Tsaddr", AmountAtoms: 1_000_000,
			State: state, RequestedAt: now, ExpiresAt: now + 300, DecidedAt: now,
		}}}, now); err != nil {
			t.Fatalf("seed %s: %v", state, err)
		}
		if _, err := recordSpendOutcome(GamingSpend{ID: "cc33"}, GamingSpendApproved, "txid99", ""); err == nil {
			t.Fatalf("an outcome was recorded over %q", state)
		}
		if got := mustReadSpendLog(t).Spends[0].State; got != state {
			t.Fatalf("recording over %q changed it to %q", state, got)
		}
	}
}

// The deny button is a kill switch, and a kill switch that reports success
// while doing nothing is the worst way for one to behave. While an approval
// runs, deny refuses; it never says "denied" over a payment going out.
func TestDenyIsRefusedWhileAnApprovalRuns(t *testing.T) {
	spendSeams(t)
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	seedPendingSpend(t, "dd44", time.Now().Unix()+300)

	entered := make(chan struct{})
	release := make(chan struct{})
	published := 0
	approvalStubs(t, &published)
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		close(entered)
		<-release
		return []byte("signed"), nil
	}

	done := make(chan error, 1)
	go func() {
		_, err := ApproveGamingSpend(context.Background(), "dd44", []byte("right"))
		done <- err
	}()
	<-entered

	if _, err := DenyGamingSpend("dd44"); !errors.Is(err, ErrGamingSpendNotPending) {
		t.Fatalf("deny during an approval came back %v, want a refusal", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("the approval the deny lost to then failed: %v", err)
	}
	if published != 1 {
		t.Fatalf("published %d times for one request", published)
	}
	if got := mustReadSpendLog(t).Spends[0].State; got != GamingSpendApproved {
		t.Fatalf("the request ended %q, want approved", got)
	}
}

// A payment already travelling to the network is not deniable either; there
// is no answer left to give.
func TestDenyIsRefusedWhileAPaymentIsBroadcasting(t *testing.T) {
	spendSeams(t)
	now := time.Now().Unix()
	if err := writeSpendLog(spendLog{Spends: []GamingSpend{{
		ID: "ee55", Game: "poker", Address: "Tsaddr", AmountAtoms: 1_000_000,
		State: GamingSpendPublishing, RequestedAt: now, ExpiresAt: now + 300,
	}}}, now); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := DenyGamingSpend("ee55"); !errors.Is(err, ErrGamingSpendNotPending) {
		t.Fatalf("deny over a broadcast came back %v, want a refusal", err)
	}
	if got := mustReadSpendLog(t).Spends[0].State; got != GamingSpendPublishing {
		t.Fatalf("deny changed a broadcast to %q", got)
	}
}

// A crash between the broadcast and its record leaves an entry publishing
// with nobody working on it. Long after its window it fails, with the one
// sentence that says where to look - never silently, and never while an
// approval in this process is still running it.
func TestAPaymentInterruptedByARestartFailsWithTheWarning(t *testing.T) {
	spendSeams(t)
	now := time.Now().Unix()
	if err := writeSpendLog(spendLog{Spends: []GamingSpend{{
		ID: "ff66", Game: "poker", Address: "Tsaddr", AmountAtoms: 1_000_000,
		State: GamingSpendPublishing, RequestedAt: now - 600, ExpiresAt: now - 301,
	}}}, now); err != nil {
		t.Fatalf("seed: %v", err)
	}
	GamingSpends()
	got := mustReadSpendLog(t).Spends[0]
	if got.State != GamingSpendFailed {
		t.Fatalf("an interrupted broadcast is %q, want failed", got.State)
	}
	want := "interrupted while broadcasting; check the wallet for the transaction before paying it again"
	if got.Error != want {
		t.Fatalf("the warning reads %q, want %q", got.Error, want)
	}
}

// However late a broadcast runs, while this process is still running it the
// sweep must not declare it dead under its feet - the outcome that is coming
// would then be refused by the entry's own log.
func TestALivePaymentIsNeverSwept(t *testing.T) {
	spendSeams(t)
	now := time.Now().Unix()
	if err := writeSpendLog(spendLog{Spends: []GamingSpend{{
		ID: "gg77", Game: "poker", Address: "Tsaddr", AmountAtoms: 1_000_000,
		State: GamingSpendPublishing, RequestedAt: now - 9999, ExpiresAt: now - 9000,
	}}}, now); err != nil {
		t.Fatalf("seed: %v", err)
	}
	spendMu.Lock()
	spendApproving["gg77"] = true
	spendMu.Unlock()
	t.Cleanup(func() {
		spendMu.Lock()
		delete(spendApproving, "gg77")
		spendMu.Unlock()
	})

	GamingSpends()
	if got := mustReadSpendLog(t).Spends[0].State; got != GamingSpendPublishing {
		t.Fatalf("a live broadcast was swept to %q", got)
	}
}

// Money on its way to the network is exposure like money waiting on a person:
// it counts against the day, it holds one of the eight outstanding slots, and
// the trim may never drop it.
func TestBroadcastingMoneyStillCountsEverywhere(t *testing.T) {
	spendSeams(t)
	now := time.Now().Unix()

	day := spendLog{Spends: []GamingSpend{
		{Game: "poker", State: GamingSpendPublishing, AmountAtoms: 70, ExpiresAt: now + 300},
		{Game: "poker", State: GamingSpendPending, AmountAtoms: 30, ExpiresAt: now + 300},
	}}
	if got, want := spentInDayLocked(day, "poker", now, ""), int64(100); got != want {
		t.Fatalf("counted %d against the day, want %d", got, want)
	}

	var spends []GamingSpend
	spends = append(spends, GamingSpend{
		ID: "hh88", Game: "poker", State: GamingSpendPublishing, AmountAtoms: 1,
		RequestedAt: now, ExpiresAt: now + 300,
	})
	for i := 0; i < 7; i++ {
		spends = append(spends, GamingSpend{
			Game: "poker", State: GamingSpendPending, AmountAtoms: 1,
			RequestedAt: now, ExpiresAt: now + 300,
		})
	}
	if err := writeSpendLog(spendLog{Spends: spends}, now); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	if _, err := RequestGamingSpend("poker", "Tsaddr", 1_000_000, "a seat"); !errors.Is(err, ErrGamingSpendOverCap) {
		t.Fatalf("a broadcast did not hold its slot: %v", err)
	}

	flood := []GamingSpend{{
		ID: "hh88", Game: "poker", State: GamingSpendPublishing, AmountAtoms: 1,
		RequestedAt: now, ExpiresAt: now + 300,
	}}
	for i := 0; i < 500; i++ {
		flood = append(flood, GamingSpend{
			Game: "poker", State: GamingSpendDenied, AmountAtoms: 1, DecidedAt: now - 30,
		})
	}
	if err := writeSpendLog(spendLog{Spends: flood}, now); err != nil {
		t.Fatalf("write: %v", err)
	}
	kept := mustReadSpendLog(t)
	if kept.Spends[0].ID != "hh88" {
		t.Fatalf("the trim dropped a broadcast; the log starts with %q", kept.Spends[0].ID)
	}
}

// The deployed game reads any state it does not know as a terminal refusal
// and drops its own double-payment guard, so a payment being broadcast is
// told to it as still pending - which is also the truthful answer.
func TestPublishingNeverReachesTheWire(t *testing.T) {
	for _, tc := range []struct {
		state GamingSpendState
		want  string
	}{
		{GamingSpendPending, "pending"},
		{GamingSpendPublishing, "pending"},
		{GamingSpendApproved, "approved"},
		{GamingSpendDenied, "denied"},
		{GamingSpendExpired, "expired"},
		{GamingSpendFailed, "failed"},
	} {
		p := spendProto(GamingSpend{ID: "x", State: tc.state})
		if p.State != tc.want {
			t.Errorf("%s rides the wire as %q, want %q", tc.state, p.State, tc.want)
		}
	}
}

// A request is checked when it is made and again when it is paid, because
// the settings can change in between. Each row edits the stored settings
// underneath a pending request - the way a hand edit or a carried-over file
// would - and the approval must refuse by the policy as it stands now, with
// the request left pending and the wallet never reached.
func TestApprovalReChecksThePolicyItWasRequestedUnder(t *testing.T) {
	for _, tc := range []struct {
		what string
		edit func(s types.GamingSettings) types.GamingSettings
	}{
		{"the bridge was switched off", func(s types.GamingSettings) types.GamingSettings {
			s.Enabled = false
			return s
		}},
		{"the game was unregistered", func(s types.GamingSettings) types.GamingSettings {
			s.Policies = map[string]types.GamePolicy{}
			return s
		}},
		{"the account was unbound", func(s types.GamingSettings) types.GamingSettings {
			return withPokerPolicy(s, func(p *types.GamePolicy) { p.Account = " " })
		}},
		{"the table cap was lowered under it", func(s types.GamingSettings) types.GamingSettings {
			return withPokerPolicy(s, func(p *types.GamePolicy) { p.PerTableCapAtoms = 100_000 })
		}},
	} {
		t.Run(tc.what, func(t *testing.T) {
			spendSeams(t)
			if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
				t.Fatalf("store a policy: %v", err)
			}
			seedPendingSpend(t, "rr11", time.Now().Unix()+300)
			spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
				t.Fatal("a refused approval reached the wallet")
				return nil, nil
			}
			if err := writeGamingSettingsLocked(tc.edit(spendPolicy())); err != nil {
				t.Fatalf("edit settings: %v", err)
			}
			_, err := ApproveGamingSpend(context.Background(), "rr11", []byte("right"))
			if !errors.Is(err, ErrGamingSpendRefused) && !errors.Is(err, ErrGamingGameNotRegistered) {
				t.Fatalf("%s and the approval came back %v, want a policy refusal", tc.what, err)
			}
			if got := mustReadSpendLog(t).Spends[0].State; got != GamingSpendPending {
				t.Fatalf("the refusal decided the request: %q", got)
			}
		})
	}

	t.Run("a day consumed since the request", func(t *testing.T) {
		spendSeams(t)
		if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
			t.Fatalf("store a policy: %v", err)
		}
		now := time.Now().Unix()
		if err := writeSpendLog(spendLog{Spends: []GamingSpend{
			{ID: "rr22", Game: "poker", Address: "Tsaddr", AmountAtoms: 1_000_000,
				State: GamingSpendPending, RequestedAt: now, ExpiresAt: now + 300},
			{Game: "poker", State: GamingSpendApproved, AmountAtoms: 499_500_000, DecidedAt: now - 60},
		}}, now); err != nil {
			t.Fatalf("seed: %v", err)
		}
		spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
			t.Fatal("a refused approval reached the wallet")
			return nil, nil
		}
		if _, err := ApproveGamingSpend(context.Background(), "rr22", []byte("right")); !errors.Is(err, ErrGamingSpendOverCap) {
			t.Fatalf("a day spent since the request approved anyway: %v", err)
		}
	})

	// The request's own amount is excluded from the day it is judged
	// against, or a request for the whole day's budget could never be
	// approved: it would be counted as already having spent itself.
	t.Run("a request for the whole day approves", func(t *testing.T) {
		spendSeams(t)
		whole := withPokerPolicy(spendPolicy(), func(p *types.GamePolicy) {
			p.PerTableCapAtoms = 500_000_000
		})
		if _, err := WriteGamingSettings(whole, true); err != nil {
			t.Fatalf("store a policy: %v", err)
		}
		now := time.Now().Unix()
		if err := writeSpendLog(spendLog{Spends: []GamingSpend{
			{ID: "rr33", Game: "poker", Address: "Tsaddr", AmountAtoms: 500_000_000,
				State: GamingSpendPending, RequestedAt: now, ExpiresAt: now + 300},
		}}, now); err != nil {
			t.Fatalf("seed: %v", err)
		}
		published := 0
		approvalStubs(t, &published)
		if _, err := ApproveGamingSpend(context.Background(), "rr33", []byte("right")); err != nil {
			t.Fatalf("a request counted against its own day: %v", err)
		}
	})
}

// A settings change answers the requests it invalidated instead of leaving
// them pending, counting against the day and holding ceiling slots - and a
// removed game's credential stops resolving now, not at the next restart.
func TestASettingsChangeRetiresTheRequestsItOrphaned(t *testing.T) {
	reason := "invalidated by a settings change"

	t.Run("disabling retires every pending request", func(t *testing.T) {
		spendSeams(t)
		if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
			t.Fatalf("store a policy: %v", err)
		}
		now := time.Now().Unix()
		if err := writeSpendLog(spendLog{Spends: []GamingSpend{
			{ID: "s1", Game: "poker", State: GamingSpendPending, AmountAtoms: 1, ExpiresAt: now + 300},
			{ID: "s2", Game: "poker", State: GamingSpendPending, AmountAtoms: 1, ExpiresAt: now + 300},
			{ID: "s3", Game: "poker", State: GamingSpendPublishing, AmountAtoms: 1, ExpiresAt: now + 300},
		}}, now); err != nil {
			t.Fatalf("seed: %v", err)
		}
		off := spendPolicy()
		off.Enabled = false
		if _, err := WriteGamingSettings(off, true); err != nil {
			t.Fatalf("disable: %v", err)
		}
		log := mustReadSpendLog(t)
		for _, i := range []int{0, 1} {
			if log.Spends[i].State != GamingSpendExpired || log.Spends[i].Error != reason {
				t.Fatalf("pending %q ended %q %q, want expired with the reason",
					log.Spends[i].ID, log.Spends[i].State, log.Spends[i].Error)
			}
		}
		if log.Spends[2].State != GamingSpendPublishing {
			t.Fatalf("disabling touched money in flight: %q", log.Spends[2].State)
		}
	})

	t.Run("dropping a game retires only its requests", func(t *testing.T) {
		spendSeams(t)
		both := spendPolicy()
		both.RegisteredGames = []string{"poker", "chess"}
		both.Policies["chess"] = both.Policies["poker"]
		if _, err := WriteGamingSettings(both, true); err != nil {
			t.Fatalf("store a policy: %v", err)
		}
		now := time.Now().Unix()
		if err := writeSpendLog(spendLog{Spends: []GamingSpend{
			{ID: "p1", Game: "poker", State: GamingSpendPending, AmountAtoms: 1, ExpiresAt: now + 300},
			{ID: "c1", Game: "chess", State: GamingSpendPending, AmountAtoms: 1, ExpiresAt: now + 300},
		}}, now); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
			t.Fatalf("drop chess: %v", err)
		}
		log := mustReadSpendLog(t)
		if log.Spends[0].State != GamingSpendPending {
			t.Fatalf("dropping chess retired poker's request: %q", log.Spends[0].State)
		}
		if log.Spends[1].State != GamingSpendExpired || log.Spends[1].Error != reason {
			t.Fatalf("chess's request ended %q %q, want expired with the reason",
				log.Spends[1].State, log.Spends[1].Error)
		}
	})

	t.Run("revoking a credential retires the game's requests", func(t *testing.T) {
		spendSeams(t)
		if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
			t.Fatalf("store a policy: %v", err)
		}
		if _, err := IssueGamingCredential("poker"); err != nil {
			t.Fatalf("issue: %v", err)
		}
		seedPendingSpend(t, "v1", time.Now().Unix()+300)
		if err := RevokeGamingCredential("poker"); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		got := mustReadSpendLog(t).Spends[0]
		if got.State != GamingSpendExpired || got.Error != reason {
			t.Fatalf("the revoked game's request ended %q %q, want expired with the reason",
				got.State, got.Error)
		}
	})

	t.Run("a dropped game's credential stops resolving", func(t *testing.T) {
		spendSeams(t)
		both := spendPolicy()
		both.RegisteredGames = []string{"poker", "chess"}
		both.Policies["chess"] = both.Policies["poker"]
		if _, err := WriteGamingSettings(both, true); err != nil {
			t.Fatalf("store a policy: %v", err)
		}
		cred, err := IssueGamingCredential("chess")
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		if _, ok := gamingAllow.Resolve(cred.Fingerprint); !ok {
			t.Fatal("a freshly issued credential does not resolve")
		}
		if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
			t.Fatalf("drop chess: %v", err)
		}
		if _, ok := gamingAllow.Resolve(cred.Fingerprint); ok {
			t.Fatal("a dropped game's credential still resolves")
		}
	})
}

// The spend log is both the audit trail and the counter the daily allowance
// is computed from. Read as empty when it cannot be parsed, corruption would
// answer with a day nobody spent and a history nobody kept - so an unreadable
// log refuses every decision instead.
func TestAnUnreadableLogRefusesToDecideAnything(t *testing.T) {
	spendSeams(t)
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		t.Fatal("an unreadable log let an approval reach the wallet")
		return nil, nil
	}
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	if err := writeGarbageSpendLog(); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	if _, err := RequestGamingSpend("poker", "Tsaddr", 1_000_000, "a seat"); err == nil {
		t.Fatal("a request was carried over an unreadable log")
	}
	if _, err := ApproveGamingSpend(context.Background(), "aa11", []byte("right")); err == nil {
		t.Fatal("an approval started over an unreadable log")
	}
	if _, err := DenyGamingSpend("aa11"); err == nil {
		t.Fatal("a deny answered over an unreadable log")
	}
	if _, err := GamingSpends(); err == nil {
		t.Fatal("an unreadable log was served as a history")
	}
	if _, err := GamingSpendFor("poker", "aa11"); err == nil {
		t.Fatal("a status was answered over an unreadable log")
	}
}

// writeGarbageSpendLog overwrites the log with bytes no parser accepts.
func writeGarbageSpendLog() error {
	return os.WriteFile(gamingSpendLogPath(), []byte("{not json"), 0o600)
}

// Money that already moved must be written down even when the log cannot be
// read: the unreadable bytes are kept aside for a person, and the outcome
// starts a fresh record rather than vanishing with the corruption.
func TestAnOutcomeSurvivesAnUnreadableLog(t *testing.T) {
	spendSeams(t)
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("store a policy: %v", err)
	}
	seedPendingSpend(t, "aa11", time.Now().Unix()+300)

	published := 0
	approvalStubs(t, &published)
	spendPublish = func(context.Context, []byte) (string, error) {
		if err := writeGarbageSpendLog(); err != nil {
			t.Fatalf("corrupt: %v", err)
		}
		published++
		return "txid00", nil
	}

	out, err := ApproveGamingSpend(context.Background(), "aa11", []byte("right"))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if out.State != GamingSpendApproved || out.TxID != "txid00" {
		t.Fatalf("the rescue recorded %v txid %q", out.State, out.TxID)
	}

	fresh := mustReadSpendLog(t)
	if len(fresh.Spends) != 1 || fresh.Spends[0].TxID != "txid00" || fresh.Spends[0].State != GamingSpendApproved {
		t.Fatalf("the fresh log holds %+v", fresh.Spends)
	}
	aside, err := filepath.Glob(gamingSpendLogPath() + ".corrupt-*")
	if err != nil || len(aside) != 1 {
		t.Fatalf("the unreadable bytes were not kept aside: %v %v", aside, err)
	}
	kept, err := os.ReadFile(aside[0])
	if err != nil || string(kept) != "{not json" {
		t.Fatalf("the bytes aside read %q (%v), want the corruption byte for byte", kept, err)
	}
	if _, err := RequestGamingSpend("poker", "Tsaddr", 1_000_000, "after the rescue"); err != nil {
		t.Fatalf("the log did not recover after the rescue: %v", err)
	}
}

// Expiry answers requests nobody decided; a payment being broadcast was
// decided, and the network is working on it.
func TestExpiryNeverTouchesAPaymentBeingBroadcast(t *testing.T) {
	now := time.Now().Unix()
	log := spendLog{Spends: []GamingSpend{
		{ID: "a", State: GamingSpendPublishing, ExpiresAt: now - 500},
	}}
	if expireLocked(&log, now) {
		t.Fatal("expiry claimed to change a broadcasting entry")
	}
	if log.Spends[0].State != GamingSpendPublishing {
		t.Fatalf("expiry changed a broadcast to %q", log.Spends[0].State)
	}
}
