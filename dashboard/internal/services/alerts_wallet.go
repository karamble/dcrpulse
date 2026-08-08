// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"time"

	"dcrpulse/internal/alerts"
	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v4"
)

// StartTransactionWatcher keeps a TransactionNotifications stream open and
// emits wallet_tx_received for each confirmed pure receive. Supervised like
// superviseRpcSync: parks while a wallet switch or restore discovery is in
// progress, reconnects with backoff when the stream dies.
func StartTransactionWatcher(ctx context.Context) {
	go func() {
		backoff := 5 * time.Second
		for {
			if ctx.Err() != nil {
				return
			}
			if SyncPaused() || RestoreDiscoveryActive() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}
			client := rpc.WalletGrpcClient
			if client == nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				continue
			}
			err := watchWalletTransactions(ctx, client)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				alrtLog.Warnf("transaction watcher: %v (retrying in %s)", err, backoff)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 60*time.Second {
				backoff *= 2
			}
		}
	}()
}

func watchWalletTransactions(ctx context.Context, client pb.WalletServiceClient) error {
	stream, err := client.TransactionNotifications(ctx, &pb.TransactionNotificationsRequest{})
	if err != nil {
		return err
	}
	for {
		resp, err := stream.Recv()
		if err != nil {
			return err
		}
		if SyncPaused() || RestoreDiscoveryActive() {
			continue
		}
		for _, blk := range resp.GetAttachedBlocks() {
			// Blocks attached far below the synced tip are catch-up replay
			// (rescan, wallet reopen after downtime), not fresh receives.
			if !blockNearTip(blk.GetHeight()) {
				continue
			}
			for _, tx := range blk.GetTransactions() {
				emitReceiveAlert(tx)
			}
		}
	}
}

func blockNearTip(height int32) bool {
	snap := GetNodeSyncSnapshot()
	if snap.Status != "running" || snap.Blocks <= 0 {
		return false
	}
	return int64(height) >= snap.Blocks-6
}

func emitReceiveAlert(tx *pb.TransactionDetails) {
	// Pure receives only: any debit means the wallet initiated the spend and
	// the credits are change.
	if tx.GetTransactionType() != pb.TransactionDetails_REGULAR {
		return
	}
	if len(tx.GetDebits()) != 0 || len(tx.GetCredits()) == 0 {
		return
	}
	var atoms int64
	for _, c := range tx.GetCredits() {
		atoms += c.GetAmount()
	}
	if atoms <= 0 {
		return
	}
	txid := ""
	if h, err := chainhash.NewHash(tx.GetHash()); err == nil {
		txid = h.String()
	}
	alerts.Emit("wallet_tx_received", fmt.Sprintf("Received %s", dcrutil.Amount(atoms)), txid)
}
