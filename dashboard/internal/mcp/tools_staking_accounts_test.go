// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// A ticket purchase resolves its accounts from the wallet before the account
// gate can run, so a test that wants to see the gate needs a readable wallet.
// plainWalletStub has only the default account; privacyWalletStub has the mixed
// and unmixed accounts at 5 and 6. Neither can list tickets, which the tool
// descriptions computed at registration try to do.
type plainWalletStub struct{ pb.WalletServiceClient }

func (plainWalletStub) Accounts(context.Context, *pb.AccountsRequest, ...grpc.CallOption) (*pb.AccountsResponse, error) {
	return &pb.AccountsResponse{Accounts: []*pb.AccountsResponse_Account{{AccountNumber: 0, AccountName: "default"}}}, nil
}

func (plainWalletStub) GetTickets(context.Context, *pb.GetTicketsRequest, ...grpc.CallOption) (pb.WalletService_GetTicketsClient, error) {
	return nil, errors.New("no tickets in a stub wallet")
}

type privacyWalletStub struct{ plainWalletStub }

func (privacyWalletStub) Accounts(context.Context, *pb.AccountsRequest, ...grpc.CallOption) (*pb.AccountsResponse, error) {
	return &pb.AccountsResponse{Accounts: []*pb.AccountsResponse_Account{
		{AccountNumber: 0, AccountName: "default"},
		{AccountNumber: 5, AccountName: services.PrivacyMixedAccountName},
		{AccountNumber: 6, AccountName: services.PrivacyChangeAccountName},
	}}, nil
}

func withWalletStub(t *testing.T, c pb.WalletServiceClient) {
	t.Helper()
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = c
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })
}

// On a privacy wallet the purchase spends from the mixed account whatever the
// agent named, so that is the account the grant must cover. The refusal text
// names no account; the audit entry does.
func TestStakingPurchaseGatesOnTheResolvedAccounts(t *testing.T) {
	withWalletStub(t, privacyWalletStub{})

	const agentID = "staking-resolved"
	grants.set(agentID, GrantSpec{Accounts: []uint32{0}, PerTxAtoms: 1e8, DailyAtoms: 1e8}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })
	cs := connectTo(t, testAgent(agentID, "staking", map[string]bool{"staking": true}))

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "staking_purchase", Arguments: map[string]any{
			"account": 0, "numTickets": 1, "vspHost": "https://vsp.example", "vspPubkey": "k",
		},
	})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(res), "not covered by the agent's spend grant") {
		t.Fatalf("account 0 is granted, yet the purchase would spend from the mixed account: %q", resultText(res))
	}
	last := AuditLog(1)
	if len(last) != 1 || last[0].Result != "denied" || last[0].Account != 5 || !strings.Contains(last[0].Detail, "not covered") {
		t.Fatalf("audit entry %+v; want the grant gate refusing account 5, the mixed account", last)
	}
}
