// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dcrpulse/pkg/bisonw"
)

// resetDexCfgCache clears the package-level DEX config cache around a test.
func resetDexCfgCache(t *testing.T) {
	reset := func() {
		dexCfgMu.Lock()
		dexCfgCache = map[string]*dexCfgEntry{}
		dexCfgMu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

// dexCfgTestServer runs a TLS server answering the bisonw RPC envelope and
// returns a client pinned to its certificate plus a request counter. hook, if
// non-nil, runs inside each request before the response is written.
func dexCfgTestServer(t *testing.T, hook func()) (*bisonw.Client, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if hook != nil {
			hook()
		}
		json.NewEncoder(w).Encode(map[string]any{
			"type":    2,
			"payload": map[string]any{"result": map[string]any{"host": "stub"}},
		})
	}))
	t.Cleanup(srv.Close)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	client, err := bisonw.New(bisonw.Config{
		Addr: strings.TrimPrefix(srv.URL, "https://"),
		User: "user",
		Pass: "pass",
		Cert: certPEM,
	})
	if err != nil {
		t.Fatalf("bisonw.New: %v", err)
	}
	return client, &calls
}

// A caller that cancels mid-fetch must not poison the shared entry: the
// detached fetch completes anyway and followers get the real config.
func TestCachedDEXConfigSurvivesCallerCancel(t *testing.T) {
	resetDexCfgCache(t)
	reqStarted := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	client, calls := dexCfgTestServer(t, func() {
		once.Do(func() { close(reqStarted) })
		<-release
	})

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	type result struct {
		raw json.RawMessage
		err error
	}
	leaderDone := make(chan result, 1)
	go func() {
		raw, err := cachedDEXConfig(leaderCtx, client, "host1")
		leaderDone <- result{raw, err}
	}()

	<-reqStarted
	cancelLeader()
	close(release)

	if lr := <-leaderDone; lr.err != nil {
		t.Fatalf("leader after own cancel: %v", lr.err)
	}
	raw, err := cachedDEXConfig(context.Background(), client, "host1")
	if err != nil {
		t.Fatalf("follower: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("follower: empty config")
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("server calls = %d, want 1", n)
	}
}

// Concurrent callers for one host coalesce into a single upstream fetch.
func TestCachedDEXConfigCoalesces(t *testing.T) {
	resetDexCfgCache(t)
	client, calls := dexCfgTestServer(t, func() { time.Sleep(100 * time.Millisecond) })

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cachedDEXConfig(context.Background(), client, "host1")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("caller: %v", err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("server calls = %d, want 1", n)
	}
}

// A fresh success is served from cache and a fresh error is served without a
// refetch.
func TestCachedDEXConfigTTLs(t *testing.T) {
	resetDexCfgCache(t)
	client, calls := dexCfgTestServer(t, nil)

	if _, err := cachedDEXConfig(context.Background(), client, "host1"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := cachedDEXConfig(context.Background(), client, "host1"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("server calls = %d, want 1", n)
	}

	seeded := errors.New("seeded failure")
	dexCfgMu.Lock()
	dexCfgCache["host2"] = &dexCfgEntry{err: seeded, at: time.Now()}
	dexCfgMu.Unlock()
	if _, err := cachedDEXConfig(context.Background(), client, "host2"); !errors.Is(err, seeded) {
		t.Fatalf("negative cache: got %v, want seeded error", err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("server calls = %d, want 1 (no refetch for cached error)", n)
	}
}
