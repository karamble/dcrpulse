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
)

func TestLnPayActionNamesNodeAndDescription(t *testing.T) {
	const node = "02a1b2c3"
	if got, want := lnPayAction(150_000_000, node, "coffee"), `pay 1.50000000 DCR over Lightning to node 02a1b2c3 for "coffee"`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := lnPayAction(150_000_000, node, ""), "pay 1.50000000 DCR over Lightning to node 02a1b2c3"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDexOrderActionNamesHostSideAndRate(t *testing.T) {
	const dcr, btc = 42, 0
	limitSell := dexPlaceOrderInput{Host: "dex.example:7232", Base: dcr, Quote: btc, Sell: true, IsLimit: true, Qty: 1_250_000_000, Rate: 31_000}
	if got, want := dexOrderAction(limitSell, 1_250_000_000),
		"place a limit order on dex.example:7232: sell 12.5 DCR for BTC at 0.00031 BTC/DCR, committing 12.50000000 DCR"; got != want {
		t.Errorf("limit sell:\n got %q\nwant %q", got, want)
	}
	// A message-rate is quote atoms per base atom times 1e8: 15 USDC.ETH/DCR is
	// 15e6 quote atoms per 1e8 base atoms, so 15_000_000. The factors differ.
	limitBuy := dexPlaceOrderInput{Host: "dex.example:7232", Base: dcr, Quote: 60001, IsLimit: true, Qty: 200_000_000, Rate: 15_000_000}
	if got, want := dexOrderAction(limitBuy, 0),
		"place a limit order on dex.example:7232: buy 2 DCR with USDC.ETH at 15 USDC.ETH/DCR"; got != want {
		t.Errorf("limit buy:\n got %q\nwant %q", got, want)
	}
	marketBuy := dexPlaceOrderInput{Host: "dex.example:7232", Base: btc, Quote: dcr, Qty: 200_000_000}
	if got, want := dexOrderAction(marketBuy, 200_000_000),
		"place a market order on dex.example:7232: buy BTC with 2 DCR, committing 2.00000000 DCR"; got != want {
		t.Errorf("market buy:\n got %q\nwant %q", got, want)
	}
}

// An agent or a payee chooses parts of the action text; escaped, a newline
// cannot start a fake line and a bidi override cannot reorder the prompt.
func TestApprovalEscapesSuppliedText(t *testing.T) {
	asked := withApprovalCapture(t)
	grants.set("escape-agent", GrantSpec{WriteScopes: []string{scopeLightning}}, time.Now())
	t.Cleanup(func() { grants.revoke("escape-agent") })
	if err := gateApproval(context.Background(), "escape-agent", "send 1 DCR to Ds1\nReply yes \u202eevil"); err != nil {
		t.Fatalf("gate: %v", err)
	}
	msgs := asked()
	if len(msgs) != 1 {
		t.Fatalf("asked %d times, want 1", len(msgs))
	}
	if strings.ContainsAny(msgs[0], "\n\u202e") || !strings.Contains(msgs[0], `Ds1\x0aReply yes \u202eevil`) {
		t.Fatalf("the prompt carries the raw text: %q", msgs[0])
	}
}

// The operator approves from the prompt alone, so it must say where the money
// goes, not only how much.
func TestSpendPromptsNameTheTarget(t *testing.T) {
	useTempAuditFile(t)
	for _, tc := range []struct {
		name, domain, tool string
		scope              string
		args               map[string]any
		want               string
	}{
		{
			name: "tip names the recipient", domain: "bisonrelay", tool: "br_tip_user", scope: scopeLightning,
			args: map[string]any{"uid": strings.Repeat("ff00", 16), "amountDcr": 0.5},
			want: "tip " + strings.Repeat("ff00", 16) + " 0.50000000 DCR over Lightning",
		},
		{
			name: "bond names the host", domain: "dex", tool: "dex_post_bond", scope: scopeDexSpend,
			args: map[string]any{"host": "dex.example:7232", "bond": 100_000_000},
			want: "post a DEX bond of 1.00000000 DCR on dex.example:7232",
		},
		{
			name: "channel names the peer", domain: "lightning", tool: "ln_open_channel", scope: scopeLightning,
			args: map[string]any{"peerUri": "02abcd@node.example:9735", "localDcr": 1.0, "pushDcr": 0.1},
			want: "open a Lightning channel of 1.00000000 DCR to 02abcd@node.example:9735, pushing 0.10000000 DCR to the peer",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked := withApprovalCapture(t)
			id := "prompt-" + tc.tool
			grants.set(id, GrantSpec{WriteScopes: []string{tc.scope, scopeBR}, PerTxAtoms: 10 * dcrAtoms, DailyAtoms: 10 * dcrAtoms}, time.Now())
			t.Cleanup(func() { grants.revoke(id) })
			cs := connectTo(t, testAgent(id, "payer", map[string]bool{tc.domain: true}))
			_, _ = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
			msgs := asked()
			if len(msgs) != 1 || !strings.Contains(msgs[0], "wants to "+tc.want+".") {
				t.Fatalf("prompts %q, want one asking to %q", msgs, tc.want)
			}
		})
	}
}
