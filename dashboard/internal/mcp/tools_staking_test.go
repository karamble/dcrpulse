// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/rpcclient/v8"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/rpc"
)

// withUnreachableWalletRPC points the wallet JSON-RPC client at a dead port so
// the account lookup fails instead of dereferencing a nil client. The tool reads
// the privacy accounts before it checks the grant, and that read must reach a
// client; a failing one puts it on the not-configured branch, which is the one
// where the caller's own change account is used.
func withUnreachableWalletRPC(t *testing.T) {
	t.Helper()
	c, err := rpcclient.New(&rpcclient.ConnConfig{
		Host: "127.0.0.1:1", User: "u", Pass: "p",
		HTTPPostMode: true, DisableTLS: true,
	}, nil)
	if err != nil {
		t.Fatalf("build wallet rpc client: %v", err)
	}
	prev := rpc.WalletClient
	rpc.WalletClient = c
	t.Cleanup(func() { rpc.WalletClient = prev })
}

// TestStakingPurchaseChecksBothAccounts verifies that staking_purchase gates on
// the change account as well as the source. The ticket split's change output is
// not bounded by the reserved ticket price, so an unchecked change account puts
// funds into one the operator never covered.
func TestStakingPurchaseChecksBothAccounts(t *testing.T) {
	withUnreachableWalletRPC(t)
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
