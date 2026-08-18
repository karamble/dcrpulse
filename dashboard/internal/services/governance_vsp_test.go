// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"google.golang.org/grpc"
)

// fakeVSPWallet drives the governance VSP sync without a wallet: it records
// every unlock, sync and lock in arrival order, so the tests can pin the one
// property the whole fix exists for - accounts open only around the vspd
// calls, and always close again.
type fakeVSPWallet struct {
	pb.WalletServiceClient

	syncErr map[string]error

	mu     sync.Mutex
	events []string
	fees   []uint32
	change []uint32
}

func (f *fakeVSPWallet) record(ev string) {
	f.mu.Lock()
	f.events = append(f.events, ev)
	f.mu.Unlock()
}

func (f *fakeVSPWallet) Accounts(ctx context.Context, in *pb.AccountsRequest, _ ...grpc.CallOption) (*pb.AccountsResponse, error) {
	return &pb.AccountsResponse{Accounts: []*pb.AccountsResponse_Account{
		{AccountName: "default", AccountNumber: 0, AccountEncrypted: true},
		{AccountName: "voting", AccountNumber: 1, AccountEncrypted: true},
		{AccountName: "imported", AccountNumber: 1<<31 - 1},
	}}, nil
}

func (f *fakeVSPWallet) UnlockAccount(ctx context.Context, in *pb.UnlockAccountRequest, _ ...grpc.CallOption) (*pb.UnlockAccountResponse, error) {
	if string(in.GetPassphrase()) != "hunter2" {
		return nil, fmt.Errorf("invalid passphrase")
	}
	f.record(fmt.Sprintf("unlock:%d", in.GetAccountNumber()))
	return &pb.UnlockAccountResponse{}, nil
}

func (f *fakeVSPWallet) LockAccount(ctx context.Context, in *pb.LockAccountRequest, _ ...grpc.CallOption) (*pb.LockAccountResponse, error) {
	f.record(fmt.Sprintf("lock:%d", in.GetAccountNumber()))
	return &pb.LockAccountResponse{}, nil
}

func (f *fakeVSPWallet) GetTrackedVSPTickets(ctx context.Context, in *pb.GetTrackedVSPTicketsRequest, _ ...grpc.CallOption) (*pb.GetTrackedVSPTicketsResponse, error) {
	return &pb.GetTrackedVSPTicketsResponse{}, nil
}

func (f *fakeVSPWallet) SetVspdVoteChoices(ctx context.Context, in *pb.SetVspdVoteChoicesRequest, _ ...grpc.CallOption) (*pb.SetVspdVoteChoicesResponse, error) {
	f.record("sync:" + in.GetVspHost())
	f.mu.Lock()
	f.fees = append(f.fees, in.GetFeeAccount())
	f.change = append(f.change, in.GetChangeAccount())
	f.mu.Unlock()
	if err, ok := f.syncErr[in.GetVspHost()]; ok {
		return nil, err
	}
	return &pb.SetVspdVoteChoicesResponse{}, nil
}

func withFakeVSPWallet(t *testing.T, f *fakeVSPWallet) {
	t.Helper()
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = f
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })
}

// classify splits the recorded events into phases; -1 means never seen.
func phaseIndexes(events []string, prefix string) (first, last int) {
	first, last = -1, -1
	for i, ev := range events {
		if strings.HasPrefix(ev, prefix) {
			if first == -1 {
				first = i
			}
			last = i
		}
	}
	return
}

// TestPushVoteChoicesUnlocksSyncsRelocks pins the D-006 shape: every unlock
// precedes every vspd call, every lock follows every vspd call, and the host
// reaches the wire VERBATIM (the D-005 shape - dcrwallet matches it
// exact-string against the ticket's stored purchase host).
func TestPushVoteChoicesUnlocksSyncsRelocks(t *testing.T) {
	f := &fakeVSPWallet{}
	withFakeVSPWallet(t, f)

	// A host any re-normalization would still "accept" but the bare-host
	// regression would mangle: scheme + port.
	hostA := "https://vsp-a.example.org:8443"
	hostB := "https://vsp-b.example.org"
	var failed []string
	pushVoteChoicesToVSPs(context.Background(),
		[]string{hostA, hostB},
		map[string]string{hostA: "pkA", hostB: "pkB"},
		3, 4, []byte("hunter2"),
		func(host, msg string) { failed = append(failed, host+": "+msg) })

	if len(failed) != 0 {
		t.Fatalf("unexpected failures: %v", failed)
	}
	uFirst, uLast := phaseIndexes(f.events, "unlock:")
	sFirst, sLast := phaseIndexes(f.events, "sync:")
	lFirst, _ := phaseIndexes(f.events, "lock:")
	if uFirst == -1 || sFirst == -1 || lFirst == -1 {
		t.Fatalf("missing a phase entirely: %v", f.events)
	}
	if uLast > sFirst {
		// Mutation catch: syncing before the accounts are open (or a
		// wallet-wide UnlockWallet, which records no unlock at all).
		t.Fatalf("a sync ran before unlocking finished: %v", f.events)
	}
	if sLast > lFirst {
		t.Fatalf("an account locked while syncing was still running: %v", f.events)
	}
	var synced []string
	for _, ev := range f.events {
		if strings.HasPrefix(ev, "sync:") {
			synced = append(synced, strings.TrimPrefix(ev, "sync:"))
		}
	}
	if len(synced) != 2 || synced[0] != hostA || synced[1] != hostB {
		// Mutation catch: any reconstruction of the host string (the D-005
		// bare-host bug) breaks the verbatim match.
		t.Fatalf("hosts did not reach the wire verbatim: %v", synced)
	}
	for i := range f.fees {
		if f.fees[i] != 3 || f.change[i] != 4 {
			t.Fatalf("fee/change accounts not passed through: fee=%v change=%v", f.fees, f.change)
		}
	}
}

// TestPushVoteChoicesRelocksOnSyncFailure pins the defer: a failing vspd call
// still re-locks every account this window opened, and the failure is
// reported per host.
func TestPushVoteChoicesRelocksOnSyncFailure(t *testing.T) {
	host := "https://vsp-a.example.org"
	f := &fakeVSPWallet{syncErr: map[string]error{host: fmt.Errorf("vspd said no")}}
	withFakeVSPWallet(t, f)

	var failed []string
	pushVoteChoicesToVSPs(context.Background(),
		[]string{host}, map[string]string{host: "pkA"},
		0, 0, []byte("hunter2"),
		func(h, msg string) { failed = append(failed, h) })

	if len(failed) != 1 || failed[0] != host {
		t.Fatalf("failure not reported per host: %v", failed)
	}
	if _, last := phaseIndexes(f.events, "lock:"); last == -1 {
		// Mutation catch: dropping the relock defer leaves keys usable.
		t.Fatalf("accounts were not re-locked after the failure: %v", f.events)
	}
}

// TestVerifyGovernancePassphraseRestoresLockState pins the pre-write check:
// a wrong passphrase is refused with a message the handlers map to 401, and a
// right one leaves the account exactly as found (unlocked, then locked again).
func TestVerifyGovernancePassphraseRestoresLockState(t *testing.T) {
	f := &fakeVSPWallet{}
	withFakeVSPWallet(t, f)

	if err := verifyGovernancePassphrase(context.Background(), []byte("wrong")); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "passphrase") {
		t.Fatalf("wrong passphrase not refused as a passphrase error: %v", err)
	}
	f.events = nil
	if err := verifyGovernancePassphrase(context.Background(), []byte("hunter2")); err != nil {
		t.Fatalf("right passphrase refused: %v", err)
	}
	if len(f.events) != 2 || f.events[0] != "unlock:0" || f.events[1] != "lock:0" {
		t.Fatalf("lock state not restored: %v", f.events)
	}
}

func TestVSPHostsFromTickets(t *testing.T) {
	tickets := []types.TicketRecord{
		{Status: "LIVE", VSPHost: "https://vsp-b.example.org"},
		{Status: "IMMATURE", VSPHost: "https://vsp-a.example.org:8443"},
		{Status: "UNMINED", VSPHost: "https://vsp-b.example.org"}, // duplicate host
		{Status: "LIVE", VSPHost: ""},                            // solo ticket - no host
		{Status: "VOTED", VSPHost: "https://vsp-c.example.org"},  // spent - not votable
		{Status: "EXPIRED", VSPHost: "https://vsp-d.example.org"},
	}
	got := vspHostsFromTickets(tickets)
	want := []string{"https://vsp-a.example.org:8443", "https://vsp-b.example.org"}
	if len(got) != len(want) {
		t.Fatalf("hosts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hosts = %v, want %v", got, want)
		}
	}
}

func TestPubkeyForVSPHost(t *testing.T) {
	used := map[string]config.VSPMetadata{
		"vsp-a.example.org:8443": {Pubkey: "pkA"}, // bare, Decrediton-compatible storage
		"Vsp-B.example.org":      {Pubkey: "pkB"}, // odd case
	}
	cases := []struct{ host, want string }{
		{"https://vsp-a.example.org:8443", "pkA"},
		{"https://vsp-b.example.org", "pkB"},
		{"https://vsp-b.example.org/", "pkB"}, // trailing slash tolerated in lookup only
		{"https://unknown.example.org", ""},
	}
	for _, c := range cases {
		if got := pubkeyForVSPHost(used, c.host); got != c.want {
			t.Fatalf("pubkeyForVSPHost(%q) = %q, want %q", c.host, got, c.want)
		}
	}
}

func TestFilterSoloTicketNoise(t *testing.T) {
	cases := []struct{ in, want string }{
		// Pure solo-ticket noise: swallowed like dcrwallet's own JSON-RPC path.
		{"rpc error: code = Unknown desc = ForUnspentUnexpiredTickets failed. Error: no VSP info for ticket 1a2b", ""},
		{"ForUnspentUnexpiredTickets failed. Error: no VSP info for ticket 1a2b\nno VSP info for ticket 3c4d", ""},
		// Real failure buried with noise: the real part survives.
		{"ForUnspentUnexpiredTickets failed. Error: no VSP info for ticket 1a2b\nvspd: connection refused", "vspd: connection refused"},
		// Plain failure: untouched.
		{"account with unique passphrase is locked", "account with unique passphrase is locked"},
	}
	for _, c := range cases {
		if got := filterSoloTicketNoise(c.in); got != c.want {
			t.Fatalf("filterSoloTicketNoise(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
