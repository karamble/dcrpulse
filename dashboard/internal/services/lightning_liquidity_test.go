// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"dcrpulse/internal/types"
)

// The provider prices the channel itself, and nothing downstream bounds what it
// may ask: upstream's invoice check compares the invoice against the provider's
// own quoted rate, and outbound capacity only says what this node CAN pay. So
// the ceiling here is the only thing standing between a quoted rate and the
// wallet, and it has to hold when the approved figure is zero - which is what a
// free provider quotes.

func quote(feeAtoms int64) types.LiquidityEstimateResponse {
	return types.LiquidityEstimateResponse{ChanSizeAtoms: 1e8, EstimatedFeeAtoms: feeAtoms}
}

func TestApprovedFeeCeilingRefusesAQuoteAboveIt(t *testing.T) {
	// A provider that quoted nothing and then asks 5 DCR. Approving zero must
	// not read as approving anything.
	if err := ApprovedFeeCeiling(0)(context.Background(), quote(5e8)); err == nil {
		t.Fatal("a 5 DCR fee was accepted against a zero ceiling")
	}
	if err := ApprovedFeeCeiling(1e6)(context.Background(), quote(5e8)); err == nil {
		t.Fatal("a 5 DCR fee was accepted against a 0.01 DCR ceiling")
	}
}

// The naive fix - requiring a positive ceiling - would refuse a genuinely free
// channel, which is a real provider policy rather than a missing value.
func TestApprovedFeeCeilingAllowsAFreeChannel(t *testing.T) {
	if err := ApprovedFeeCeiling(0)(context.Background(), quote(0)); err != nil {
		t.Fatalf("a free channel was refused: %v", err)
	}
}

func TestApprovedFeeCeilingIsExact(t *testing.T) {
	if err := ApprovedFeeCeiling(1000)(context.Background(), quote(1000)); err != nil {
		t.Fatalf("a fee equal to the ceiling was refused: %v", err)
	}
	if err := ApprovedFeeCeiling(1000)(context.Background(), quote(1001)); err == nil {
		t.Fatal("a fee one atom over the ceiling was accepted")
	}
}

// Validation below the client check would answer "dcrlnd not available" and
// never name the real reason, so this runs with NO client: that is the state in
// which the two orderings give different answers.
func TestRequestLiquidityValidatesBeforeReachingDcrlnd(t *testing.T) {
	withLightning(t, nil)

	for _, tc := range []struct {
		name    string
		req     *types.RequestLiquidityRequest
		confirm LiquidityConfirmer
		want    string
	}{
		{"no channel size", &types.RequestLiquidityRequest{}, ApprovedFeeCeiling(0), "chanSizeAtoms must be positive"},
		{"negative channel size", &types.RequestLiquidityRequest{ChanSizeAtoms: -1}, ApprovedFeeCeiling(0), "chanSizeAtoms must be positive"},
		{"no confirmation", &types.RequestLiquidityRequest{ChanSizeAtoms: 1e8}, nil, "no fee confirmation supplied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RequestLiquidityChannel(context.Background(), tc.req, tc.confirm)
			if err == nil {
				t.Fatal("the request was accepted")
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
		})
	}
}

// A prompt nobody answers must abort the request rather than hold the payment
// open indefinitely.
func TestPromptLiquidityConfirmAbortsOnTimeout(t *testing.T) {
	withConfirmTimeout(t, 20*time.Millisecond)

	if err := PromptLiquidityConfirm(context.Background(), quote(1e6)); err == nil {
		t.Fatal("an unanswered prompt was treated as approval")
	}
	if p := PendingLiquidityConfirm(); p != nil {
		t.Fatalf("the timed-out prompt is still pending: %+v", p)
	}
}

func TestPromptLiquidityConfirmAbortsOnCancel(t *testing.T) {
	withConfirmTimeout(t, 5*time.Second)

	done := make(chan error, 1)
	go func() { done <- PromptLiquidityConfirm(context.Background(), quote(1e6)) }()

	id := awaitPrompt(t)
	if err := ResolveLiquidityConfirm(id, false); err != nil {
		t.Fatalf("ResolveLiquidityConfirm() = %v, want nil", err)
	}
	if err := <-done; err == nil {
		t.Fatal("a cancelled prompt was treated as approval")
	}
}

func TestPromptLiquidityConfirmProceedsOnApproval(t *testing.T) {
	withConfirmTimeout(t, 5*time.Second)

	done := make(chan error, 1)
	go func() { done <- PromptLiquidityConfirm(context.Background(), quote(1e6)) }()

	if err := ResolveLiquidityConfirm(awaitPrompt(t), true); err != nil {
		t.Fatalf("ResolveLiquidityConfirm() = %v, want nil", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("an approved prompt still refused: %v", err)
	}
}

// The id is what ties an answer to the request it was shown for; without it a
// click left over from an earlier dialog would approve a later payment.
func TestResolveLiquidityConfirmRejectsAnUnknownID(t *testing.T) {
	withConfirmTimeout(t, 5*time.Second)

	if err := ResolveLiquidityConfirm("deadbeef", true); err == nil {
		t.Fatal("an answer was accepted with no prompt pending")
	}

	done := make(chan error, 1)
	go func() { done <- PromptLiquidityConfirm(context.Background(), quote(1e6)) }()
	id := awaitPrompt(t)

	if err := ResolveLiquidityConfirm("deadbeef", true); err == nil {
		t.Fatal("a stale id approved the pending prompt")
	}
	if err := ResolveLiquidityConfirm(id, false); err != nil {
		t.Fatalf("ResolveLiquidityConfirm() = %v, want nil", err)
	}
	<-done
}

func TestPromptLiquidityConfirmRefusesASecondPrompt(t *testing.T) {
	withConfirmTimeout(t, 5*time.Second)

	done := make(chan error, 1)
	go func() { done <- PromptLiquidityConfirm(context.Background(), quote(1e6)) }()
	id := awaitPrompt(t)

	if err := PromptLiquidityConfirm(context.Background(), quote(2e6)); err == nil {
		t.Fatal("a second prompt was raised while one was pending")
	}
	// The refusal must not have disturbed the prompt already waiting.
	if p := PendingLiquidityConfirm(); p == nil || p.ID != id {
		t.Fatalf("the pending prompt changed: %+v", p)
	}
	if err := ResolveLiquidityConfirm(id, false); err != nil {
		t.Fatalf("ResolveLiquidityConfirm() = %v, want nil", err)
	}
	<-done
}

func withConfirmTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := liquidityConfirmTimeout
	liquidityConfirmTimeout = d
	t.Cleanup(func() {
		liquidityConfirmTimeout = prev
		clearPendingConfirm()
	})
}

// clearPendingConfirm drops whatever a failed test left behind, so one failure
// does not cascade into every later single-flight assertion.
func clearPendingConfirm() {
	liquidityConfirmMu.Lock()
	p := liquidityConfirmPending
	liquidityConfirmMu.Unlock()
	if p != nil {
		clearLiquidityConfirm(p.ev.ID)
	}
}

// awaitPrompt waits for the goroutine under test to register its prompt and
// returns its id.
func awaitPrompt(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p := PendingLiquidityConfirm(); p != nil {
			return p.ID
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no prompt was ever raised")
	return ""
}
