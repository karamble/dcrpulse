// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

func TestRecoveryReasonsNameTheRefund(t *testing.T) {
	const refund, other = "aa11", "bb22"
	cases := []struct{ got, want string }{
		{pendingReason(refund, refund), "Refund aa11 is in the mempool, waiting for a block"},
		{pendingReason(other, refund), "Another transaction bb22 spends this output"},
		{pendingReason("", refund), "Refund aa11 is signed; broadcasting, retried automatically"},
		{pendingReason("", ""), "A transaction already reserves this output"},
		{spendReason(refund, refund, true), "Refunded to your wallet in aa11"},
		{spendReason(other, refund, true), "Spent by transaction bb22"},
		{spendReason(refund, refund, false), "Refund aa11 is in the mempool, waiting for a block"},
		{spendReason(other, refund, false), "Transaction bb22 spends this output and is waiting for a block"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: got %q, want %q", i, c.got, c.want)
		}
	}
}
