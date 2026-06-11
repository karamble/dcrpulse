// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"sync"
	"sync/atomic"
)

// switchInProgress parks the RpcSync supervisor while a wallet switch tears the
// dcrwallet process down and brings a different wallet up. dcrwallet permits one
// syncer at a time; without this gate the supervisor would race to grab the slot
// against the relaunching daemon and the post-switch OpenWallet. Mirrors the
// restoreDiscoveryActive gate used during a seed restore.
var (
	switchInProgress atomic.Bool

	syncCancelMu sync.Mutex
	syncCancelFn context.CancelFunc
)

// PauseSync marks a wallet switch as owning the RpcSync slot and cancels any
// in-flight sync stream so the supervisor parks promptly.
func PauseSync() {
	switchInProgress.Store(true)
	syncCancelMu.Lock()
	if syncCancelFn != nil {
		syncCancelFn()
	}
	syncCancelMu.Unlock()
}

// ResumeSync releases the slot so the supervisor reconnects RpcSync.
func ResumeSync() { switchInProgress.Store(false) }

// SyncPaused reports whether a wallet switch currently owns the slot.
func SyncPaused() bool { return switchInProgress.Load() }

// RegisterSyncCancel stores the cancel for the current RpcSync attempt so a
// switch can interrupt the otherwise-blocking stream.
func RegisterSyncCancel(cancel context.CancelFunc) {
	syncCancelMu.Lock()
	syncCancelFn = cancel
	syncCancelMu.Unlock()
}
