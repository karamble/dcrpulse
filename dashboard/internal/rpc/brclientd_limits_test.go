// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A page fetch pulls markdown authored by a remote peer, and the handler
// unmarshals and re-renders it, so an unbounded read there is remote-controlled
// allocation. Bison Relay only fulfils a resource reply that fits one message
// payload (1 MiB on the current protocol version), so a generous cap cannot
// reject a page a peer could legitimately serve.
//
// The bodies must be reported as too large rather than truncated: a truncated
// JSON body fails later as a parse error, which points at the wrong thing.

func bodyResponse(t *testing.T, status int, n int) *http.Response {
	t.Helper()
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("x"), n))),
	}
}

func TestReadBrclientdBodyUnderLimit(t *testing.T) {
	resp := bodyResponse(t, http.StatusOK, 100)
	body, err := readBrclientdBody(resp, "/test", 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(body) != 100 {
		t.Errorf("read %d bytes, want 100", len(body))
	}
}

// Exactly at the limit is still valid; the cap is inclusive.
func TestReadBrclientdBodyAtLimit(t *testing.T) {
	resp := bodyResponse(t, http.StatusOK, 1024)
	body, err := readBrclientdBody(resp, "/test", 1024)
	if err != nil {
		t.Fatalf("unexpected error at exactly the limit: %v", err)
	}
	if len(body) != 1024 {
		t.Errorf("read %d bytes, want 1024", len(body))
	}
}

func TestReadBrclientdBodyOverLimitErrors(t *testing.T) {
	resp := bodyResponse(t, http.StatusOK, 1025)
	body, err := readBrclientdBody(resp, "/pages/fetch", 1024)
	if err == nil {
		t.Fatal("expected an error for an oversized success body")
	}
	if body != nil {
		t.Errorf("body returned alongside the error: %d bytes", len(body))
	}
	// The message has to name the cause; a truncated body would instead have
	// surfaced as a JSON parse failure somewhere else entirely.
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %q, want it to say the response exceeds the limit", err)
	}
}

// An oversized ERROR body is truncated, not rejected: it only becomes text in
// an error message, and failing to report a failure would be worse.
func TestReadBrclientdBodyTruncatesErrorBody(t *testing.T) {
	resp := bodyResponse(t, http.StatusInternalServerError, 5000)
	body, err := readBrclientdBody(resp, "/test", 1024)
	if err != nil {
		t.Fatalf("an oversized error body must not fail the read: %v", err)
	}
	if len(body) != 1024 {
		t.Errorf("error body is %d bytes, want it truncated to 1024", len(body))
	}
}

// The POST helper must apply the limit it is given, not a default.
func TestDoPostJSONRawAppliesItsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pad":"` + strings.Repeat("y", 4096) + `"}`))
	}))
	defer srv.Close()

	prev := BrclientdCfg
	host, port, _ := strings.Cut(strings.TrimPrefix(srv.URL, "http://"), ":")
	BrclientdCfg.Host, BrclientdCfg.StatusPort = host, port
	t.Cleanup(func() { BrclientdCfg = prev })

	// The helper builds an https URL, so point it at the plaintext test server
	// through a transport that rewrites the scheme.
	cli := &http.Client{Transport: schemeRewriter{srv.Client().Transport}}

	if _, err := brclientdDoPostJSONRaw(context.Background(), cli, "/x", struct{}{}, 1024); err == nil {
		t.Error("expected the small limit to reject a 4 KiB body")
	}
	if _, err := brclientdDoPostJSONRaw(context.Background(), cli, "/x", struct{}{}, 1<<20); err != nil {
		t.Errorf("the generous limit should have accepted the same body: %v", err)
	}
}

// schemeRewriter lets the https URL the helper builds reach an http test server.
type schemeRewriter struct{ rt http.RoundTripper }

func (s schemeRewriter) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme = "http"
	rt := s.rt
	if rt == nil {
		rt = http.DefaultTransport
	}
	return rt.RoundTrip(r)
}

// The two named limits must stay distinct and in the right order, since the
// whole point is that a page gets more headroom than a control call.
func TestBrclientdLimits(t *testing.T) {
	if brclientdControlRespLimit != 1<<20 {
		t.Errorf("control limit = %d, want 1 MiB", brclientdControlRespLimit)
	}
	// 16 MiB clears the protocol's 1 MiB reply ceiling, and the 10 MiB one a
	// future MaxMsgSizeV1 server would allow, with room for JSON escaping.
	if brclientdPageRespLimit != 16<<20 {
		t.Errorf("page limit = %d, want 16 MiB", brclientdPageRespLimit)
	}
	if brclientdPageRespLimit <= brclientdControlRespLimit {
		t.Error("the page limit must exceed the control limit")
	}
}
