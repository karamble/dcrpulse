// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// STAKEGOV-8: a trickled vote is decided only by a receipt for its own ticket,
// and a retry answered "ticket already voted" (Politeia code 9) is the earlier
// attempt having gone through.
func TestTrickleReceiptOutcome(t *testing.T) {
	vote := piBallotVote{Ticket: "aa11"}
	own := func(code int) piCastBallotReceipt {
		return piCastBallotReceipt{Ticket: "aa11", ErrorCode: code, ErrorMsg: "x"}
	}
	for _, tc := range []struct {
		name     string
		receipts []piCastBallotReceipt
		attempt  int
		want     receiptOutcome
	}{
		{"no receipts", nil, 0, receiptRetry},
		{"another ticket's receipt", []piCastBallotReceipt{{Ticket: "bb22"}}, 0, receiptRetry},
		{"two receipts", []piCastBallotReceipt{own(0), own(0)}, 0, receiptRetry},
		{"accepted", []piCastBallotReceipt{own(0)}, 0, receiptCast},
		{"already voted before this trickle", []piCastBallotReceipt{own(9)}, 0, receiptFailed},
		{"already voted on a retry", []piCastBallotReceipt{own(9)}, 2, receiptCast},
		{"signature invalid", []piCastBallotReceipt{own(7)}, 1, receiptFailed},
		{"ticket not eligible on a retry", []piCastBallotReceipt{own(8)}, 1, receiptFailed},
	} {
		if got, _ := trickleReceiptOutcome(piCastBallotResponse{Receipts: tc.receipts}, vote, tc.attempt); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
