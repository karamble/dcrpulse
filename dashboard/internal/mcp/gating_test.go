// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// minimalArgs builds a schema-valid argument map for a tool by filling every
// required property with a type-appropriate dummy value. The tool input schemas
// here are flat (scalars + arrays), so this is enough to pass input validation
// and reach the handler's grant check.
func minimalArgs(schema any) map[string]any {
	raw, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{}
	}
	var s struct {
		Properties map[string]struct {
			Type any `json:"type"` // string, or ["null","array"] for nullable fields
		} `json:"properties"`
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(raw, &s)
	args := map[string]any{}
	for _, name := range s.Required {
		args[name] = dummyForType(s.Properties[name].Type)
	}
	return args
}

// dummyForType returns a schema-valid placeholder for a JSON Schema type, which
// may be a plain string ("string") or a union list (["null","array"]).
func dummyForType(t any) any {
	typ := ""
	switch v := t.(type) {
	case string:
		typ = v
	case []any:
		for _, e := range v {
			if es, ok := e.(string); ok && es != "null" {
				typ = es
				break
			}
		}
	}
	switch typ {
	case "integer", "number":
		return 1
	case "boolean":
		return false
	case "array":
		return []any{}
	case "object":
		return map[string]any{}
	default: // string and anything unspecified
		return "x"
	}
}

func resultText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

// TestEveryWriteToolGatesWithoutGrant is the security invariant net: an agent
// with every domain but NO spend grant must have EVERY write tool refused before
// it does anything. Daemon-free - gate-first ordering means each write returns
// the grant denial before any service call. Write tools are identified by the
// absence of the read-only annotation (which also confirms annotations are
// emitted). A new write tool added without a gate fails this test.
func TestEveryWriteToolGatesWithoutGrant(t *testing.T) {
	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	a := testAgent("gating-test", "gating", domains)
	cs := connectTo(t, a)

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	writes := 0
	for _, tl := range res.Tools {
		if tl.Annotations != nil && tl.Annotations.ReadOnlyHint {
			continue // read-only tool (incl. capabilities); nothing to gate
		}
		writes++
		out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      tl.Name,
			Arguments: minimalArgs(tl.InputSchema),
		})
		if err != nil {
			t.Errorf("%s: unexpected transport/validation error (placeholder args insufficient?): %v", tl.Name, err)
			continue
		}
		if !out.IsError {
			t.Errorf("%s: SUCCEEDED without a spend grant - missing grant gate?", tl.Name)
			continue
		}
		if txt := resultText(out); !strings.Contains(txt, "no spend grant") && !strings.Contains(txt, "no write grant") &&
			!strings.Contains(txt, "does not allow") && !strings.Contains(txt, "does not include") {
			t.Errorf("%s: refused, but not by the grant gate: %q", tl.Name, txt)
		}
	}
	if writes == 0 {
		t.Fatal("no write tools detected - annotation classification is broken")
	}
	t.Logf("grant gate verified for %d write tools", writes)
}

// TestDomainRevocationAppliesToLiveSession covers the revocation gap: a session
// keeps the server it was built with, so the domain has to be re-checked when
// the tool is called or a removed domain keeps working until the session ends.
func TestDomainRevocationAppliesToLiveSession(t *testing.T) {
	// The session is built while the wallet domain is granted, so wallet tools
	// are registered on its server.
	a := testAgent("revoke-test", "revoke", map[string]bool{"node": true, "wallet": true})
	cs := connectTo(t, a)

	// Take the domain away on that same live session. The tool stays registered
	// (the server is not rebuilt), so only the per-call check can refuse it -
	// and it must refuse before reaching the absent daemon.
	a.setDomainMap(map[string]bool{"node": true})

	after, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "wallet_accounts"})
	if err != nil {
		t.Fatalf("call after revocation: %v", err)
	}
	if !after.IsError {
		t.Fatal("wallet_accounts succeeded after its domain was revoked")
	}
	if txt := resultText(after); !strings.Contains(txt, "does not allow") {
		t.Fatalf("refused, but not by the domain check: %q", txt)
	}
}

// TestFreezeMechanics covers the kill-switch in-memory effects: every grant is
// revoked (passphrase zeroed) and every agent token is blocked.
func TestFreezeMechanics(t *testing.T) {
	r := newRegistry()
	r.addToken("a1", "one", "tok1")
	r.addToken("a2", "two", "tok2")
	r.blockAllAgents()
	for _, ai := range r.list() {
		if !ai.Blocked {
			t.Errorf("agent %s should be blocked after blockAllAgents", ai.ID)
		}
	}

	gs := newGrantStore()
	now := time.Now()
	gs.set("a1", GrantSpec{Accounts: []uint32{0}, PerTxAtoms: dcrAtoms, DailyAtoms: dcrAtoms, Passphrase: []byte("secret")}, now)
	gs.set("a2", GrantSpec{WriteScopes: []string{scopeTimestamp}}, now)
	g := gs.byAgent["a1"]
	gs.revokeAll()
	if _, ok := gs.info("a1"); ok {
		t.Error("a1 grant should be gone after revokeAll")
	}
	if _, ok := gs.info("a2"); ok {
		t.Error("a2 grant should be gone after revokeAll")
	}
	for _, b := range g.passphrase {
		if b != 0 {
			t.Fatal("passphrase not zeroed on revokeAll")
		}
	}
}

// TestReadToolsAnnotatedReadOnly confirms read tools advertise the read-only
// hint so MCP clients do not warn on them.
func TestReadToolsAnnotatedReadOnly(t *testing.T) {
	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	cs := connectTo(t, testAgent("anno-test", "anno", domains))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range res.Tools {
		if tl.Name == "capabilities" {
			if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
				t.Errorf("capabilities must be annotated read-only")
			}
		}
		if tl.Annotations == nil {
			t.Errorf("%s: missing annotations", tl.Name)
		}
	}
}

// capBoundTools are the fund-moving tools that reach their cap check without any
// daemon, with the arguments needed to attempt a spend above a 1-atom cap.
//
// The three staking fund tools are absent deliberately: staking_purchase and the
// two VSP maintenance runs resolve a VSP over the network and read the ticket
// price before they authorize, so without daemons they fail earlier than the cap
// and would assert nothing.
var capBoundTools = []struct {
	name string
	args map[string]any
}{
	{"wallet_send", map[string]any{"account": 0, "address": "DsTest", "amountDcr": 1.0}},
	{"ln_pay", map[string]any{"payReq": "lnbogus"}},
	{"ln_open_channel", map[string]any{"peerUri": lnTestPeer, "localDcr": 1.0}},
	{"ln_liquidity_request", map[string]any{"chanSizeDcr": 1.0, "approvedFeeDcr": 0.5}},
	{"br_tip_user", map[string]any{"uid": "ab12", "amountDcr": 1.0}},
	{"br_content_get", map[string]any{"uid": "ab12", "fid": "f", "maxCostAtoms": 100000000}},
	// base must stay 42 (bisonw.AssetDCR): dexOrderDCROutlay returns 0 for a
	// market with no DCR side, which makes the call scope-only and leaves
	// nothing for the cap to bind against.
	{"dex_place_order", map[string]any{"host": "h", "base": 42, "quote": 0, "qty": 100000000, "rate": 100000000, "isLimit": true, "sell": true}},
	{"dex_post_bond", map[string]any{"host": "h", "bond": 100000000}},
}

// TestFundToolsRespectTheCaps exercises the fund tools *with* a grant, which the
// rest of the suite never does. Pinning the scope alone cannot see a gate that
// was downgraded rather than dropped - swapping a reserving authorizer for a
// scope-only one keeps the scope name and silently stops the cap binding - so
// each tool here must refuse a spend that exceeds a 1-atom per-transaction cap.
//
// Each tool gets its own agent because exceeding a cap trips the tripwire, which
// revokes the grant and blocks the token.
func TestFundToolsRespectTheCaps(t *testing.T) {
	prev := rpc.SwapDcrlndClients(rpc.DcrlndClients{Lightning: stubLightning{decodeAtoms: 1e8}})
	t.Cleanup(func() { rpc.SwapDcrlndClients(prev) })

	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	for i, c := range capBoundTools {
		t.Run(c.name, func(t *testing.T) {
			id := fmt.Sprintf("cap-bound-%d", i)
			grants.set(id, GrantSpec{
				Accounts: []uint32{0}, PerTxAtoms: 1, DailyAtoms: 1,
				WriteScopes: []string{scopeLightning, scopeBR, scopeDex, scopeDexSpend, scopeStaking},
			}, time.Now())
			t.Cleanup(func() { grants.revoke(id) })

			cs := connectTo(t, testAgent(id, "cap", domains))
			out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: c.name, Arguments: c.args})
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if !out.IsError {
				t.Fatalf("spent above the per-transaction cap without refusing")
			}
			if txt := resultText(out); !strings.Contains(txt, errPerTxExceeded.Error()) &&
				!strings.Contains(txt, errDailyExceeded.Error()) {
				t.Errorf("refused, but not by the caps - the reservation may have been dropped: %q", txt)
			}
		})
	}
}

// A bond post already in flight from the browser must refuse the agent too, and
// the refusal must hand back the reservation it took, or the agent's next post
// dies on the daily cap instead of reaching the DEX.
func TestDexPostBondToolRefusesWhileAPostIsInFlight(t *testing.T) {
	const host = "inflight.mcp.test:7232"
	const bond = int64(100000000)
	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	const id = "bond-inflight"
	grants.set(id, GrantSpec{
		Accounts: []uint32{0}, PerTxAtoms: bond, DailyAtoms: bond,
		WriteScopes: []string{scopeDex, scopeDexSpend},
	}, time.Now())
	t.Cleanup(func() { grants.revoke(id) })
	cs := connectTo(t, testAgent(id, "bond", domains))
	call := func() *mcp.CallToolResult {
		out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "dex_post_bond", Arguments: map[string]any{"host": host, "bond": bond},
		})
		if err != nil {
			t.Fatalf("unexpected transport error: %v", err)
		}
		return out
	}

	if !services.BeginDexBondPost(host) {
		t.Fatal("could not seed an in-flight post")
	}
	out := call()
	if !out.IsError || !strings.Contains(resultText(out), "already being posted") {
		t.Fatalf("posted over an in-flight bond: %q", resultText(out))
	}
	services.EndDexBondPost(host, nil)

	// The daily cap equals one bond, so only a refunded reservation lets this
	// call reach the DEX gate, which is locked in a test process.
	out = call()
	if !out.IsError {
		t.Fatal("posted a bond in a test process")
	}
	if txt := resultText(out); !strings.Contains(txt, dexLocked().Error()) {
		t.Fatalf("second call refused by %q, want the DEX lock; was the reservation refunded?", txt)
	}
	if s := services.DexBondPostState(host); s.Phase != "error" || s.Error != "DCRDEX is locked" {
		t.Fatalf("registry after the agent's locked attempt: %+v", s)
	}
}
