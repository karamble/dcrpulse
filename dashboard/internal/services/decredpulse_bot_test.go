// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"
)

// The solver must stop at its caller's deadline instead of grinding the
// iteration cap out - the cap alone costs over a minute of CPU.
func TestSolvePoWStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := solvePoW(ctx, "nonce", powMaxBits)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a cancelled solve reported success")
	}
	if ctx.Err() == nil || err.Error() != ctx.Err().Error() {
		t.Fatalf("solve returned %v, want the context's error", err)
	}
	if elapsed > time.Second {
		t.Fatalf("solve ran %s after cancellation, want a prompt stop", elapsed)
	}
}

// A challenge above the ceiling is refused before any hashing: expected work
// is 2^bits, so it could not finish inside the deadline or the cap anyway.
func TestSolvePoWRefusesUnreasonableBits(t *testing.T) {
	start := time.Now()
	_, err := solvePoW(context.Background(), "nonce", powMaxBits+1)
	if err == nil {
		t.Fatal("an unsolvable challenge was accepted")
	}
	if time.Since(start) > 10*time.Millisecond {
		t.Fatal("the refusal iterated instead of returning immediately")
	}
}

// The happy path still solves, and the solution verifies the same way the
// brulse bot checks it.
func TestSolvePoWSolves(t *testing.T) {
	sol, err := solvePoW(context.Background(), "nonce", 8)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	sum := sha256.Sum256([]byte("nonce:" + sol))
	if leadingZeroBits(sum[:]) < 8 {
		t.Fatalf("solution %q does not meet the difficulty", sol)
	}
}
