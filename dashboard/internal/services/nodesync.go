// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/utils"

	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
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
	// Set only while dcrd is unreachable: the startup explanation and the dcrd
	// log line that classified it.
	StartupNote string `json:"startupNote,omitempty"`
	StartupLog  string `json:"startupLog,omitempty"`
}

const (
	// Refresh faster while the node is not serving, so "recovers automatically"
	// is true within seconds rather than a poll interval.
	nodeSyncFastEvery = 3 * time.Second
	nodeSyncSlowEvery = 20 * time.Second
	// The hint tails a log file, so it is not recomputed on every fast tick.
	nodeHintTTL = 10 * time.Second
)

var (
	nodeSyncMu    sync.RWMutex
	nodeSyncSnap  NodeSyncSnapshot
	nodeBus       eventBus[NodeSyncSnapshot]
	nodeRefreshCh = make(chan struct{}, 1)

	nodeHintMu sync.Mutex
	nodeHint   DaemonStartupState
	nodeHintAt time.Time
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
	return nodeBus.subscribe(8)
}

func broadcastNodeSync(snap NodeSyncSnapshot) {
	nodeBus.publish(snap)
}

// syncFromChainInfo derives the sync state from getblockchaininfo. Shared with
// the dashboard poll so the pushed and polled views cannot disagree.
func syncFromChainInfo(ci *chainjson.GetBlockChainInfoResult) NodeSyncSnapshot {
	snap := NodeSyncSnapshot{
		Blocks:     ci.Blocks,
		Headers:    ci.Headers,
		SyncHeight: ci.SyncHeight,
	}
	if !ci.InitialBlockDownload {
		snap.Status = "running"
		snap.SyncPhase = "synced"
		snap.SyncMessage = "Fully synced"
		snap.SyncProgress = 100
		return snap
	}

	// syncHeight is the sync peer's advertised tip and only rises, so it is the
	// one denominator that cannot make the bar run backwards. dcrd's own
	// verificationprogress is blocks/headers, which falls as headers arrive.
	if ci.SyncHeight <= 0 {
		snap.Status = "connecting"
		snap.SyncPhase = "starting"
		snap.SyncMessage = "Looking for peers"
		return snap
	}

	snap.Status = "syncing"
	if ci.Headers < ci.SyncHeight {
		snap.SyncPhase = "headers"
		snap.SyncMessage = fmt.Sprintf("Fetching block headers: %s of %s",
			utils.FormatNumber(ci.Headers), utils.FormatNumber(ci.SyncHeight))
		snap.SyncProgress = float64(ci.Headers) / float64(ci.SyncHeight) * 100
	} else {
		snap.SyncPhase = "blocks"
		snap.SyncMessage = fmt.Sprintf("Downloading blocks: %s of %s",
			utils.FormatNumber(ci.Blocks), utils.FormatNumber(ci.SyncHeight))
		snap.SyncProgress = float64(ci.Blocks) / float64(ci.SyncHeight) * 100
	}
	if snap.SyncProgress > 100 {
		snap.SyncProgress = 100
	} else if snap.SyncProgress < 0 {
		snap.SyncProgress = 0
	}
	return snap
}

// nodeStartupHint classifies an unreachable dcrd, cached because it tails a log
// file and the refresh loop runs every few seconds while the node is down.
func nodeStartupHint() DaemonStartupState {
	nodeHintMu.Lock()
	defer nodeHintMu.Unlock()
	if !nodeHintAt.IsZero() && time.Since(nodeHintAt) < nodeHintTTL {
		return nodeHint
	}
	// Bounded: the hint resolves the network over RPC first, which is slow to
	// fail while dcrd is down.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	nodeHint, nodeHintAt = DaemonStartupHint(ctx, LogComponentDcrd), time.Now()
	return nodeHint
}

// notReadySnapshot describes a dcrd that is not serving RPC. Heights carry over
// so a restart does not blank the last known tip, but progress does not: a
// preserved 100% would render as a full bar labelled "Starting".
func notReadySnapshot() NodeSyncSnapshot {
	hint := nodeStartupHint()
	prev := GetNodeSyncSnapshot()

	snap := NodeSyncSnapshot{
		Status:      hint.State,
		SyncPhase:   "starting",
		SyncMessage: "Starting up",
		StartupNote: hint.Message,
		StartupLog:  hint.Detail,
		Blocks:      prev.Blocks,
		Headers:     prev.Headers,
		SyncHeight:  prev.SyncHeight,
	}
	if hint.State == "upgrading" {
		snap.SyncMessage = "Upgrading database"
	}
	return snap
}

// RefreshNodeSync recomputes the snapshot from getblockchaininfo and broadcasts
// it to subscribers. A node that is not serving yet is published as its own
// state rather than dropped, so the UI can explain the wait.
func RefreshNodeSync() {
	var (
		snap  NodeSyncSnapshot
		ready *chainjson.GetBlockChainInfoResult
	)
	if rpc.DcrdClient == nil {
		nodeAlertsUnreachable("dcrd RPC client not initialized")
		snap = notReadySnapshot()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		ci, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
		cancel()
		switch {
		case err == nil:
			snap, ready = syncFromChainInfo(ci), ci
		case IsDaemonUnreachable(err):
			nodeAlertsUnreachable(err.Error())
			snap = notReadySnapshot()
		default:
			// Reachable but refusing to answer, so waiting will not fix it.
			nodeAlertsUnreachable(err.Error())
			snap = NodeSyncSnapshot{Status: "error", StartupNote: err.Error()}
		}
	}

	nodeSyncMu.Lock()
	nodeSyncSnap = snap
	nodeSyncMu.Unlock()
	broadcastNodeSync(snap)
	if ready != nil {
		nodeAlertsHealthy(ready.Blocks)
	}
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
	go func() {
		RefreshNodeSync()
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
			case <-time.After(nodeSyncEvery()):
				RefreshNodeSync()
			}
		}
	}()
}

// nodeSyncEvery is the safety-timer interval, shortened while the node is not
// serving so the page recovers promptly once dcrd comes up.
func nodeSyncEvery() time.Duration {
	if GetNodeSyncSnapshot().Status == "running" {
		return nodeSyncSlowEvery
	}
	return nodeSyncFastEvery
}
