// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/types"
)

// TestStakingPurchaseChecksBothAccounts verifies that staking_purchase gates on
// the change account as well as the source. The ticket split's change output is
// not bounded by the reserved ticket price, so an unchecked change account puts
// funds into one the operator never covered.
func TestStakingPurchaseChecksBothAccounts(t *testing.T) {
	const agentID = "staking-accounts"
	grants.set(agentID, GrantSpec{
		Accounts:   []uint32{0},
		PerTxAtoms: 1e8,
		DailyAtoms: 1e8,
	}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "staking", map[string]bool{"staking": true}))
	call := func(t *testing.T, args map[string]any) string {
		t.Helper()
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "staking_purchase", Arguments: args,
		})
		if err != nil {
			t.Fatalf("CallTool(%v) transport error: %v", args, err)
		}
		return resultText(res)
	}
	const denied = "not covered by the agent's spend grant"

	t.Run("an ungranted change account is refused", func(t *testing.T) {
		txt := call(t, map[string]any{
			"account": 0, "changeAccount": 7, "numTickets": 1,
			"vspHost": "https://vsp.example", "vspPubkey": "k",
		})
		if !strings.Contains(txt, denied) {
			t.Fatalf("change account 7 was not refused: %q", txt)
		}
	})

	t.Run("an ungranted source account is still refused", func(t *testing.T) {
		txt := call(t, map[string]any{
			"account": 7, "numTickets": 1,
			"vspHost": "https://vsp.example", "vspPubkey": "k",
		})
		if !strings.Contains(txt, denied) {
			t.Fatalf("source account 7 was not refused: %q", txt)
		}
	})

	// A granted pair gets past the account gate and then fails further along on
	// the VSP lookup, so assert the gate's message is absent rather than success
	// - the carve-out gating_test.go uses for ln_pay.
	t.Run("granted accounts pass the account gate", func(t *testing.T) {
		for _, args := range []map[string]any{
			{"account": 0, "numTickets": 1, "vspHost": "https://vsp.example", "vspPubkey": "k"},
			{"account": 0, "changeAccount": 0, "numTickets": 1, "vspHost": "https://vsp.example", "vspPubkey": "k"},
		} {
			if txt := call(t, args); strings.Contains(txt, denied) {
				t.Errorf("granted accounts %v refused by the account gate: %q", args, txt)
			}
		}
	})
}

// TestToolDescriptionsNameGrantedAccounts checks that the per-agent description
// reaches the tool listing. The text is only a hint - the grant checks remain
// the authority - but a hint that never renders is worse than none.
func TestToolDescriptionsNameGrantedAccounts(t *testing.T) {
	const agentID = "describe-accounts"
	grants.set(agentID, GrantSpec{
		Accounts:   []uint32{2, 0},
		PerTxAtoms: 1e8,
		DailyAtoms: 1e8,
	}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "describe", map[string]bool{"staking": true, "wallet": true}))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := map[string]string{}
	for _, tl := range res.Tools {
		got[tl.Name] = tl.Description
	}

	// Sorted and de-duplicated, so the text is stable whatever order the grant
	// was written in.
	const want = "Your grant covers accounts 0, 2."
	for _, name := range []string{"staking_purchase", "wallet_send", "staking_sync_failed_vsp_tickets"} {
		desc, ok := got[name]
		if !ok {
			t.Fatalf("%s missing from the tool listing", name)
		}
		if !strings.Contains(desc, want) {
			t.Errorf("%s description does not name the granted accounts: %q", name, desc)
		}
	}
}

// TestGrantedAccountsWithoutGrant covers the ungranted case: the hint has to say
// so rather than claim an empty set of accounts is usable.
func TestGrantedAccountsWithoutGrant(t *testing.T) {
	if got := grantedAccounts(testAgent("no-grant", "none", nil)); !strings.Contains(got, "refused") {
		t.Errorf("grantedAccounts without a grant = %q, want it to say a call would be refused", got)
	}
}

// TestVSPFeeCandidatesNeedsFeeStatus covers the input shape that makes a VSP
// maintenance run unboundable. fetchFeeStatusMap logs and continues on each of
// its four lookups, so a wallet that cannot report fee status yields tickets with
// an empty FeeStatus - which the sync run must not read as "nothing to pay".
func TestVSPFeeCandidatesNeedsFeeStatus(t *testing.T) {
	noFeeStatus := []types.TicketRecord{
		{Status: "LIVE", FeeStatus: "", TicketPrice: 100},
		{Status: "LIVE", FeeStatus: "", TicketPrice: 100},
	}

	t.Run("sync counts nothing without fee status", func(t *testing.T) {
		if count, price := vspFeeCandidates(noFeeStatus, false); count != 0 || price != 0 {
			t.Fatalf("count=%d price=%v, want 0/0 so the run is refused rather than unbounded", count, price)
		}
	})

	t.Run("sync counts tickets whose fees failed or are unpaid", func(t *testing.T) {
		tickets := []types.TicketRecord{
			{Status: "LIVE", FeeStatus: "ERRORED", TicketPrice: 100},
			{Status: "LIVE", FeeStatus: "UNPAID", TicketPrice: 50},
			{Status: "LIVE", FeeStatus: "PAID", TicketPrice: 999},
		}
		if count, price := vspFeeCandidates(tickets, false); count != 2 || price != 150 {
			t.Fatalf("count=%d price=%v, want 2/150", count, price)
		}
	})

	t.Run("an empty ticket list counts nothing", func(t *testing.T) {
		if count, _ := vspFeeCandidates(nil, false); count != 0 {
			t.Fatalf("count=%d, want 0", count)
		}
		if count, _ := vspFeeCandidates(nil, true); count != 0 {
			t.Fatalf("unmanaged count=%d, want 0", count)
		}
	})
}

// TestVSPInfoProbeIsConstrained pins that the VSP probe cannot be pointed at an
// arbitrary host. It makes an outbound request to whatever it is given, so it is
// limited to the same set the paying staking tools accept.
func TestVSPInfoProbeIsConstrained(t *testing.T) {
	cs := connectTo(t, testAgent("vsp-probe", "probe", map[string]bool{"staking": true}))
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "staking_vsp_info", Arguments: map[string]any{"host": "https://evil.example"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !out.IsError {
		t.Fatal("an unknown VSP host was probed rather than refused")
	}
	// With no wallet and no registry reachable both candidate sets are empty, so
	// every host is refused here; this pins the refusal, not the accept path.
	if txt := resultText(out); !strings.Contains(txt, "is not a VSP this wallet has used") {
		t.Errorf("refused, but not by the known-VSP check: %q", txt)
	}
}
