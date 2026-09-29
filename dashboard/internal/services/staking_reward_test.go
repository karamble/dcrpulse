// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

func votedTicket(priceAtoms, spenderCredit int64) *pb.GetTicketsResponse {
	return &pb.GetTicketsResponse{Ticket: &pb.GetTicketsResponse_TicketDetails{
		TicketStatus: pb.GetTicketsResponse_TicketDetails_VOTED,
		Ticket: &pb.TransactionDetails{
			Hash:    make([]byte, 32),
			Credits: []*pb.TransactionDetails_Output{{Index: 0, Amount: priceAtoms}},
		},
		Spender: &pb.TransactionDetails{
			Hash:    make([]byte, 32),
			Credits: []*pb.TransactionDetails_Output{{Index: 0, Amount: spenderCredit}},
		},
	}}
}

// A voted ticket's reward is the difference in atoms: subtracting the two as
// float DCR gives 0.0012345600000003287 here.
func TestTicketRewardIsExact(t *testing.T) {
	got := ticketRecordFromResponse(votedTicket(1_234_567_890, 1_234_691_346))
	if got.TicketPrice != 12.3456789 || got.Reward != 0.00123456 {
		t.Errorf("price %v reward %v, want 12.3456789 and exactly 0.00123456", got.TicketPrice, got.Reward)
	}
	if r := ticketRecordFromResponse(votedTicket(1_000, 900)).Reward; r != 0 {
		t.Errorf("reward below the price = %v, want 0", r)
	}
}
