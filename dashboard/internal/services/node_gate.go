// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "dcrpulse/internal/rpc"

// NodeWalletGate is how dcrd's polled state should affect a wallet request.
type NodeWalletGate int

const (
	GateOK          NodeWalletGate = iota // proceed
	GateIBD                               // dcrd is still downloading the chain
	GateUnreachable                       // dcrd is starting, upgrading, or down
)

// msgNodeSyncing is what the wallet routes answer during initial block download.
const msgNodeSyncing = "The Decred node is still downloading the blockchain. Your wallet will be available once the node finishes syncing."

// msgNodeStarting stands in when the poller has no startup detail to offer.
const msgNodeStarting = "The Decred node is starting up. Your wallet will be available once it is ready."

// NodeWalletGateState reads the snapshot the nodesync poller already keeps
// instead of asking dcrd on every request. It fails OPEN: it exists to turn a
// confusing wallet error into a clear one, not to gate anything, so every state
// today's callers proceed through still proceeds - a nil client (no dcrd
// credentials, so the poller never starts), the empty snapshot before the first
// poll, and "error" (dcrd reachable but refusing, which was never unreachable).
func NodeWalletGateState() (NodeWalletGate, string) {
	if rpc.DcrdClient == nil {
		return GateOK, ""
	}
	snap := GetNodeSyncSnapshot()
	switch snap.Status {
	case "syncing", "connecting":
		// The only two statuses syncFromChainInfo produces while
		// InitialBlockDownload is set.
		return GateIBD, msgNodeSyncing
	case "starting", "upgrading":
		// StartupNote is the same DaemonStartupHint message the per-request
		// check used to compute, already known to the poller.
		if snap.StartupNote != "" {
			return GateUnreachable, snap.StartupNote
		}
		return GateUnreachable, msgNodeStarting
	}
	return GateOK, ""
}

// SetNodeSyncSnapshotForTest installs a snapshot and returns a func that
// restores the previous one. A shipping function so tests in other packages can
// drive the gate; rpc.SwapDcrlndClients exists for the same reason.
func SetNodeSyncSnapshotForTest(s NodeSyncSnapshot) func() {
	nodeSyncMu.Lock()
	prev := nodeSyncSnap
	nodeSyncSnap = s
	nodeSyncMu.Unlock()
	return func() {
		nodeSyncMu.Lock()
		nodeSyncSnap = prev
		nodeSyncMu.Unlock()
	}
}
