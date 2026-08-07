// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A batch ballot carries one signed vote per eligible ticket, so its size scales
// with the wallet. It used to be capped at 30s twice over — by piPost's own
// context and by the shared client's Timeout, which bounds the whole
// request-response cycle and overrides a longer context. A cut mid-cast records
// no receipts locally while Politeia may have accepted the votes, and a retry
// cannot repair it because duplicates come back as skips.

// piTestServer stands up a server that delays before replying, and points
// piHTTPClient at it by rewriting the request URL. Nothing in the non-test code
// has to change for this.
func piTestServer(t *testing.T, delay time.Duration) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	prev := piHTTPClient
	piHTTPClient = &http.Client{Transport: piRedirect{to: srv.URL, rt: srv.Client().Transport}}
	t.Cleanup(func() { piHTTPClient = prev })
}

type piRedirect struct {
	to string
	rt http.RoundTripper
}

func (p piRedirect) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	u, err := http.NewRequest(r.Method, p.to+r.URL.Path, nil)
	if err != nil {
		return nil, err
	}
	r.URL.Scheme, r.URL.Host = u.URL.Scheme, u.URL.Host
	rt := p.rt
	if rt == nil {
		rt = http.DefaultTransport
	}
	return rt.RoundTrip(r)
}

func TestPiPostHonoursItsTimeout(t *testing.T) {
	piTestServer(t, 300*time.Millisecond)

	var out struct{ OK bool }
	err := piPost(context.Background(), "/slow", struct{}{}, &out, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected the short per-call timeout to cut the request")
	}
}

// Timeout 0 is what the batch ballot passes: the caller's context is the bound.
// This is the case the shared client's Timeout used to defeat.
func TestPiPostZeroTimeoutUsesCallerContext(t *testing.T) {
	piTestServer(t, 300*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out struct{ OK bool }
	if err := piPost(ctx, "/slow", struct{}{}, &out, 0); err != nil {
		t.Fatalf("a generous caller context should have allowed this: %v", err)
	}
	if !out.OK {
		t.Error("response was not decoded")
	}
}

// "Inherit", not "unbounded": a caller deadline still cuts a zero-timeout call.
func TestPiPostZeroTimeoutStillObeysContextDeadline(t *testing.T) {
	piTestServer(t, 500*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var out struct{ OK bool }
	if err := piPost(ctx, "/slow", struct{}{}, &out, 0); err == nil {
		t.Fatal("the caller's deadline should still bound a zero-timeout call")
	}
}

// The regression that mattered: the shared client must not carry a Timeout of
// its own, or a ballot could never outlive it however long the handler allows.
func TestPiHTTPClientHasNoOwnTimeout(t *testing.T) {
	if piHTTPClient.Timeout != 0 {
		t.Errorf("piHTTPClient.Timeout = %s, want 0; a client timeout overrides "+
			"a longer context and would re-cap the ballot", piHTTPClient.Timeout)
	}
}
