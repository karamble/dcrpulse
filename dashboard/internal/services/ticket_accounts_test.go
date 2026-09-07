// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"google.golang.org/grpc"
)

// A ticket purchase must spend the accounts its caller resolved and checked,
// never a second reading of the wallet. These tests present a privacy wallet
// through both clients the old path read (JSON-RPC getbalance for names, gRPC
// Accounts for numbers), so any re-derivation shows up as accounts 5 and 6
// turning up uninvited.

// privacyWallet has the mixed and unmixed accounts at 5 and 6, answers what a
// purchase needs, and records what it was asked to unlock and to buy.
type privacyWallet struct {
	pb.WalletServiceClient
	accountsErr error

	mu       sync.Mutex
	unlocked []uint32
	purchase *pb.PurchaseTicketsRequest
}

func (w *privacyWallet) Accounts(context.Context, *pb.AccountsRequest, ...grpc.CallOption) (*pb.AccountsResponse, error) {
	if w.accountsErr != nil {
		return nil, w.accountsErr
	}
	return &pb.AccountsResponse{Accounts: []*pb.AccountsResponse_Account{
		{AccountNumber: 0, AccountName: "default", AccountEncrypted: true},
		{AccountNumber: 5, AccountName: PrivacyMixedAccountName, AccountEncrypted: true},
		{AccountNumber: 6, AccountName: PrivacyChangeAccountName, AccountEncrypted: true},
	}}, nil
}

func (w *privacyWallet) UnlockAccount(_ context.Context, req *pb.UnlockAccountRequest, _ ...grpc.CallOption) (*pb.UnlockAccountResponse, error) {
	w.mu.Lock()
	w.unlocked = append(w.unlocked, req.AccountNumber)
	w.mu.Unlock()
	return &pb.UnlockAccountResponse{}, nil
}

func (w *privacyWallet) LockAccount(context.Context, *pb.LockAccountRequest, ...grpc.CallOption) (*pb.LockAccountResponse, error) {
	return &pb.LockAccountResponse{}, nil
}

func (w *privacyWallet) PurchaseTickets(_ context.Context, req *pb.PurchaseTicketsRequest, _ ...grpc.CallOption) (*pb.PurchaseTicketsResponse, error) {
	w.mu.Lock()
	w.purchase = req
	w.mu.Unlock()
	return &pb.PurchaseTicketsResponse{}, nil
}

func (w *privacyWallet) request(t *testing.T) *pb.PurchaseTicketsRequest {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.purchase == nil {
		t.Fatal("no purchase reached the wallet")
	}
	return w.purchase
}

// privacyBalances is getbalance on the same wallet: names only, as dcrwallet
// returns them.
func privacyBalances(w http.ResponseWriter, id json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"result":{"balances":[{"accountname":"default"},{"accountname":%q},{"accountname":%q}]},"error":null,"id":%s}`,
		PrivacyMixedAccountName, PrivacyChangeAccountName, id)
}

func TestPurchaseSpendsExactlyTheResolvedAccounts(t *testing.T) {
	w := &privacyWallet{}
	withWallet(t, w)
	walletRPCServer(t, privacyBalances)
	// The old path can see the privacy accounts in this process, so a
	// re-derivation inside the purchase would be visible, not silently "plain".
	if m, ok := TicketMixingParams(context.Background()); !ok || m.Mixed != 5 || m.Change != 6 {
		t.Fatalf("the fakes do not present a privacy wallet: %+v %v", m, ok)
	}

	if _, err := PurchaseTickets(context.Background(), TicketAccounts{Source: 1, Change: 2}, 1, "vsp.test", "k", []byte("pass")); err != nil {
		t.Fatalf("PurchaseTickets() = %v", err)
	}
	req := w.request(t)
	if req.Account != 1 || req.ChangeAccount != 2 || req.EnableMixing {
		t.Fatalf("spent from %d with change to %d, mixing %v; the caller resolved 1 and 2, plain", req.Account, req.ChangeAccount, req.EnableMixing)
	}
}

func TestMixedPurchaseDerivesEveryFieldFromSource(t *testing.T) {
	w := &privacyWallet{}
	withWallet(t, w)

	if _, err := PurchaseTickets(context.Background(), TicketAccounts{Source: 5, Change: 6, Mixed: true}, 1, "vsp.test", "k", []byte("pass")); err != nil {
		t.Fatalf("PurchaseTickets() = %v", err)
	}
	req := w.request(t)
	if !req.EnableMixing || req.Account != 5 || req.MixedAccount != 5 || req.MixedSplitAccount != 5 || req.ChangeAccount != 6 {
		t.Fatalf("mixed purchase request: account %d mixed %d split %d change %d mixing %v; every mixed field must be the source",
			req.Account, req.MixedAccount, req.MixedSplitAccount, req.ChangeAccount, req.EnableMixing)
	}
}

func TestResolveTicketAccountsReadsTheWalletOnce(t *testing.T) {
	withWallet(t, &privacyWallet{})
	got, err := ResolveTicketAccounts(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != (TicketAccounts{Source: 5, Change: 6, Mixed: true}) {
		t.Fatalf("privacy wallet resolved to %+v, want the mixed pair", got)
	}

	withWallet(t, &stubWallet{}) // no privacy accounts
	got, err = ResolveTicketAccounts(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != (TicketAccounts{Source: 1, Change: 2}) {
		t.Fatalf("plain wallet resolved to %+v, want the caller's accounts", got)
	}

	// A wallet that cannot be read is an error, not a plain purchase from the
	// caller's account on a wallet that may well be a privacy one.
	withWallet(t, &privacyWallet{accountsErr: errors.New("wallet is syncing")})
	if _, err := ResolveTicketAccounts(context.Background(), 1, 2); err == nil {
		t.Fatal("an unreadable wallet resolved to a plain purchase")
	}
}

// The worker verifies the passphrase before detaching. It must do so against
// the account the purchase then unlocks, not the one the caller named.
func TestPurchaseWorkerVerifiesTheAccountItUnlocks(t *testing.T) {
	w := &privacyWallet{}
	withWallet(t, w)
	events, stop := SubscribePurchaseEvents()
	defer stop()

	if err := StartPurchaseWorker(TicketAccounts{Source: 5, Change: 6, Mixed: true}, 1, "vsp.test", "k", []byte("pass")); err != nil {
		t.Fatalf("StartPurchaseWorker() = %v", err)
	}
	// Wait for the detached run to finish, or its exit would race the stub
	// and leave the single-flight flag set for the next test.
	deadline := time.After(5 * time.Second)
	for done := false; !done; {
		select {
		case ev := <-events:
			done = ev.Kind == "done" || ev.Kind == "error"
		case <-deadline:
			t.Fatal("the purchase worker did not finish")
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.unlocked) == 0 || w.unlocked[0] != 5 {
		t.Fatalf("the passphrase was verified against accounts %v; the purchase unlocks 5", w.unlocked)
	}
}
