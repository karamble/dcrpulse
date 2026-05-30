// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/utils"
)

// NodeSyncSnapshot is the current dcrd blockchain sync state, sourced from
// getblockchaininfo and pushed to the UI (verificationProgress is the source of
// truth; dcrd block-connected notifications are only the refresh trigger).
type NodeSyncSnapshot struct {
	Status       string  `json:"status"`
	SyncProgress float64 `json:"syncProgress"`
	SyncPhase    string  `json:"syncPhase"`
	SyncMessage  string  `json:"syncMessage"`
	Blocks       int64   `json:"blocks"`
	Headers      int64   `json:"headers"`
	SyncHeight   int64   `json:"syncHeight"`
}

var (
	nodeSyncMu     sync.RWMutex
	nodeSyncSnap   NodeSyncSnapshot
	nodeSubsMu     sync.Mutex
	nodeSubs       []chan NodeSyncSnapshot
	nodeRefreshCh  = make(chan struct{}, 1)
)

// GetNodeSyncSnapshot returns a copy of the current node sync snapshot.
func GetNodeSyncSnapshot() NodeSyncSnapshot {
	nodeSyncMu.RLock()
	defer nodeSyncMu.RUnlock()
	return nodeSyncSnap
}

// SubscribeNodeSyncEvents returns a channel that receives every snapshot update
// and a cleanup func.
func SubscribeNodeSyncEvents() (<-chan NodeSyncSnapshot, func()) {
	ch := make(chan NodeSyncSnapshot, 8)
	nodeSubsMu.Lock()
	nodeSubs = append(nodeSubs, ch)
	nodeSubsMu.Unlock()
	return ch, func() {
		nodeSubsMu.Lock()
		defer nodeSubsMu.Unlock()
		for i, sub := range nodeSubs {
			if sub == ch {
				nodeSubs = append(nodeSubs[:i], nodeSubs[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

func broadcastNodeSync(snap NodeSyncSnapshot) {
	nodeSubsMu.Lock()
	for _, sub := range nodeSubs {
		select {
		case sub <- snap:
		default:
		}
	}
	nodeSubsMu.Unlock()
}

// RefreshNodeSync recomputes the snapshot from getblockchaininfo and broadcasts
// it to subscribers.
func RefreshNodeSync() {
	if rpc.DcrdClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ci, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
	if err != nil {
		return
	}

	snap := NodeSyncSnapshot{
		Blocks:     ci.Blocks,
		Headers:    ci.Headers,
		SyncHeight: ci.SyncHeight,
	}
	if ci.InitialBlockDownload {
		snap.Status = "syncing"
		if ci.Blocks == 0 && ci.Headers > 0 {
			snap.SyncPhase = "headers"
			snap.SyncMessage = "Fetching headers..."
			if ci.SyncHeight > 0 {
				snap.SyncProgress = float64(ci.Headers) / float64(ci.SyncHeight) * 100
			}
		} else if ci.Blocks > 0 {
			snap.SyncPhase = "blocks"
			snap.SyncMessage = "Synced to block " + utils.FormatNumber(ci.Blocks)
			if ci.SyncHeight > 0 {
				snap.SyncProgress = float64(ci.Blocks) / float64(ci.SyncHeight) * 100
			}
		} else {
			snap.SyncPhase = "starting"
			snap.SyncMessage = "Starting sync..."
		}
		// verificationProgress is the most accurate signal when available.
		if ci.VerificationProgress > 0 {
			snap.SyncProgress = ci.VerificationProgress * 100
		}
	} else {
		snap.Status = "running"
		snap.SyncPhase = "synced"
		snap.SyncMessage = "Fully synced"
		snap.SyncProgress = 100
	}
	if snap.SyncProgress > 100 {
		snap.SyncProgress = 100
	} else if snap.SyncProgress < 0 {
		snap.SyncProgress = 0
	}

	nodeSyncMu.Lock()
	nodeSyncSnap = snap
	nodeSyncMu.Unlock()
	broadcastNodeSync(snap)
}

// TriggerNodeSyncRefresh requests a refresh (non-blocking, coalesced). Called
// from the dcrd block-connected notification handler.
func TriggerNodeSyncRefresh() {
	select {
	case nodeRefreshCh <- struct{}{}:
	default:
	}
}

// StartNodeSync seeds the snapshot and runs the refresh loop: it reacts to
// block-connected triggers (throttled to avoid a getblockchaininfo call per
// block during IBD) and refreshes on a slow safety timer in case notifications
// stall.
func StartNodeSync(ctx context.Context) {
	RefreshNodeSync()
	go func() {
		safety := time.NewTicker(20 * time.Second)
		defer safety.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-nodeRefreshCh:
				RefreshNodeSync()
				// Throttle bursts: during IBD blocks connect rapidly, so cap the
				// refresh rate and drop any trigger that arrived meanwhile.
				time.Sleep(time.Second)
				select {
				case <-nodeRefreshCh:
				default:
				}
			case <-safety.C:
				RefreshNodeSync()
			}
		}
	}()
}
