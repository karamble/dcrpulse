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
	SyncPhaseUnknown            SyncPhase = "unknown"
	SyncPhaseUnsynced           SyncPhase = "unsynced"
	SyncPhaseFetchingCfilters   SyncPhase = "fetching_cfilters"
	SyncPhaseFetchingHeaders    SyncPhase = "fetching_headers"
	SyncPhaseDiscoverAddresses  SyncPhase = "discovering_addresses"
	SyncPhaseRescanning         SyncPhase = "rescanning"
	SyncPhaseSynced             SyncPhase = "synced"
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
	syncMu       sync.RWMutex
	syncSnap     = SyncSnapshot{Phase: SyncPhaseUnknown}
	syncSubsMu   sync.Mutex
	syncSubs     []chan SyncSnapshot
)

// GetSyncSnapshot returns a copy of the current snapshot.
func GetSyncSnapshot() SyncSnapshot {
	syncMu.RLock()
	defer syncMu.RUnlock()
	return syncSnap
}

// SubscribeSyncEvents returns a channel that receives every snapshot update and a cleanup func.
func SubscribeSyncEvents() (<-chan SyncSnapshot, func()) {
	ch := make(chan SyncSnapshot, 8)
	syncSubsMu.Lock()
	syncSubs = append(syncSubs, ch)
	syncSubsMu.Unlock()
	return ch, func() {
		syncSubsMu.Lock()
		defer syncSubsMu.Unlock()
		for i, sub := range syncSubs {
			if sub == ch {
				syncSubs = append(syncSubs[:i], syncSubs[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

// ApplyRpcSyncNotification updates the snapshot from one RpcSync response.
func ApplyRpcSyncNotification(resp *pb.RpcSyncResponse) {
	if resp == nil {
		return
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
		if syncSnap.RescanFrom == 0 && rpc.DcrdClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if h, err := rpc.DcrdClient.GetBlockCount(ctx); err == nil {
				syncSnap.RescanFrom = h
			}
			cancel()
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
	syncMu.Lock()
	syncSnap.Phase = SyncPhaseRescanning
	syncSnap.RescanThrough = rescannedThrough
	syncSnap.LastNotification = time.Now().UTC()
	if syncSnap.RescanFrom == 0 && rpc.DcrdClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if h, err := rpc.DcrdClient.GetBlockCount(ctx); err == nil {
			syncSnap.RescanFrom = h
		}
		cancel()
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
	syncSubsMu.Lock()
	for _, sub := range syncSubs {
		select {
		case sub <- snap:
		default:
		}
	}
	syncSubsMu.Unlock()
}
