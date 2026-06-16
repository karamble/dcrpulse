// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	clientName    = "mcptest"
	clientVersion = "0.1.0"
	callTimeout   = 60 * time.Second // treasury/governance reads can be slow
)

// bearerRT injects a static bearer token on every request so the streamable
// transport (POSTs and the standalone SSE GET) authenticates as the agent.
type bearerRT struct {
	token string
	base  http.RoundTripper
}

func (b bearerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// connect opens an MCP session against endpoint authenticating with token.
func connect(ctx context.Context, endpoint, token string) (*mcp.ClientSession, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           &http.Client{Transport: bearerRT{token: token, base: http.DefaultTransport}},
		DisableStandaloneSSE: true, // request/response only; no server-initiated stream needed
	}
	client := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: clientVersion}, nil)
	return client.Connect(ctx, transport, nil)
}

// callResult is the flattened outcome of a single tool call.
type callResult struct {
	isError bool
	text    string // concatenated text content: a JSON payload, or an error message
	data    any    // the unwrapped tool payload (the value under "data"), nil on error
}

// call invokes a tool and flattens its result. A transport/protocol failure is
// returned as err; a tool-level error (e.g. a denied spend) comes back with
// isError=true and the message in text.
func call(ctx context.Context, s *mcp.ClientSession, name string, args map[string]any) (callResult, error) {
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	res, err := s.CallTool(cctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return callResult{}, err
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	out := callResult{isError: res.IsError, text: sb.String()}
	if !res.IsError {
		out.data = unwrapData(out.text)
	}
	return out, nil
}

// unwrapData parses a tool's JSON text payload and returns the value nested
// under "data" (the server wraps every read result as {"data": ...}).
func unwrapData(text string) any {
	var v any
	if json.Unmarshal([]byte(text), &v) != nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		if d, ok := m["data"]; ok {
			return d
		}
	}
	return v
}

var (
	hex64Re = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	// Decred mainnet/testnet base58 addresses: a leading D (mainnet) or T/S
	// (testnet/simnet) followed by base58 characters.
	addrRe = regexp.MustCompile(`^[DTS][a-km-zA-HJ-NP-Z1-9]{24,34}$`)
)

// walk visits every scalar in a decoded-JSON tree, calling fn with the nearest
// object key (empty for array elements) and the scalar value.
func walk(v any, key string, fn func(key string, val any)) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			walk(child, k, fn)
		}
	case []any:
		for _, child := range t {
			walk(child, key, fn)
		}
	default:
		fn(key, v)
	}
}

// findString returns the first string in the tree whose key matches one of
// keys (case-insensitive) and whose value satisfies ok. keys may be empty to
// match on value alone.
func findString(v any, ok func(string) bool, keys ...string) string {
	want := map[string]bool{}
	for _, k := range keys {
		want[strings.ToLower(k)] = true
	}
	var found string
	walk(v, "", func(key string, val any) {
		if found != "" {
			return
		}
		s, isStr := val.(string)
		if !isStr || !ok(s) {
			return
		}
		if len(want) == 0 || want[strings.ToLower(key)] {
			found = s
		}
	})
	return found
}

// findInt returns the first number in the tree whose key matches one of keys.
func findInt(v any, keys ...string) (int64, bool) {
	want := map[string]bool{}
	for _, k := range keys {
		want[strings.ToLower(k)] = true
	}
	var found int64
	var ok bool
	walk(v, "", func(key string, val any) {
		if ok || !want[strings.ToLower(key)] {
			return
		}
		if f, isNum := val.(float64); isNum {
			found, ok = int64(f), true
		}
	})
	return found, ok
}

// findFloat returns the first number in the tree whose key matches one of keys.
func findFloat(v any, keys ...string) (float64, bool) {
	want := map[string]bool{}
	for _, k := range keys {
		want[strings.ToLower(k)] = true
	}
	var found float64
	var ok bool
	walk(v, "", func(key string, val any) {
		if ok || !want[strings.ToLower(key)] {
			return
		}
		if f, isNum := val.(float64); isNum {
			found, ok = f, true
		}
	})
	return found, ok
}

func isHex64(s string) bool { return hex64Re.MatchString(s) }
func isAddr(s string) bool  { return addrRe.MatchString(s) }
func anyString(string) bool { return true }

// preview collapses whitespace and truncates a payload for one-line display.
func preview(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
