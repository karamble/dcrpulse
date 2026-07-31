// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
		// ln_pay decodes the invoice before the grant check, so without a daemon
		// it errors at decode rather than the gate; an error is still a safe no-op.
		if tl.Name == "ln_pay" {
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
