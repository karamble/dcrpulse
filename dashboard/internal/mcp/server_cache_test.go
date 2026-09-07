// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/types"
)

// An agent's server is built on its first request, and the staking tool
// descriptions inside that build ask the wallet. These tests park that read to
// stand in for a wedged wallet and watch what the shared lock does meanwhile.

// parkedWallet replaces the profile read with one that reports when a build has
// reached it and then waits to be released. The cache is cleared first, or the
// read would be skipped.
type parkedWallet struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func parkWallet(t *testing.T) *parkedWallet {
	t.Helper()
	p := &parkedWallet{entered: make(chan struct{}, 8), release: make(chan struct{})}
	prev := fetchStakingProfile
	fetchStakingProfile = func(context.Context) types.StakingProfile {
		p.entered <- struct{}{}
		<-p.release
		return types.StakingProfile{}
	}
	InvalidateStakingProfile()
	t.Cleanup(func() {
		p.let()
		fetchStakingProfile = prev
		InvalidateStakingProfile()
	})
	return p
}

func (p *parkedWallet) let() { p.once.Do(func() { close(p.release) }) }

// parked blocks until a build has reached the wallet read.
func (p *parkedWallet) parked(t *testing.T) {
	t.Helper()
	select {
	case <-p.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("no build reached the wallet read")
	}
}

func stakingAgent(t *testing.T, id string) *agent {
	t.Helper()
	t.Cleanup(func() { invalidateAgentServer(id) })
	return testAgent(id, id, map[string]bool{"staking": true})
}

func buildInBackground(a *agent) <-chan *mcp.Server {
	ch := make(chan *mcp.Server, 1)
	go func() { ch <- scopedServerFor(a) }()
	return ch
}

func await(t *testing.T, ch <-chan *mcp.Server) *mcp.Server {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("the build never finished")
		return nil
	}
}

// Block and freeze invalidate an agent's server under the shared lock. They
// must not queue behind a build that is waiting on the wallet.
func TestServerBuildDoesNotHoldTheServersLock(t *testing.T) {
	// The agent before the wallet: cleanups run last-in first-out, and the
	// agent's invalidation must not run while the wallet is still parked.
	a := stakingAgent(t, "slow-build")
	w := parkWallet(t)
	built := buildInBackground(a)
	w.parked(t)

	done := make(chan struct{})
	go func() {
		invalidateAgentServer("someone-else")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		w.let()
		t.Fatal("invalidating another agent waited on a build that is waiting on the wallet")
	}
	w.let()
	await(t, built)
}

// Listen streams attach to one server, so two callers who built the same
// agent's server at once must end up sharing one.
func TestConcurrentBuildsShareOneServer(t *testing.T) {
	a := stakingAgent(t, "double-build")
	w := parkWallet(t)
	first := buildInBackground(a)
	w.parked(t)
	second := buildInBackground(a)
	// The second build queues on the profile cache's own lock; give it time
	// to get there so both are in flight before the wallet answers.
	time.Sleep(50 * time.Millisecond)
	w.let()

	s1, s2 := await(t, first), await(t, second)
	if s1 != s2 {
		t.Fatal("two callers got two servers for one agent")
	}
	if got := scopedServerFor(a); got != s1 {
		t.Fatal("the cached server is neither of the ones handed out")
	}
}

// An invalidation that lands while a build is parked on the wallet must not be
// undone when that build finishes: what it built may reflect the old grant.
func TestInvalidationDuringABuildIsNotLost(t *testing.T) {
	a := stakingAgent(t, "stale-build")
	w := parkWallet(t)
	parked := buildInBackground(a)
	w.parked(t)
	// In a goroutine with a deadline, so a build that (wrongly) holds the
	// lock fails this test instead of deadlocking it.
	done := make(chan struct{})
	go func() {
		invalidateAgentServer(a.id)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		w.let()
		t.Fatal("invalidating the agent waited on its own parked build")
	}
	w.let()

	stale := await(t, parked)
	if fresh := scopedServerFor(a); fresh == stale {
		t.Fatal("a build that predates the invalidation was installed as the agent's server")
	}
}
