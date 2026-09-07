// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDexBondPostRegistry(t *testing.T) {
	const host = "registry.test:7232"
	if got := DexBondPostState(host); got.Phase != "none" {
		t.Fatalf("unknown host reports %q, want none", got.Phase)
	}
	if !BeginDexBondPost(host) {
		t.Fatal("the first post was refused")
	}
	if BeginDexBondPost(host) {
		t.Fatal("a second post was allowed while the first is in flight")
	}
	if got := DexBondPostState(host); got.Phase != "submitting" {
		t.Fatalf("phase = %q while in flight, want submitting", got.Phase)
	}

	EndDexBondPost(host, errors.New("insufficient funds"))
	if got := DexBondPostState(host); got.Phase != "error" || got.Error != "insufficient funds" {
		t.Fatalf("after a failure: %+v", got)
	}
	if !BeginDexBondPost(host) {
		t.Fatal("a retry after a failure was refused")
	}

	EndDexBondPost(host, nil)
	if got := DexBondPostState(host); got.Phase != "broadcast" || got.Error != "" {
		t.Fatalf("after a broadcast: %+v", got)
	}
	if !BeginDexBondPost(host) {
		t.Fatal("a further post after a broadcast was refused; the guard is not a one-shot")
	}
	EndDexBondPost(host, nil)

	// The guard is per host.
	if !BeginDexBondPost("a.test") || !BeginDexBondPost("b.test") {
		t.Fatal("a post to one host blocked another")
	}
}

// Begin is a check-and-set under one lock: of N racing callers exactly one may
// proceed, or two bonds go out.
func TestDexBondPostBeginIsAtomic(t *testing.T) {
	const host = "atomic.test:7232"
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if BeginDexBondPost(host) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := wins.Load(); got != 1 {
		t.Fatalf("%d callers won the in-flight slot, want exactly 1", got)
	}
}
