// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
	"dcrpulse/internal/utils"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

// Send-all sweeps the whole balance through the change destination, so a second
// recipient and the stated amount cannot both be honoured. Reinterpreting the
// request as "everything to the first address" is the failure this refuses.
// The fake embeds the interface without implementing ConstructTransaction, so
// reaching the RPC panics: the guard is proven by the call returning cleanly.
type sendAllWallet struct{ pb.WalletServiceClient }

func TestConstructTransactionRejectsSendAllWithManyOutputs(t *testing.T) {
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = &sendAllWallet{}
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })

	many := []types.TxRecipient{
		{Address: "DsAlpha", AmountAtoms: 100000000},
		{Address: "DsBeta", AmountAtoms: 0},
	}
	_, err := ConstructTransaction(context.Background(), 0, many, true)
	if err == nil {
		t.Fatal("send-all with two recipients was accepted")
	}
	if !errors.Is(err, utils.ErrSendAllSingleRecipient) {
		t.Fatalf("refusal = %v, want the shared sentinel", err)
	}

	// The regression pin: one recipient must still get past the guard, which is
	// observable as the panic the fake raises when the RPC is reached.
	defer func() {
		if recover() == nil {
			t.Fatal("a single-recipient send-all never reached the RPC")
		}
	}()
	_, _ = ConstructTransaction(context.Background(), 0, many[:1], true)
}
