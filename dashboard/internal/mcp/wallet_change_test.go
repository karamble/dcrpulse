// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// describeWalletSend lists tools through scopedServerFor, the cache the HTTP
// handler serves from, rather than buildServer, which would hide a stale entry.
func describeWalletSend(t *testing.T, a *agent) string {
	t.Helper()
	ctx := context.Background()
	srvT, cliT := mcp.NewInMemoryTransports()
	ss, err := scopedServerFor(a).Connect(ctx, srvT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(ctx, cliT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range res.Tools {
		if tl.Name == "wallet_send" {
			return tl.Description
		}
	}
	t.Fatal("wallet_send missing from the listing")
	return ""
}

// A grant records account NUMBERS and a passphrase but names no wallet, so one
// issued against wallet A would otherwise authorise spends from B's
// same-numbered accounts. Asserted on the real store the hook acts on, not a
// private one: a test against its own store would prove nothing about the hook.
func TestWalletChangeRevokesGrantsAndZeroesPassphrases(t *testing.T) {
	const spender, scoped = "wc-spender", "wc-scoped"
	now := time.Now()
	grants.set(spender, GrantSpec{
		Accounts:   []uint32{0, 2},
		PerTxAtoms: 1e8, DailyAtoms: 1e8,
		Passphrase: []byte("wallet-a-passphrase"),
	}, now)
	grants.set(scoped, GrantSpec{WriteScopes: []string{scopeTimestamp}}, now)
	t.Cleanup(func() { grants.revoke(spender); grants.revoke(scoped) })

	// Held before the hook: revoking deletes the record, so the passphrase can
	// only be inspected through a pointer taken while the grant still exists.
	held := grants.byAgent[spender]
	if held == nil {
		t.Fatal("the grant was not installed, so this test would assert nothing")
	}
	if _, ok := grants.info(scoped); !ok {
		t.Fatal("the scope-only grant was not installed")
	}

	onActiveWalletChange(services.ActiveWalletChange{Old: "alpha", New: "beta"})

	if _, ok := grants.info(spender); ok {
		t.Error("the spend grant survived the wallet change; its accounts now name the new wallet's")
	}
	if _, ok := grants.info(scoped); ok {
		t.Error("the scope-only grant survived; its daemons follow the wallet too")
	}
	for i, b := range held.passphrase {
		if b != 0 {
			t.Fatalf("the held passphrase was not zeroed: byte %d is %#x", i, b)
		}
	}
	if _, err := grants.authorize(context.Background(), spender, 0, 1, "addr", time.Now()); err == nil {
		t.Error("account 0 was still authorised after the wallet changed")
	}
	if err := grants.authorizeAction(scoped, scopeTimestamp, time.Now()); err == nil {
		t.Error("a write scope was still authorised after the wallet changed")
	}
}

// Revoking is not enough on its own. Tool descriptions are built once per cached
// server and name the accounts a grant covers, so a bare revokeAll leaves the
// agent being told it may spend from accounts that belong to the old wallet.
func TestWalletChangeRebuildsScopedServers(t *testing.T) {
	const granted, plain = "wc-desc-granted", "wc-desc-plain"
	ga := testAgent(granted, "granted", map[string]bool{"wallet": true})
	pa := testAgent(plain, "plain", map[string]bool{"wallet": true})
	invalidateAgentServer(granted)
	invalidateAgentServer(plain)
	t.Cleanup(func() {
		RevokeSpendGrant(granted)
		invalidateAgentServer(granted)
		invalidateAgentServer(plain)
	})

	SetSpendGrant(granted, GrantSpec{Accounts: []uint32{0, 2}, PerTxAtoms: 1e8, DailyAtoms: 1e8})

	// Warm both caches and prove they hold what we expect, or the assertions
	// after the hook would pass against servers that were never built.
	if got := describeWalletSend(t, ga); !strings.Contains(got, "covers accounts 0, 2") {
		t.Fatalf("the granted description did not warm as expected: %q", got)
	}
	if got := describeWalletSend(t, pa); !strings.Contains(got, "No spend grant") {
		t.Fatalf("the ungranted description did not warm as expected: %q", got)
	}

	onActiveWalletChange(services.ActiveWalletChange{Old: "alpha", New: "beta"})

	serversMu.Lock()
	n := len(servers)
	serversMu.Unlock()
	if n != 0 {
		t.Errorf("%d cached server(s) survived the wallet change; a grantless agent's "+
			"description still carries the old wallet's staking hint", n)
	}
	if got := describeWalletSend(t, ga); !strings.Contains(got, "No spend grant") {
		t.Errorf("the cached server was not rebuilt; the description still reads: %q", got)
	}
}

func TestWalletChangeDropsStakingProfileCache(t *testing.T) {
	profileCache.mu.Lock()
	profileCache.have = true
	profileCache.at = time.Now()
	profileCache.prof = types.StakingProfile{Accounts: []uint32{7}}
	profileCache.mu.Unlock()
	t.Cleanup(func() {
		profileCache.mu.Lock()
		profileCache.have = false
		profileCache.mu.Unlock()
	})

	onActiveWalletChange(services.ActiveWalletChange{Old: "alpha", New: "beta"})

	profileCache.mu.Lock()
	have := profileCache.have
	profileCache.mu.Unlock()
	if have {
		t.Error("the staking profile of the old wallet survived the change; it names that wallet's accounts and VSPs")
	}
}

// The feeds behind these rings are per-wallet daemons, so their buffered
// history describes another wallet's messages, channels, tickets and mixes.
func TestWalletChangeClearsEventRings(t *testing.T) {
	rings := []struct {
		name string
		ring *eventRing
	}{
		{"bisonrelay", brRing},
		{"lightning", lnRing},
		{"staking", stakingRing},
		{"mixer", mixerRing},
	}
	for _, r := range rings {
		r.ring.add("event from the old wallet")
		if len(r.ring.snapshot()) == 0 {
			t.Fatalf("the %s ring did not take the entry, so this test would assert nothing", r.name)
		}
		t.Cleanup(r.ring.reset)
	}

	onActiveWalletChange(services.ActiveWalletChange{Old: "alpha", New: "beta"})

	for _, r := range rings {
		if got := len(r.ring.snapshot()); got != 0 {
			t.Errorf("the %s ring still holds %d entr(ies) from the old wallet", r.name, got)
		}
	}
}

// The unit tests above call the callback directly, which proves nothing about
// whether a real wallet change reaches it. This one goes through the services
// seam, so it fails if the registration is dropped.
func TestActiveWalletSwitchReachesGrants(t *testing.T) {
	for _, p := range []string{"/dashboard-data", "/app-data"} {
		if _, err := os.Stat(p); err == nil {
			t.Skipf("%s exists: SetActiveWallet would write to a real deployment", p)
		}
	}

	const agentID = "wc-endtoend"
	grants.set(agentID, GrantSpec{
		Accounts:   []uint32{0},
		PerTxAtoms: 1e8, DailyAtoms: 1e8,
		Passphrase: []byte("wallet-a-passphrase"),
	}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	held := grants.byAgent[agentID]
	if held == nil {
		t.Fatal("the grant was not installed, so this test would assert nothing")
	}

	prev := services.ActiveWalletName()
	t.Cleanup(func() {
		if prev == "" {
			_ = services.ClearActiveWallet()
			return
		}
		_ = services.SetActiveWallet(prev, "mainnet")
	})

	// The persist step fails with no data mount; the hook has already fired by
	// then, which is the ordering this asserts.
	_ = services.SetActiveWallet("wc-endtoend-wallet", "mainnet")

	if _, ok := grants.info(agentID); ok {
		t.Fatal("a real wallet change left the grant standing; the change hook is not wired up")
	}
	for i, b := range held.passphrase {
		if b != 0 {
			t.Fatalf("a real wallet change left the passphrase in memory: byte %d is %#x", i, b)
		}
	}
}
