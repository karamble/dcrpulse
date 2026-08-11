// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"sync"
	"time"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

// SyncPhase is the active stage of wallet sync.
type SyncPhase string

const (
	SyncPhaseUnknown           SyncPhase = "unknown"
	SyncPhaseUnsynced          SyncPhase = "unsynced"
	SyncPhaseFetchingCfilters  SyncPhase = "fetching_cfilters"
	SyncPhaseFetchingHeaders   SyncPhase = "fetching_headers"
	SyncPhaseDiscoverAddresses SyncPhase = "discovering_addresses"
	SyncPhaseRescanning        SyncPhase = "rescanning"
	SyncPhaseSynced            SyncPhase = "synced"
)

// SyncSnapshot is the current wallet sync state.
type SyncSnapshot struct {
	Phase            SyncPhase `json:"phase"`
	DaemonConnected  bool      `json:"daemonConnected"`
	PeerCount        uint32    `json:"peerCount"`
	CfiltersStart    int32     `json:"cfiltersStart"`
	CfiltersEnd      int32     `json:"cfiltersEnd"`
	HeadersCount     int32     `json:"headersCount"`
	LastHeaderTime   int64     `json:"lastHeaderTime"`
	FirstHeaderTime  int64     `json:"firstHeaderTime"`
	RescanFrom       int64     `json:"rescanFrom"`
	RescanThrough    int32     `json:"rescanThrough"`
	RescanProgressPc float64   `json:"rescanProgress"`
	LastNotification time.Time `json:"lastNotification"`
	LastError        string    `json:"lastError,omitempty"`
}

var (
	syncMu   sync.RWMutex
	syncSnap = SyncSnapshot{Phase: SyncPhaseUnknown}
	syncBus  eventBus[SyncSnapshot]
)

// GetSyncSnapshot returns a copy of the current snapshot.
func GetSyncSnapshot() SyncSnapshot {
	syncMu.RLock()
	defer syncMu.RUnlock()
	return syncSnap
}

// SubscribeSyncEvents returns a channel that receives every snapshot update and a cleanup func.
func SubscribeSyncEvents() (<-chan SyncSnapshot, func()) {
	return syncBus.subscribe(8)
}

// rescanBaseHeight reads the chain height a rescan's progress is measured
// against. It runs unlocked because the call can take seconds and every reader
// of the snapshot would otherwise wait behind it; callers store the result
// under syncMu.
func rescanBaseHeight() int64 {
	if rpc.DcrdClient == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	h, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		return 0
	}
	return h
}

// ApplyRpcSyncNotification updates the snapshot from one RpcSync response.
func ApplyRpcSyncNotification(resp *pb.RpcSyncResponse) {
	if resp == nil {
		return
	}

	// A starting rescan needs a chain height to measure against. Fetch it
	// before locking; a value that turns out to be unneeded is discarded.
	var rescanBase int64
	if resp.NotificationType == pb.SyncNotificationType_RESCAN_STARTED {
		syncMu.RLock()
		need := syncSnap.RescanFrom == 0
		syncMu.RUnlock()
		if need {
			rescanBase = rescanBaseHeight()
		}
	}

	syncMu.Lock()
	syncSnap.LastNotification = time.Now().UTC()
	// Receiving any notification implies the gRPC stream is alive; peer
	// notifications are separate (wallet's P2P peers, not dcrd RPC).
	syncSnap.DaemonConnected = true

	switch resp.NotificationType {
	case pb.SyncNotificationType_SYNCED:
		syncSnap.Phase = SyncPhaseSynced
		syncSnap.RescanProgressPc = 100
	case pb.SyncNotificationType_UNSYNCED:
		syncSnap.Phase = SyncPhaseUnsynced
	case pb.SyncNotificationType_PEER_CONNECTED:
		if resp.PeerInformation != nil {
			syncSnap.PeerCount = uint32(resp.PeerInformation.PeerCount)
		} else {
			syncSnap.PeerCount++
		}
	case pb.SyncNotificationType_PEER_DISCONNECTED:
		if resp.PeerInformation != nil {
			syncSnap.PeerCount = uint32(resp.PeerInformation.PeerCount)
		} else if syncSnap.PeerCount > 0 {
			syncSnap.PeerCount--
		}
	case pb.SyncNotificationType_FETCHED_MISSING_CFILTERS_STARTED:
		syncSnap.Phase = SyncPhaseFetchingCfilters
		syncSnap.CfiltersStart = 0
		syncSnap.CfiltersEnd = 0
	case pb.SyncNotificationType_FETCHED_MISSING_CFILTERS_PROGRESS:
		syncSnap.Phase = SyncPhaseFetchingCfilters
		if resp.FetchMissingCfilters != nil {
			syncSnap.CfiltersStart = resp.FetchMissingCfilters.FetchedCfiltersStartHeight
			syncSnap.CfiltersEnd = resp.FetchMissingCfilters.FetchedCfiltersEndHeight
		}
	case pb.SyncNotificationType_FETCHED_MISSING_CFILTERS_FINISHED:
	case pb.SyncNotificationType_FETCHED_HEADERS_STARTED:
		syncSnap.Phase = SyncPhaseFetchingHeaders
		syncSnap.HeadersCount = 0
		syncSnap.FirstHeaderTime = 0
		syncSnap.LastHeaderTime = 0
	case pb.SyncNotificationType_FETCHED_HEADERS_PROGRESS:
		syncSnap.Phase = SyncPhaseFetchingHeaders
		if resp.FetchHeaders != nil {
			// FetchedHeadersCount is the per-batch count from
			// dcrwallet/chain.getHeaders, not a running total, so accumulate.
			syncSnap.HeadersCount += resp.FetchHeaders.FetchedHeadersCount
			syncSnap.LastHeaderTime = resp.FetchHeaders.LastHeaderTime
			if syncSnap.FirstHeaderTime == 0 {
				syncSnap.FirstHeaderTime = resp.FetchHeaders.LastHeaderTime
			}
		}
	case pb.SyncNotificationType_FETCHED_HEADERS_FINISHED:
		syncSnap.Phase = SyncPhaseFetchingHeaders
	case pb.SyncNotificationType_DISCOVER_ADDRESSES_STARTED:
		syncSnap.Phase = SyncPhaseDiscoverAddresses
	case pb.SyncNotificationType_DISCOVER_ADDRESSES_FINISHED:
	case pb.SyncNotificationType_RESCAN_STARTED:
		syncSnap.Phase = SyncPhaseRescanning
		syncSnap.RescanThrough = 0
		syncSnap.RescanProgressPc = 0
		if syncSnap.RescanFrom == 0 && rescanBase > 0 {
			syncSnap.RescanFrom = rescanBase
		}
	case pb.SyncNotificationType_RESCAN_PROGRESS:
		syncSnap.Phase = SyncPhaseRescanning
		if resp.RescanProgress != nil {
			syncSnap.RescanThrough = resp.RescanProgress.RescannedThrough
			if syncSnap.RescanFrom > 0 {
				pct := float64(syncSnap.RescanThrough) / float64(syncSnap.RescanFrom) * 100
				if pct > 100 {
					pct = 100
				}
				syncSnap.RescanProgressPc = pct
			}
		}
	case pb.SyncNotificationType_RESCAN_FINISHED:
		syncSnap.Phase = SyncPhaseSynced
		syncSnap.RescanProgressPc = 100
	}

	if resp.Synced {
		syncSnap.Phase = SyncPhaseSynced
		syncSnap.RescanProgressPc = 100
	}

	snap := syncSnap
	syncMu.Unlock()

	broadcastSyncSnapshot(snap)
}

// MarkRescanProgress updates the snapshot from a user-initiated WalletService.Rescan stream.
func MarkRescanProgress(rescannedThrough int32) {
	// Same as above: resolve the denominator before taking the lock.
	syncMu.RLock()
	need := syncSnap.RescanFrom == 0
	syncMu.RUnlock()
	var rescanBase int64
	if need {
		rescanBase = rescanBaseHeight()
	}

	syncMu.Lock()
	syncSnap.Phase = SyncPhaseRescanning
	syncSnap.RescanThrough = rescannedThrough
	syncSnap.LastNotification = time.Now().UTC()
	if syncSnap.RescanFrom == 0 && rescanBase > 0 {
		syncSnap.RescanFrom = rescanBase
	}
	if syncSnap.RescanFrom > 0 {
		pct := float64(syncSnap.RescanThrough) / float64(syncSnap.RescanFrom) * 100
		if pct > 100 {
			pct = 100
		}
		syncSnap.RescanProgressPc = pct
	}
	snap := syncSnap
	syncMu.Unlock()
	broadcastSyncSnapshot(snap)
}

// MarkRescanFinished transitions the snapshot to synced.
func MarkRescanFinished() {
	syncMu.Lock()
	syncSnap.Phase = SyncPhaseSynced
	syncSnap.RescanProgressPc = 100
	syncSnap.LastNotification = time.Now().UTC()
	snap := syncSnap
	syncMu.Unlock()
	broadcastSyncSnapshot(snap)
}

var (
	rescanRunMu  sync.Mutex
	rescanBusy   bool
	rescanFrom   int64 // begin height of the in-flight rescan
	rescanQueued bool
	rescanNext   int64 // lowest begin height requested while busy
)

// BeginRescan reserves the single rescan slot. Returns true if the caller should
// run a rescan now, false if one is already in flight (the request is coalesced;
// a lower begin height is remembered for one follow-up pass).
func BeginRescan(from int64) bool {
	if from < 0 {
		from = 0
	}
	rescanRunMu.Lock()
	defer rescanRunMu.Unlock()
	if rescanBusy {
		if from < rescanFrom && (!rescanQueued || from < rescanNext) {
			rescanQueued, rescanNext = true, from
		}
		return false
	}
	rescanBusy, rescanFrom = true, from
	return true
}

// EndRescan releases the slot. If a lower begin height was requested while the
// rescan ran, returns that height and true so the caller runs one more pass.
func EndRescan() (from int64, again bool) {
	rescanRunMu.Lock()
	defer rescanRunMu.Unlock()
	if rescanQueued {
		rescanQueued, rescanFrom = false, rescanNext
		return rescanNext, true
	}
	rescanBusy = false
	return 0, false
}

// MarkSyncDisconnected flags the snapshot as daemon-disconnected.
func MarkSyncDisconnected(reason string) {
	syncMu.Lock()
	syncSnap.DaemonConnected = false
	syncSnap.PeerCount = 0
	syncSnap.LastError = reason
	syncSnap.LastNotification = time.Now().UTC()
	snap := syncSnap
	syncMu.Unlock()
	broadcastSyncSnapshot(snap)
}

func broadcastSyncSnapshot(snap SyncSnapshot) {
	syncBus.publish(snap)
}
