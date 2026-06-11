// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	"dcrpulse/internal/rpc"
)

// WalletReady reports whether the wallet is fully synced and responsive to RPC.
// It gates first-time feature setup (privacy, lightning, Bison Relay, DCRDEX):
// creating the dedicated accounts or activating those features before dcrd and
// dcrwallet have finished syncing has caused setup failures. The returned reason
// string is suitable for a 503 response body.
func WalletReady(ctx context.Context) (bool, string) {
	// dcrd must be past initial block download.
	if rpc.DcrdClient != nil {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if info, err := rpc.DcrdClient.GetBlockChainInfo(checkCtx); err == nil && info.InitialBlockDownload {
			return false, "The Decred node is still downloading the blockchain. This feature will be available once the node finishes syncing."
		}
	}
	// dcrwallet must be loaded, answering RPC, and fully synced (not rescanning).
	status, err := FetchWalletStatus()
	if err != nil || status == nil {
		return false, "The wallet is not ready yet. Please wait until it becomes available."
	}
	if status.Status != "synced" {
		msg := status.SyncMessage
		if msg == "" {
			msg = "The wallet is still syncing."
		}
		return false, msg + " This feature will be available once your wallet is fully synced."
	}
	return true, ""
}
