// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"sync"
	"time"

	"dcrpulse/internal/alerts"
	"dcrpulse/internal/rpc"
)

// Node alert probes are driven by RefreshNodeSync (block-connected triggers
// plus its 20s safety ticker), so the conditions keep evaluating while dcrd
// is down and the engine's debounce sees a steady observation stream.
var nodeAlertsState struct {
	sync.Mutex
	lastPeerProbe time.Time
	lastHeight    int64
	lastAdvance   time.Time
}

const (
	nodePeerProbeEvery = 30 * time.Second
	chainStallAfter    = 60 * time.Minute
)

// nodeAlertsUnreachable records a failed dcrd probe.
func nodeAlertsUnreachable(detail string) {
	alerts.Raise("dcrd_unreachable", "", detail)
}

// nodeAlertsHealthy records a successful getblockchaininfo pass: resolves the
// unreachable condition, tracks tip advance for chain_sync_stalled, and
// rate-limits a getpeerinfo probe for dcrd_no_peers.
func nodeAlertsHealthy(blocks int64) {
	alerts.Resolve("dcrd_unreachable", "")

	st := &nodeAlertsState
	st.Lock()
	now := time.Now()
	if st.lastAdvance.IsZero() {
		// Seed to the first healthy pass so a fresh boot cannot false-fire
		// the stall condition.
		st.lastAdvance = now
		st.lastHeight = blocks
	}
	if blocks != st.lastHeight {
		st.lastHeight = blocks
		st.lastAdvance = now
	}
	stalled := now.Sub(st.lastAdvance) >= chainStallAfter
	probePeers := now.Sub(st.lastPeerProbe) >= nodePeerProbeEvery
	if probePeers {
		st.lastPeerProbe = now
	}
	st.Unlock()

	if stalled {
		alerts.Raise("chain_sync_stalled", "", "no new block for over an hour")
	} else {
		alerts.Resolve("chain_sync_stalled", "")
	}

	if !probePeers {
		return
	}
	client := rpc.DcrdClient
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	peers, err := client.GetPeerInfo(ctx)
	cancel()
	if err != nil {
		// Connectivity problems are the unreachable condition's job; a failed
		// peer probe alone must not raise or resolve anything.
		return
	}
	if len(peers) == 0 {
		alerts.Raise("dcrd_no_peers", "", "dcrd reports zero connected peers")
	} else {
		alerts.Resolve("dcrd_no_peers", "")
	}
}

// resetAlertWatcherBaselines clears per-wallet alert watcher baselines after
// a wallet switch so the new wallet's state is seeded silently instead of
// flooding events.
func resetAlertWatcherBaselines() {
	resetTicketAlertBaseline()
}
