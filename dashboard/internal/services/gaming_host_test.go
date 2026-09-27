// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dcrpulse/internal/gamingcore"
	"dcrpulse/internal/rpc"
)

type signingMetadata string

func (m signingMetadata) Get(_ string, out any) (bool, error) {
	if m == "" {
		return false, nil
	}
	return true, json.Unmarshal([]byte(m), out)
}

func TestGamingSigningMetadataFailsClosed(t *testing.T) {
	for _, input := range []string{"", "null", "true", "\"false\"", "{", "0"} {
		t.Run(input, func(t *testing.T) {
			if err := requireGamingSigningMetadata(signingMetadata(input)); err == nil {
				t.Fatal("unproven signing wallet accepted")
			}
		})
	}
	if err := requireGamingSigningMetadata(signingMetadata("false")); err != nil {
		t.Fatal(err)
	}
}

// The mixer, Lightning, DEX and imported accounts are bound to by name, in any
// case: a game bound to one would spend funds another daemon relies on.
func TestTheDashboardReservesItsDaemonAccountsFromGames(t *testing.T) {
	w := GamingHost(nil).Wallet
	for _, name := range []string{"lightning", "dex", "mixed", "unmixed", "imported", "  Lightning "} {
		if !w.ReservedAccount(name) {
			t.Errorf("%q is not reserved", name)
		}
	}
	if w.ReservedAccount("games") {
		t.Error("an ordinary account is reserved")
	}
}

func TestAnOperatorWithoutAPasswordCheckIsUnprotected(t *testing.T) {
	if GamingHost(nil).Operator.Protected() {
		t.Fatal("no password check read as protected")
	}
	if !GamingHost(func() bool { return true }).Operator.Protected() {
		t.Fatal("the App Password was not consulted")
	}
}

type fakeGamingWallet struct {
	pb.WalletServiceClient
	tx    *pb.GetTransactionResponse
	txErr error
	resps []*pb.GetTransactionsResponse
	start int32
}

func (f *fakeGamingWallet) GetTransaction(context.Context, *pb.GetTransactionRequest, ...grpc.CallOption) (*pb.GetTransactionResponse, error) {
	return f.tx, f.txErr
}

func (f *fakeGamingWallet) GetTransactions(_ context.Context, req *pb.GetTransactionsRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.GetTransactionsResponse], error) {
	f.start = req.StartingBlockHeight
	return &fakeListTxStream{resps: f.resps}, nil
}

func withGamingWalletClient(t *testing.T, c pb.WalletServiceClient) {
	t.Helper()
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = c
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })
}

func TestGamingWalletReportsItsOwnTransactions(t *testing.T) {
	ctx := context.Background()
	w := GamingHost(nil).Wallet
	var hash chainhash.Hash
	hash[0] = 7

	withGamingWalletClient(t, nil)
	if _, ok, err := w.Transaction(ctx, hash); ok || err != nil {
		t.Fatalf("no wallet = %v, %v; want unknown", ok, err)
	}
	f := &fakeGamingWallet{txErr: status.Error(codes.NotFound, "no such transaction")}
	withGamingWalletClient(t, f)
	if _, ok, err := w.Transaction(ctx, hash); ok || err != nil {
		t.Fatalf("a transaction the wallet never saw = %v, %v", ok, err)
	}
	f.txErr = errors.New("wallet down")
	if _, _, err := w.Transaction(ctx, hash); err == nil {
		t.Fatal("a failed lookup read as an answer")
	}
	f.txErr = nil
	f.tx = &pb.GetTransactionResponse{Transaction: &pb.TransactionDetails{Transaction: []byte{1, 2}}, Confirmations: 3, BlockHash: bytes.Repeat([]byte{0xbb}, 32)}
	tx, ok, err := w.Transaction(ctx, hash)
	if !ok || err != nil || tx.Confirmations != 3 || !bytes.Equal(tx.Raw, []byte{1, 2}) || tx.BlockHash == nil || tx.BlockHash[0] != 0xbb {
		t.Fatalf("mined = %+v, %v, %v", tx, ok, err)
	}
	f.tx = &pb.GetTransactionResponse{Transaction: &pb.TransactionDetails{Transaction: []byte{1}}}
	if tx, ok, _ = w.Transaction(ctx, hash); !ok || tx.BlockHash != nil {
		t.Fatalf("an unmined transaction = %+v, %v", tx, ok)
	}
	f.tx = &pb.GetTransactionResponse{}
	if _, ok, _ = w.Transaction(ctx, hash); ok {
		t.Fatal("an empty answer read as a transaction")
	}
}

func TestGamingWalletWalksMinedBlocksInOrder(t *testing.T) {
	ctx := context.Background()
	f := &fakeGamingWallet{resps: []*pb.GetTransactionsResponse{
		{UnminedTransactions: []*pb.TransactionDetails{{Transaction: []byte{9}}}},
		{MinedTransactions: &pb.BlockDetails{Height: 10, Transactions: []*pb.TransactionDetails{{Transaction: []byte{1}}, {Transaction: []byte{2}}}}},
		{MinedTransactions: &pb.BlockDetails{Height: 11, Transactions: []*pb.TransactionDetails{{Transaction: []byte{3}}}}},
	}}
	withGamingWalletClient(t, f)
	w := GamingHost(nil).Wallet
	var heights []int32
	var txs [][]byte
	err := w.MinedTransactions(ctx, 10, func(h int32, b [][]byte) error {
		heights = append(heights, h)
		txs = append(txs, b...)
		return nil
	})
	if err != nil || f.start != 10 || !slices.Equal(heights, []int32{10, 11}) || len(txs) != 3 || txs[2][0] != 3 {
		t.Fatalf("walk = %v %v %v, from %d", heights, txs, err, f.start)
	}
	stop := errors.New("stop")
	calls := 0
	err = w.MinedTransactions(ctx, 0, func(int32, [][]byte) error { calls++; return stop })
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("a failing block = %v after %d calls", err, calls)
	}
}

func TestGamingRelayTellsARefusalFromAnUnknownSend(t *testing.T) {
	ctx := context.Background()
	var gotID string
	var answer error
	prev := gamingBRSend
	gamingBRSend = func(_ context.Context, id rpc.ShortIDHex, _ string, _ int) error {
		gotID = id.String()
		return answer
	}
	t.Cleanup(func() { gamingBRSend = prev })
	r := GamingHost(nil).Relay
	var gcid [32]byte
	gcid[0] = 0xab

	answer = &rpc.BrclientdStatusError{Path: "/gc", Code: 503, Body: "BR client not yet running"}
	if err := r.SendGroupMessage(ctx, gcid, "x"); !errors.Is(err, gamingcore.ErrNotSent) {
		t.Fatalf("a refused send = %v, want not sent", err)
	}
	if gotID != hex.EncodeToString(gcid[:]) {
		t.Fatalf("sent to %q", gotID)
	}
	answer = errors.New("connection reset")
	if err := r.SendGroupMessage(ctx, gcid, "x"); err == nil || errors.Is(err, gamingcore.ErrNotSent) {
		t.Fatalf("an unknown outcome = %v, want an error that is not a refusal", err)
	}
	answer = nil
	if err := r.SendGroupMessage(ctx, gcid, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestGamingRelayReadsIdentityAndHistory(t *testing.T) {
	ctx := context.Background()
	uid := bytes.Repeat([]byte{0x22}, 32)
	prevID, prevHist := gamingBRIdentity, gamingBRHistory
	t.Cleanup(func() { gamingBRIdentity, gamingBRHistory = prevID, prevHist })
	gamingBRIdentity = func(context.Context) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"identity": uid, "nick": "me"})
	}
	gamingBRHistory = func(context.Context, rpc.ShortIDHex, int, int) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"entries": []map[string]string{{"from": "me", "message": "a"}, {"from": "peer", "message": "b"}}})
	}
	r := GamingHost(nil).Relay
	got, nick, err := r.Identity(ctx)
	if err != nil || nick != "me" || got != [32]byte(uid) {
		t.Fatalf("identity = %x %q %v", got, nick, err)
	}
	entries, err := r.GroupHistory(ctx, [32]byte{}, 0, 500)
	if err != nil || len(entries) != 2 || entries[0] != (gamingcore.GroupEntry{From: "me", Message: "a"}) || entries[1].From != "peer" {
		t.Fatalf("history = %+v, %v", entries, err)
	}
	gamingBRIdentity = func(context.Context) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"identity": uid[:16], "nick": "me"})
	}
	if got, _, _ = r.Identity(ctx); got != ([32]byte{}) {
		t.Fatalf("a short identity was taken: %x", got)
	}
}
