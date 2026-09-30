// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"google.golang.org/grpc"

	"dcrpulse/internal/rpc"
)

// rejectingWallet refuses every publish with dcrd's duplicate-inputs rejection.
type rejectingWallet struct {
	pb.WalletServiceClient
}

func (rejectingWallet) PublishTransaction(context.Context, *pb.PublishTransactionRequest, ...grpc.CallOption) (*pb.PublishTransactionResponse, error) {
	return nil, errors.New("rpc error: code = Unknown desc = transaction contains duplicate inputs")
}

// MSIG-17: dcrwallet already treats a genuinely known transaction as success,
// so a rejection that mentions "duplicate" must reach the user as a rejection.
func TestBroadcastReportsARejectionMentioningDuplicates(t *testing.T) {
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = rejectingWallet{}
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })

	tx := wire.NewMsgTx()
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{1}, 0, 0), 1e8, []byte{0x51}))
	tx.AddTxOut(wire.NewTxOut(5e7, []byte{0x51}))
	raw, err := tx.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	BroadcastSignedTransactionHandler(rec, httptest.NewRequest(http.MethodPost, "/api/wallet/broadcast-signed",
		strings.NewReader(`{"signedTx":"`+hex.EncodeToString(raw)+`"}`)))
	if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "duplicate inputs") {
		t.Fatalf("got %d %q, want the rejection passed through", rec.Code, rec.Body.String())
	}
}
