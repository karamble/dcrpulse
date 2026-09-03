// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"reflect"
	"testing"

	"dcrpulse/internal/types"
)

// TestStakingAccounts pins which accounts count as staking accounts. The result
// is suggested to an agent as a funding account, so an account a purchase cannot
// spend from must never appear.
func TestStakingAccounts(t *testing.T) {
	accounts := []types.AccountInfo{
		{AccountName: "default", AccountNumber: 0, LockedByTickets: 12.5},
		{AccountName: "savings", AccountNumber: 1},                                // no stake activity
		{AccountName: "rewards", AccountNumber: 3, ImmatureStakeGeneration: 0.9},  // votes returning
		{AccountName: "watcher", AccountNumber: 4, VotingAuthority: 99},           // votes for others' tickets
		{AccountName: "imported", AccountNumber: 2147483647, LockedByTickets: 50}, // cannot be spent from
		{AccountName: "dex", AccountNumber: 6, LockedByTickets: 7},                // bisonw-managed
		{AccountName: "xpubwatch", AccountNumber: 1 << 31, LockedByTickets: 3},    // xpub-imported
	}
	got := stakingAccounts(accounts)
	want := []uint32{0, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stakingAccounts = %v, want %v", got, want)
	}
}

// TestStakingAccountsNone covers a wallet that has never staked: no accounts
// rather than a misleading suggestion.
func TestStakingAccountsNone(t *testing.T) {
	if got := stakingAccounts([]types.AccountInfo{
		{AccountName: "default", AccountNumber: 0, SpendableBalance: 100},
	}); len(got) != 0 {
		t.Errorf("stakingAccounts on a non-staking wallet = %v, want none", got)
	}
}

// TestTicketVSPUse pins the "most used" ordering the tool description relies on.
func TestTicketVSPUse(t *testing.T) {
	tickets := []types.TicketRecord{
		{VSPHost: "https://b.example"}, {VSPHost: "https://a.example"},
		{VSPHost: "https://a.example"}, {VSPHost: "https://a.example"},
		{VSPHost: "https://c.example"}, {VSPHost: "https://c.example"},
		{VSPHost: ""}, // solo ticket, no VSP
	}
	got := ticketVSPUse(tickets)
	want := []types.VSPUse{
		{Host: "https://a.example", Tickets: 3},
		{Host: "https://c.example", Tickets: 2},
		{Host: "https://b.example", Tickets: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ticketVSPUse = %v, want %v", got, want)
	}
}
