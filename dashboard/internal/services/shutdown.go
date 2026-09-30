// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"dcrpulse/internal/alerts"
)

// ErrShuttingDown refuses work that would unlock an account once the dashboard
// has begun to stop.
var ErrShuttingDown = errors.New("the dashboard is shutting down")

var shuttingDown atomic.Bool

// BeginShutdown refuses every new account unlock from here on.
func BeginShutdown() { shuttingDown.Store(true) }

// ShuttingDown reports whether BeginShutdown has been called.
func ShuttingDown() bool { return shuttingDown.Load() }

// shutdownSweepTimeout bounds the final relock, which runs even when ctx has
// already expired.
const shutdownSweepTimeout = 15 * time.Second

// StopWalletWork is the dashboard's part of closing the wallet, in the order of
// Decrediton's finalCloseWallet: the mixer, the autobuyer and the vote trickles
// stop and relock what they opened, in-flight spends finish, and every account
// nothing needs is locked. It waits no longer than ctx allows.
func StopWalletWork(ctx context.Context) {
	BeginShutdown()
	StopMixer()
	StopAutobuyer()
	stopAllVoteTrickles()
	waitUntil(ctx, func() bool {
		return !IsMixerRunning() && !IsAutobuyerRunning() && !voteTricklesRunning()
	})
	waitUntil(ctx, func() bool {
		return unlockedOps.Load() == 0 && !IsTicketPurchaseInProgress() &&
			!RestoreDiscoveryActive() && !SyncPaused()
	})
	if IsTicketPurchaseInProgress() {
		wlltLog.Warnf("shutdown: a ticket purchase is still running and is cut off")
		emitShutdownAlert("ticket_purchase_interrupted",
			"The dashboard stopped while a ticket purchase was running. Check My Tickets for what was bought.", "")
	}
	sweepCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownSweepTimeout)
	defer cancel()
	sweepLockableAccounts(sweepCtx)
}

// emitShutdownAlert records the interrupted purchase; a variable for tests.
var emitShutdownAlert = alerts.Emit

func waitUntil(ctx context.Context, done func() bool) {
	for !done() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func stopAllVoteTrickles() {
	vtMu.Lock()
	var cancels []context.CancelFunc
	for _, st := range vtRuns {
		if !st.done && st.cancel != nil {
			cancels = append(cancels, st.cancel)
		}
	}
	vtMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func voteTricklesRunning() bool {
	vtMu.Lock()
	defer vtMu.Unlock()
	if len(vtStarting) > 0 {
		return true
	}
	for _, st := range vtRuns {
		if !st.done {
			return true
		}
	}
	return false
}
