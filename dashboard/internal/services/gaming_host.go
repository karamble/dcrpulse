// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"os"

	"github.com/decred/dcrd/chaincfg/v3"
)

// The wallet and chain helpers the gaming bridge calls into.

func ChainParams(ctx context.Context) (*chaincfg.Params, error) { return chainParams(ctx) }

func SpendGuard() error { return spendGuard() }

func UnlockAccountForSpend(ctx context.Context, account uint32, passphrase []byte) (bool, error) {
	return unlockAccountForSpend(ctx, account, passphrase)
}

func SignTransactionForSpend(ctx context.Context, account uint32, unsignedTx, passphrase []byte) ([]byte, error) {
	return signTransactionForSpend(ctx, account, unsignedTx, passphrase)
}

func PublishSignedTransaction(ctx context.Context, signedTx []byte) (string, error) {
	return publishSignedTransaction(ctx, signedTx)
}

func BeginUnlockedOp() { beginUnlockedOp() }

func EndUnlockedOp() { endUnlockedOp() }

func WriteFileSynced(path string, data []byte, perm os.FileMode) error {
	return writeFileSynced(path, data, perm)
}
