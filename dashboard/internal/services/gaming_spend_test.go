// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/types"
)

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

func withPokerPolicy(s types.GamingSettings, edit func(*types.GamePolicy)) types.GamingSettings {
	p := s.Policies["poker"]
	edit(&p)
	s.Policies = map[string]types.GamePolicy{"poker": p}
	return s
}

func spendSeams(t *testing.T) {
	t.Helper()
	origDir, origAccount := GamingStateDir, spendAccount
	origConstruct, origSign, origPublish := spendConstruct, spendSign, spendPublish
	origDecode := spendDecodeAddress
	GamingStateDir = t.TempDir()
	spendDecodeAddress = func(context.Context, string) error { return nil }
	spendMu.Lock()
	spendApproving = map[string]bool{}
	spendMu.Unlock()
	t.Cleanup(func() {
		GamingStateDir, spendAccount = origDir, origAccount
		spendConstruct, spendSign, spendPublish = origConstruct, origSign, origPublish
		spendDecodeAddress = origDecode
		spendMu.Lock()
		spendApproving = map[string]bool{}
		spendMu.Unlock()
	})
}

func mustReadSpendLog(t *testing.T) spendLog {
	t.Helper()
	log, err := readSpendLog()
	if err != nil {
		t.Fatalf("read spend log: %v", err)
	}
	return log
}

func seedPendingSpend(t *testing.T, id string, expiresAt int64) {
	t.Helper()
	if err := writeSpendLog(spendLog{Spends: []GamingSpend{{
		ID: id, Game: "poker", Address: "Tsaddr", AmountAtoms: 1_000_000,
		State: GamingSpendPending, RequestedAt: time.Now().Unix(), ExpiresAt: expiresAt,
	}}}, time.Now().Unix()); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestPolicyRefusesInvalidRequests(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings func(types.GamingSettings) types.GamingSettings
		game     string
		address  string
		amount   int64
		want     error
	}{
		{"switched off", func(s types.GamingSettings) types.GamingSettings { s.Enabled = false; return s }, "poker", "Tsaddr", 1, ErrGamingSpendRefused},
		{"no account", func(s types.GamingSettings) types.GamingSettings {
			return withPokerPolicy(s, func(p *types.GamePolicy) { p.Account = " " })
		}, "poker", "Tsaddr", 1, ErrGamingSpendRefused},
		{"unregistered game", func(s types.GamingSettings) types.GamingSettings { return s }, "chess", "Tsaddr", 1, ErrGamingGameNotRegistered},
		{"no address", func(s types.GamingSettings) types.GamingSettings { return s }, "poker", " ", 1, ErrGamingSpendRefused},
		{"zero amount", func(s types.GamingSettings) types.GamingSettings { return s }, "poker", "Tsaddr", 0, ErrGamingSpendRefused},
		{"over table cap", func(s types.GamingSettings) types.GamingSettings { return s }, "poker", "Tsaddr", 100_000_001, ErrGamingSpendRefused},
		{"over monetary bound", func(s types.GamingSettings) types.GamingSettings { return s }, "poker", "Tsaddr", maxSpendAtoms + 1, ErrGamingSpendRefused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := checkSpendRequest(tc.settings(spendPolicy()), tc.game, tc.address, tc.amount)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// The retired address-only entry point must remain fail-closed even for a
// request that is otherwise inside policy. It must not consult the wallet.
func TestAddressOnlySpendCannotCreateAnApproval(t *testing.T) {
	spendSeams(t)
	spendAccount = func(context.Context, string) (uint32, error) {
		t.Fatal("an address-only request reached the wallet account")
		return 0, nil
	}
	spendConstruct = func(context.Context, uint32, string, int64) ([]byte, error) {
		t.Fatal("an address-only request constructed a transaction")
		return nil, nil
	}
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		t.Fatal("an address-only request reached signing")
		return nil, nil
	}
	spendPublish = func(context.Context, []byte) (string, error) {
		t.Fatal("an address-only request reached broadcast")
		return "", nil
	}

	_, err := RequestGamingSpend(context.Background(), "poker", "Tsaddr", 10_000_000, "a seat")
	if !errors.Is(err, ErrGamingSpendRefused) || !strings.Contains(err.Error(), "verified deposit") {
		t.Fatalf("address-only request returned %v", err)
	}
	if _, statErr := os.Stat(gamingSpendLogPath()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a refused request wrote a spend log: %v", statErr)
	}
}

func TestDayAccountingIsPerGameAndCannotOverflow(t *testing.T) {
	now := time.Now().Unix()
	log := spendLog{Spends: []GamingSpend{
		{Game: "poker", State: GamingSpendApproved, AmountAtoms: 100, DecidedAt: now - 60},
		{Game: "poker", State: GamingSpendPending, AmountAtoms: 50},
		{Game: "poker", State: GamingSpendDenied, AmountAtoms: 900, DecidedAt: now - 60},
		{Game: "chess", State: GamingSpendApproved, AmountAtoms: 700, DecidedAt: now - 60},
		{Game: "poker", State: GamingSpendApproved, AmountAtoms: 8, DecidedAt: now - int64((24 * time.Hour).Seconds()) - 1},
	}}
	if got := spentInDayLocked(log, "poker", now, ""); got != 150 {
		t.Fatalf("poker day total %d, want 150", got)
	}
	if got := spentInDayLocked(log, "chess", now, ""); got != 700 {
		t.Fatalf("chess day total %d, want 700", got)
	}
	log.Spends = append(log.Spends, GamingSpend{Game: "poker", State: GamingSpendPending, AmountAtoms: math.MaxInt64})
	if got := spentInDayLocked(log, "poker", now, ""); got != math.MaxInt64 {
		t.Fatalf("overflowing total became %d", got)
	}
}

func TestDailyCapCannotBeWrapped(t *testing.T) {
	p := spendPolicy().Policies["poker"]
	for _, tc := range []struct {
		amount int64
		used   int64
		ok     bool
	}{
		{p.PerDayCapAtoms - 100, 100, true},
		{p.PerDayCapAtoms, 1, false},
		{math.MaxInt64, 100, false},
		{1, -1, false},
	} {
		err := checkSpendAgainstDay(p, tc.amount, tc.used)
		if tc.ok && err != nil {
			t.Errorf("amount %d used %d refused: %v", tc.amount, tc.used, err)
		}
		if !tc.ok && !errors.Is(err, ErrGamingSpendOverCap) {
			t.Errorf("amount %d used %d returned %v", tc.amount, tc.used, err)
		}
	}
}

func TestUnansweredRequestExpires(t *testing.T) {
	now := time.Now().Unix()
	log := spendLog{Spends: []GamingSpend{
		{ID: "a", State: GamingSpendPending, ExpiresAt: now - 1},
		{ID: "b", State: GamingSpendPending, ExpiresAt: now + 60},
		{ID: "c", State: GamingSpendPublishing, ExpiresAt: now - 1},
	}}
	if !expireLocked(&log, now) {
		t.Fatal("overdue request was not expired")
	}
	if log.Spends[0].State != GamingSpendExpired || log.Spends[1].State != GamingSpendPending || log.Spends[2].State != GamingSpendPublishing {
		t.Fatalf("unexpected expiry states: %+v", log.Spends)
	}
}

func TestAuthorityApprovalRebuildsExactDashboardRequest(t *testing.T) {
	approval := gamingfunds.FundingApproval{
		ID:    "approval-1",
		Scope: gamingfunds.Scope{Game: "poker", Network: "simnet", Wallet: "wallet", Account: 2},
		Deposit: gamingfunds.Deposit{
			ID: "deposit-1", Address: "SsDeposit",
			Terms: gamingfunds.Terms{Table: "a1", Kind: "stake", Atoms: 2_000_000, LockBlocks: 288},
		},
		Preview: gamingfunds.PaymentPreview{RequestedAt: 100, ExpiresAt: 200, Reason: "fund stake", FeeAtoms: 123},
	}
	got := spendFromFundingApproval(approval, 150)
	if got.ID != approval.ID || got.Game != "poker" || got.DepositID != "deposit-1" || got.TableID != "a1" || got.DepositKind != "stake" || got.Address != "SsDeposit" || got.AmountAtoms != 2_000_000 || got.FundingFeeAtoms != 123 || got.RecoveryLockBlocks != 288 || got.State != GamingSpendPending {
		t.Fatalf("rebuilt request differs from authority record: %+v", got)
	}
	if expired := spendFromFundingApproval(approval, 201); expired.State != GamingSpendExpired || expired.DecidedAt != 200 {
		t.Fatalf("expired authority request rebuilt as %+v", expired)
	}
}

// A hand-written pending record is not financial authority. Approval must
// fail before signing or publishing because no immutable deposit backs it.
func TestUnverifiedPendingSpendCannotBeApproved(t *testing.T) {
	spendSeams(t)
	if _, err := WriteGamingSettings(spendPolicy(), true, true); err != nil {
		t.Fatalf("store policy: %v", err)
	}
	seedPendingSpend(t, "aa11", time.Now().Unix()+300)
	spendAccount = func(context.Context, string) (uint32, error) { return 1, nil }
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		t.Fatal("unverified request reached signing")
		return nil, nil
	}
	spendPublish = func(context.Context, []byte) (string, error) {
		t.Fatal("unverified request reached broadcast")
		return "", nil
	}

	if _, err := ApproveGamingSpend(context.Background(), "aa11", []byte("pass")); err == nil {
		t.Fatal("unverified request was approved")
	}
	if got := mustReadSpendLog(t).Spends[0].State; got != GamingSpendPending {
		t.Fatalf("refused approval changed state to %q", got)
	}
}

func TestExpiredRequestCannotBeApproved(t *testing.T) {
	spendSeams(t)
	seedPendingSpend(t, "bb22", time.Now().Unix()-1)
	spendSign = func(context.Context, uint32, []byte, []byte) ([]byte, error) {
		t.Fatal("expired request reached signing")
		return nil, nil
	}
	if _, err := ApproveGamingSpend(context.Background(), "bb22", []byte("pass")); !errors.Is(err, ErrGamingSpendNotPending) {
		t.Fatalf("expired approval returned %v", err)
	}
}

func TestPublishingAndOutcomeTransitionsAreDurable(t *testing.T) {
	spendSeams(t)
	seedPendingSpend(t, "cc33", time.Now().Unix()+300)
	if err := markSpendPublishing("cc33"); err != nil {
		t.Fatalf("mark publishing: %v", err)
	}
	if got := mustReadSpendLog(t).Spends[0].State; got != GamingSpendPublishing {
		t.Fatalf("state before broadcast is %q", got)
	}
	out, err := recordSpendOutcome(GamingSpend{ID: "cc33"}, GamingSpendApproved, "txid00", "")
	if err != nil || out.State != GamingSpendApproved || out.TxID != "txid00" {
		t.Fatalf("record outcome: %+v, %v", out, err)
	}
	if _, err := recordSpendOutcome(out, GamingSpendApproved, "txid99", ""); err == nil {
		t.Fatal("decided request was rewritten")
	}
}

func TestAmbiguousBroadcastStaysPublishingForReconciliation(t *testing.T) {
	spendSeams(t)
	seedPendingSpend(t, "dd44", time.Now().Unix()+300)
	if err := markSpendPublishing("dd44"); err != nil {
		t.Fatalf("mark publishing: %v", err)
	}
	const warning = "broadcast outcome unknown; bridge will reconcile the recorded transaction"
	out, err := recordSpendOutcome(GamingSpend{ID: "dd44"}, GamingSpendPublishing, "possible-txid", warning)
	if err == nil {
		t.Fatal("ambiguous broadcast reported success")
	}
	if out.State != GamingSpendPublishing || out.TxID != "possible-txid" || out.Error != warning {
		t.Fatalf("ambiguous outcome was not retained: %+v", out)
	}
	if sweepPublishingLocked(&spendLog{Spends: []GamingSpend{out}}, time.Now().Add(24*time.Hour).Unix()) {
		t.Fatal("unreconciled broadcast was swept")
	}
}

func TestDenyRefusesPublishingPayment(t *testing.T) {
	spendSeams(t)
	now := time.Now().Unix()
	if err := writeSpendLog(spendLog{Spends: []GamingSpend{{
		ID: "ee55", Game: "poker", State: GamingSpendPublishing,
		RequestedAt: now, ExpiresAt: now + 300,
	}}}, now); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := DenyGamingSpend("ee55"); !errors.Is(err, ErrGamingSpendNotPending) {
		t.Fatalf("deny returned %v", err)
	}
	if got := mustReadSpendLog(t).Spends[0].State; got != GamingSpendPublishing {
		t.Fatalf("deny changed publishing state to %q", got)
	}
}

func TestPublishingCountsAsOutstandingAndStaysOnWirePending(t *testing.T) {
	now := time.Now().Unix()
	log := spendLog{Spends: []GamingSpend{
		{ID: "publishing", Game: "poker", State: GamingSpendPublishing, AmountAtoms: 70, ExpiresAt: now + 300},
		{ID: "pending", Game: "poker", State: GamingSpendPending, AmountAtoms: 30, ExpiresAt: now + 300},
	}}
	if got := spentInDayLocked(log, "poker", now, ""); got != 100 {
		t.Fatalf("day total %d, want 100", got)
	}
	if !spendOutstanding(log.Spends[0]) {
		t.Fatal("publishing payment is not outstanding")
	}
	if got := spendProto(log.Spends[0]).State; got != "pending" {
		t.Fatalf("publishing rides the game wire as %q", got)
	}
}

func TestSettingsChangeOnlyRetiresPendingRequests(t *testing.T) {
	spendSeams(t)
	if _, err := WriteGamingSettings(spendPolicy(), true, true); err != nil {
		t.Fatalf("store policy: %v", err)
	}
	now := time.Now().Unix()
	if err := writeSpendLog(spendLog{Spends: []GamingSpend{
		{ID: "p", Game: "poker", State: GamingSpendPending, AmountAtoms: 1, ExpiresAt: now + 300},
		{ID: "b", Game: "poker", State: GamingSpendPublishing, AmountAtoms: 1, ExpiresAt: now + 300},
	}}, now); err != nil {
		t.Fatalf("seed: %v", err)
	}
	off := spendPolicy()
	off.Enabled = false
	if _, err := WriteGamingSettings(off, true, true); err != nil {
		t.Fatalf("disable: %v", err)
	}
	log := mustReadSpendLog(t)
	if log.Spends[0].State != GamingSpendExpired || log.Spends[1].State != GamingSpendPublishing {
		t.Fatalf("settings change produced %+v", log.Spends)
	}
}

func TestAddressMustBelongToActiveNetwork(t *testing.T) {
	mainnet := chaincfg.MainNetParams()
	testnet := chaincfg.TestNet3Params()
	addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(make([]byte, 20), mainnet)
	if err != nil {
		t.Fatalf("address: %v", err)
	}
	if err := checkSpendAddress(addr.String(), mainnet); err != nil {
		t.Fatalf("mainnet address refused on mainnet: %v", err)
	}
	if err := checkSpendAddress(addr.String(), testnet); !errors.Is(err, ErrGamingSpendRefused) {
		t.Fatalf("wrong-network address returned %v", err)
	}
}

func TestUnreadableAuditLogRefusesEveryDecision(t *testing.T) {
	spendSeams(t)
	if _, err := WriteGamingSettings(spendPolicy(), true, true); err != nil {
		t.Fatalf("store policy: %v", err)
	}
	if err := os.WriteFile(gamingSpendLogPath(), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("corrupt log: %v", err)
	}
	if _, err := RequestGamingSpend(context.Background(), "poker", "Tsaddr", 1, "seat"); err == nil {
		t.Fatal("legacy request crossed corrupt log")
	}
	if _, err := ApproveGamingSpend(context.Background(), "x", []byte("pass")); err == nil {
		t.Fatal("approval crossed corrupt log")
	}
	if _, err := DenyGamingSpend("x"); err == nil {
		t.Fatal("denial crossed corrupt log")
	}
	if _, _, err := GamingSpendLedger(); err == nil {
		t.Fatal("corrupt log was served as empty history")
	}
}
