// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"google.golang.org/grpc"
)

// rotateWallet is a wallet whose passphrase change succeeds and whose listed
// accounts are all encrypted; failOn makes one account's update fail.
type rotateWallet struct {
	pb.WalletServiceClient
	failOn uint32
}

func (rotateWallet) ChangePassphrase(context.Context, *pb.ChangePassphraseRequest, ...grpc.CallOption) (*pb.ChangePassphraseResponse, error) {
	return &pb.ChangePassphraseResponse{}, nil
}

func (rotateWallet) Accounts(context.Context, *pb.AccountsRequest, ...grpc.CallOption) (*pb.AccountsResponse, error) {
	return &pb.AccountsResponse{Accounts: []*pb.AccountsResponse_Account{
		{AccountNumber: 0, AccountEncrypted: true},
		{AccountNumber: 1, AccountEncrypted: true},
	}}, nil
}

func (f rotateWallet) SetAccountPassphrase(_ context.Context, in *pb.SetAccountPassphraseRequest, _ ...grpc.CallOption) (*pb.SetAccountPassphraseResponse, error) {
	if in.GetAccountNumber() == f.failOn {
		return nil, errors.New("account locked by another operation")
	}
	return &pb.SetAccountPassphraseResponse{}, nil
}

// Once the wallet passphrase has changed, dcrlnd's macaroon store is keyed to
// the old one whether or not every account followed, so the reset is armed on
// a partial change too, and the partial result is still reported.
func TestPassphraseChangeArmsTheMacaroonResetEvenWhenPartial(t *testing.T) {
	for _, tc := range []struct {
		name   string
		failOn uint32
		status int
	}{
		{"every account follows", 99, http.StatusNoContent},
		{"one account keeps the previous passphrase", 1, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			armed := false
			prevSetUp, prevArm, prevWallet := lnSetUp, armMacaroonReset, rpc.WalletGrpcClient
			lnSetUp = func() bool { return true }
			armMacaroonReset = func() error { armed = true; return nil }
			rpc.WalletGrpcClient = rotateWallet{failOn: tc.failOn}
			t.Cleanup(func() { lnSetUp, armMacaroonReset, rpc.WalletGrpcClient = prevSetUp, prevArm, prevWallet })

			rec := httptest.NewRecorder()
			ChangePassphraseHandler(rec, httptest.NewRequest("POST", "/",
				strings.NewReader(`{"oldPassphrase":"old-passphrase","newPassphrase":"new-passphrase"}`)))

			if rec.Code != tc.status {
				t.Fatalf("answer = %d %q, want %d", rec.Code, rec.Body.String(), tc.status)
			}
			if tc.status != http.StatusNoContent && !strings.Contains(rec.Body.String(), "account(s) 1 still unlock with the previous one") {
				t.Errorf("the partial change is not reported: %q", rec.Body.String())
			}
			if !armed {
				t.Error("the macaroon reset was not armed")
			}
		})
	}
}
