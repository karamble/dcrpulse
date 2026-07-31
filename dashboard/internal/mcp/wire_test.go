// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newWireServer serves authMiddleware(buildHandler()) for a fresh node-only
// agent, returning the endpoint URL and the agent's bearer token.
func newWireServer(t *testing.T) (url, token string) {
	t.Helper()
	r := newRegistry()
	const id, tok = "wire-agent", "wire-test-token"
	r.addToken(id, "wire", tok)
	t.Cleanup(func() { invalidateAgentServer(id) })
	srv := httptest.NewServer(r.authMiddleware(buildHandler()))
	t.Cleanup(srv.Close)
	return srv.URL, tok
}

// postMCP sends one JSON-RPC frame the way a raw (non-SDK) agent would,
// returning the response and the decoded body: SSE data lines concatenated,
// plain JSON as-is.
func postMCP(t *testing.T, url, token, body string, hdr ...string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		var sb strings.Builder
		for _, line := range strings.Split(text, "\n") {
			if data, ok := strings.CutPrefix(line, "data:"); ok {
				sb.WriteString(strings.TrimSpace(data))
			}
		}
		text = sb.String()
	}
	return resp, text
}

// rpcFrame decodes one JSON-RPC response body into its result and error
// members.
func rpcFrame(t *testing.T, body string) (result, rpcErr json.RawMessage) {
	t.Helper()
	var frame struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &frame); err != nil {
		t.Fatalf("undecodable JSON-RPC body: %v: %s", err, body)
	}
	return frame.Result, frame.Error
}

// TestLegacyWireHTTP drives the endpoint the way a pre-2026-07-28 agent does -
// raw initialize, no session header discipline - and pins the visible changes
// of the stateless handler: no Mcp-Session-Id, no standalone GET stream, and
// the synchronous legacy subscribe deny.
func TestLegacyWireHTTP(t *testing.T) {
	url, token := newWireServer(t)

	resp, body := postMCP(t, url, token,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"legacy","version":"0"}}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize status %d: %s", resp.StatusCode, body)
	}
	res, rpcErr := rpcFrame(t, body)
	if len(rpcErr) > 0 {
		t.Fatalf("initialize error: %s", rpcErr)
	}
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(res, &init); err != nil {
		t.Fatal(err)
	}
	if init.ProtocolVersion != "2025-11-25" {
		t.Fatalf("initialize answered %q; want 2025-11-25", init.ProtocolVersion)
	}
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.Fatalf("stateless endpoint issued a session id %q", sid)
	}

	// A bare tools/call with no prior initialize and no session header:
	// per-request synthesis is what keeps old agents working against the
	// stateless endpoint.
	resp, body = postMCP(t, url, token,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"capabilities","arguments":{}}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/call status %d: %s", resp.StatusCode, body)
	}
	res, rpcErr = rpcFrame(t, body)
	if len(rpcErr) > 0 {
		t.Fatalf("tools/call error: %s", rpcErr)
	}
	if !strings.Contains(string(res), `"content"`) {
		t.Fatalf("capabilities result: %s", res)
	}

	// The legacy resources/subscribe stays synchronous, so the gate's refusal
	// is visible on this wire.
	_, body = postMCP(t, url, token,
		`{"jsonrpc":"2.0","id":3,"method":"resources/subscribe","params":{"uri":"`+resAudit+`"}}`)
	if _, rpcErr = rpcFrame(t, body); !strings.Contains(string(rpcErr), "resource not available") {
		t.Fatalf("subscribe to ungranted resource: want a refusal, got error=%s", rpcErr)
	}
	_, body = postMCP(t, url, token,
		`{"jsonrpc":"2.0","id":4,"method":"resources/subscribe","params":{"uri":"`+resNodeSync+`"}}`)
	if _, rpcErr = rpcFrame(t, body); len(rpcErr) > 0 {
		t.Fatalf("subscribe to granted resource refused: %s", rpcErr)
	}

	// The standalone SSE stream and session DELETE are gone; both eras of
	// agents treat 405 as "not offered".
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Mcp-Session-Id", "ignored")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s status %d != 405", method, resp.StatusCode)
		}
	}
}

// TestStatelessDiscoverSurface pins the dual-wire advertisement: one discover
// probe with the SEP-2243 headers, no session residue, both protocol
// generations offered.
func TestStatelessDiscoverSurface(t *testing.T) {
	url, token := newWireServer(t)

	resp, body := postMCP(t, url, token,
		`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`,
		"Mcp-Protocol-Version", "2026-07-28", "Mcp-Method", "server/discover")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("discover status %d: %s", resp.StatusCode, body)
	}
	res, rpcErr := rpcFrame(t, body)
	if len(rpcErr) > 0 {
		t.Fatalf("discover error: %s", rpcErr)
	}
	var disc struct {
		SupportedVersions []string `json:"supportedVersions"`
	}
	if err := json.Unmarshal(res, &disc); err != nil {
		t.Fatal(err)
	}
	var new2026, legacy bool
	for _, v := range disc.SupportedVersions {
		new2026 = new2026 || v == "2026-07-28"
		legacy = legacy || v == "2025-11-25"
	}
	if !new2026 || !legacy {
		t.Fatalf("supportedVersions %v; want both 2026-07-28 and 2025-11-25", disc.SupportedVersions)
	}
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.Fatalf("discover response carries a session id %q", sid)
	}
}
