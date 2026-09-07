// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"dcrpulse/internal/types"
)

// The browser confirms a liquidity fee the way bruig does: the prompt carries
// the provider's live policy and the request blocks inside PolicyFetched until
// the operator answers, so there is no window in which the quoted rate can move
// between what was shown and what is paid.

// liquidityConfirmTimeout matches bruig's own confirmation window. A package
// var so a test can shorten it.
var liquidityConfirmTimeout = time.Minute

var (
	liquidityConfirmBus eventBus[types.LiquidityConfirmEvent]

	liquidityConfirmMu sync.Mutex
	// One prompt at a time, like bruig's single confirmation channel. A
	// second request refuses immediately rather than queueing behind a
	// minute of someone else's reading.
	liquidityConfirmPending *pendingLiquidityConfirm
)

type pendingLiquidityConfirm struct {
	ev     types.LiquidityConfirmEvent
	answer chan bool
}

// SubscribeLiquidityConfirmEvents returns a channel receiving every future
// prompt plus a cleanup func to call when the subscriber detaches.
func SubscribeLiquidityConfirmEvents() (<-chan types.LiquidityConfirmEvent, func()) {
	return liquidityConfirmBus.subscribe(8)
}

// PendingLiquidityConfirm returns the prompt still waiting for an answer, or
// nil. Deliberately not an event ring like the purchase stream: replaying a
// prompt that has already been answered or timed out would show a reloaded page
// a dialog whose buttons can no longer reach anything.
func PendingLiquidityConfirm() *types.LiquidityConfirmEvent {
	liquidityConfirmMu.Lock()
	defer liquidityConfirmMu.Unlock()
	if liquidityConfirmPending == nil {
		return nil
	}
	ev := liquidityConfirmPending.ev
	return &ev
}

// PromptLiquidityConfirm is the browser's LiquidityConfirmer: it publishes the
// live quote and waits for the operator. Every exit clears the prompt and says
// so, so a second tab showing the same dialog drops it.
func PromptLiquidityConfirm(ctx context.Context, quote types.LiquidityEstimateResponse) error {
	id, err := randomConfirmID()
	if err != nil {
		return err
	}
	p := &pendingLiquidityConfirm{
		ev: types.LiquidityConfirmEvent{
			ID:        id,
			Kind:      "prompt",
			Quote:     quote,
			ExpiresAt: time.Now().UTC().Add(liquidityConfirmTimeout),
		},
		answer: make(chan bool, 1),
	}

	liquidityConfirmMu.Lock()
	if liquidityConfirmPending != nil {
		liquidityConfirmMu.Unlock()
		return fmt.Errorf("another liquidity request is waiting for confirmation")
	}
	liquidityConfirmPending = p
	liquidityConfirmMu.Unlock()

	defer clearLiquidityConfirm(id)
	liquidityConfirmBus.publish(p.ev)

	select {
	case ok := <-p.answer:
		if ok {
			return nil
		}
		return fmt.Errorf("cancelled by the user")
	case <-time.After(liquidityConfirmTimeout):
		return fmt.Errorf("confirmation timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ResolveLiquidityConfirm answers the pending prompt. An id that names no
// pending prompt is an error rather than a silent no-op, so a stale click
// cannot approve a request made after the one it was looking at.
func ResolveLiquidityConfirm(id string, approve bool) error {
	liquidityConfirmMu.Lock()
	p := liquidityConfirmPending
	if p == nil || p.ev.ID != id {
		liquidityConfirmMu.Unlock()
		return fmt.Errorf("no liquidity request is waiting for this confirmation")
	}
	liquidityConfirmMu.Unlock()

	select {
	case p.answer <- approve:
	default: // already answered; the waiter is on its way out
	}
	return nil
}

// clearLiquidityConfirm drops the prompt and tells subscribers it is gone.
func clearLiquidityConfirm(id string) {
	liquidityConfirmMu.Lock()
	if liquidityConfirmPending == nil || liquidityConfirmPending.ev.ID != id {
		liquidityConfirmMu.Unlock()
		return
	}
	ev := liquidityConfirmPending.ev
	liquidityConfirmPending = nil
	liquidityConfirmMu.Unlock()

	ev.Kind = "resolved"
	liquidityConfirmBus.publish(ev)
}

func randomConfirmID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate confirmation id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
