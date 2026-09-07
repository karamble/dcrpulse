// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"google.golang.org/grpc"
)

// ticketStream answers a canned list of tickets, then EOF.
type ticketStream struct {
	grpc.ServerStreamingClient[pb.GetTicketsResponse]
	items []*pb.GetTicketsResponse
}

func (s *ticketStream) Recv() (*pb.GetTicketsResponse, error) {
	if len(s.items) == 0 {
		return nil, io.EOF
	}
	it := s.items[0]
	s.items = s.items[1:]
	return it, nil
}

// ticketWallet records every listing request and counts the fee-status calls
// a full listing makes and a bounded one must not.
type ticketWallet struct {
	pb.WalletServiceClient
	mu        sync.Mutex
	tickets   []*pb.GetTicketsResponse
	lastReq   *pb.GetTicketsRequest
	streams   int
	feeStatus int
}

func ticketResponse(hash string, height int32) *pb.GetTicketsResponse {
	h := make([]byte, 32)
	copy(h, hash)
	return &pb.GetTicketsResponse{
		Ticket: &pb.GetTicketsResponse_TicketDetails{
			Ticket:       &pb.TransactionDetails{Hash: h},
			TicketStatus: pb.GetTicketsResponse_TicketDetails_LIVE,
		},
		Block: &pb.GetTicketsResponse_BlockDetails{Height: height},
	}
}

func (w *ticketWallet) GetTickets(_ context.Context, req *pb.GetTicketsRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.GetTicketsResponse], error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastReq = req
	w.streams++
	return &ticketStream{items: append([]*pb.GetTicketsResponse(nil), w.tickets...)}, nil
}

func (w *ticketWallet) GetVSPTicketsByFeeStatus(context.Context, *pb.GetVSPTicketsByFeeStatusRequest, ...grpc.CallOption) (*pb.GetVSPTicketsByFeeStatusResponse, error) {
	w.mu.Lock()
	w.feeStatus++
	w.mu.Unlock()
	return &pb.GetVSPTicketsByFeeStatusResponse{}, nil
}

func (w *ticketWallet) add(hash string, height int32) {
	w.mu.Lock()
	w.tickets = append(w.tickets, ticketResponse(hash, height))
	w.mu.Unlock()
}

// The autobuyer only needs the tickets it can have bought: those mined since
// it started, or not yet mined. It must not pay for the whole history or for
// fee status it never reads.
func TestAutobuyerPollListsOnlyTicketsSinceItStarted(t *testing.T) {
	w := &ticketWallet{}
	w.add("old-ticket", 100)
	withWallet(t, w)

	st := &autobuyerPollState{since: 900, seen: map[string]struct{}{}}
	autobuyerTick(context.Background(), st)
	if w.lastReq == nil || w.lastReq.StartingBlockHeight != 900 {
		t.Fatalf("the poll listed from %+v, want a listing from height 900", w.lastReq)
	}
	if w.feeStatus != 0 {
		t.Fatalf("the poll made %d fee-status calls it never reads", w.feeStatus)
	}

	w.add("fresh-ticket", 0)
	autobuyerTick(context.Background(), st)
	raw := make([]byte, 32)
	copy(raw, "fresh-ticket")
	want, err := chainhash.NewHash(raw)
	if err != nil {
		t.Fatal(err)
	}
	purchased := 0
	for _, ev := range LastAutobuyerEvents(50) {
		if strings.Contains(ev.Message, "purchased ticket "+want.String()) {
			purchased++
		}
	}
	if purchased != 1 {
		t.Fatalf("%d purchase events for the new ticket, want exactly 1", purchased)
	}
}

// One listing serves everyone who asks within the TTL, for the same wallet
// and tip, until this process changes the set.
func TestTicketListIsSharedForTenSeconds(t *testing.T) {
	w := &ticketWallet{}
	w.add("a-ticket", 500)
	withWallet(t, w)
	withDcrdClient(t, nil)
	t.Cleanup(SetNodeSyncSnapshotForTest(NodeSyncSnapshot{Status: "running", Blocks: 1000}))
	prevTTL := ticketListTTL
	t.Cleanup(func() { ticketListTTL = prevTTL; invalidateTicketList() })
	invalidateTicketList()

	list := func() {
		t.Helper()
		if _, err := ListTickets(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	list()
	list()
	if w.streams != 1 {
		t.Fatalf("two listings streamed the wallet %d times, want 1", w.streams)
	}
	invalidateTicketList()
	list()
	if w.streams != 2 {
		t.Fatalf("a listing after a change streamed %d times in total, want 2", w.streams)
	}
	t.Cleanup(SetNodeSyncSnapshotForTest(NodeSyncSnapshot{Status: "running", Blocks: 1001}))
	list()
	if w.streams != 3 {
		t.Fatalf("a listing at a new tip streamed %d times in total, want 3", w.streams)
	}
	activeWalletMu.Lock()
	prevWallet := activeWallet
	activeWallet = "other-wallet"
	activeWalletMu.Unlock()
	t.Cleanup(func() {
		activeWalletMu.Lock()
		activeWallet = prevWallet
		activeWalletMu.Unlock()
	})
	list()
	if w.streams != 4 {
		t.Fatalf("a listing on another wallet streamed %d times in total, want 4", w.streams)
	}
	ticketListTTL = 0
	time.Sleep(time.Millisecond)
	list()
	if w.streams != 5 {
		t.Fatalf("a listing past the TTL streamed %d times in total, want 5", w.streams)
	}
}
