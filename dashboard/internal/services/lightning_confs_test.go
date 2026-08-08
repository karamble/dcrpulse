// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

const dcr = int64(1e8)

// The thresholds come from dcrlnd's own formula (server.go:1192): with the
// scaling cancelled down, a channel needs 3 confirmations until its stake
// reaches 7.1583 DCR, then one more per 1.7896 DCR up to 6.
func TestRequiredConfsForCapacity(t *testing.T) {
	tests := []struct {
		name     string
		capacity int64
		pushAmt  int64
		want     int32
	}{
		{"tiny channels take the floor", dcr / 10, 0, 3},
		{"3 DCR is still the floor", 3 * dcr, 0, 3},
		{"just under the first step", 715827881, 0, 3},
		{"the first step up", 715827882, 0, 4},
		{"just under the second step", 894784852, 0, 4},
		{"the second step up", 894784853, 0, 5},
		{"the ceiling", 1073741823, 0, 6},
		{"wumbo takes the ceiling", 2 * 1073741823, 0, 6},
		{"a push counts toward the stake", 4 * dcr, 4 * dcr, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := requiredConfsForCapacity(tc.capacity, tc.pushAmt); got != tc.want {
				t.Errorf("requiredConfsForCapacity(%d, %d) = %d, want %d",
					tc.capacity, tc.pushAmt, got, tc.want)
			}
		})
	}
}

// Only the side that accepted a channel sizes the requirement, over what was
// pushed to it. Feeding the other side's balance roughly doubles the stake.
func TestFundingPushProxy(t *testing.T) {
	const local, remote = 1 * dcr, 4 * dcr

	if got := fundingPushProxy(true, local, remote); got != remote {
		t.Errorf("we opened it: proxy = %d, want the remote balance %d", got, remote)
	}
	if got := fundingPushProxy(false, local, remote); got != local {
		t.Errorf("they opened it: proxy = %d, want our own balance %d", got, local)
	}
}

// The bug this fixes, in the terms it appeared in: a 4 DCR channel the peer
// opened to us needs 3 confirmations, but was displayed as 4 because the
// remote balance was fed in as the push. Divergence starts at 3.5791 DCR and
// reaches 6 by 5.3687, while dcrlnd stays at 3 until 7.1583.
func TestInboundChannelConfsMatchDcrlnd(t *testing.T) {
	tests := []struct {
		capacity    int64
		wantInbound int32
		wantIfWrong int32
	}{
		{3 * dcr, 3, 3},   // below the divergence, nothing visibly changed
		{357913940, 3, 3}, // the last capacity that agreed
		{357913941, 3, 4}, // the exact first divergence
		{4 * dcr, 3, 4},
		{447392427, 3, 5},
		{536870912, 3, 6},
		{8 * dcr, 4, 6},
	}

	for _, tc := range tests {
		// The peer opened it and pushed us nothing, so our balance is zero
		// and theirs is the whole capacity.
		proxy := fundingPushProxy(false, 0, tc.capacity)
		if got := requiredConfsForCapacity(tc.capacity, proxy); got != tc.wantInbound {
			t.Errorf("capacity %d: got %d confirmations, want %d", tc.capacity, got, tc.wantInbound)
		}
		// Guards the branch: feeding the remote balance is what over-reported.
		if got := requiredConfsForCapacity(tc.capacity, tc.capacity); got != tc.wantIfWrong {
			t.Errorf("capacity %d: the old behaviour gave %d, expected the table's %d",
				tc.capacity, got, tc.wantIfWrong)
		}
	}
}
