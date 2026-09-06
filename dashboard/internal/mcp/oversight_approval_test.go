// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"testing"
	"time"
)

// An operator who freezes or revokes an agent expects that to reach work already
// waiting. The grant is checked before the approval is sent and never again
// after, so without this the operator's later "yes" still lets the spend through.
func TestCancelAgentFailsThatAgentsPendingApprovals(t *testing.T) {
	r := newApprovalRegistry()
	idA, chA, err := r.register("agent-a")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	_, chB, err := r.register("agent-b")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	r.cancelAgent("agent-a")

	select {
	case v := <-chA:
		if !v.cancelled {
			t.Fatalf("want a cancelled verdict, got %+v", v)
		}
		if v.approved {
			t.Fatal("a cancelled approval must never read as approved")
		}
	default:
		t.Fatal("the cancelled agent's approval should have been failed")
	}
	select {
	case v := <-chB:
		t.Fatalf("another agent's approval must be untouched, got %+v", v)
	default:
	}

	// The cancelled entry is gone, so a late reply naming it resolves nothing.
	if r.resolve(idA, approvalVerdict{approved: true}) {
		t.Fatal("a cancelled approval must not still be resolvable")
	}
}

// The id is deliberately short so it can be typed back on a phone. That is only
// safe if an id is never handed out twice: a silently reused id would route the
// operator's reply to a request they were never shown.
func TestRegisterNeverReplacesALiveEntry(t *testing.T) {
	r := newApprovalRegistry()
	seen := map[string]bool{}
	for i := 0; i < maxPendingPerAgent; i++ {
		id, ch, err := r.register("agent-a")
		if err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
		if seen[id] {
			t.Fatalf("id %q was handed out twice", id)
		}
		seen[id] = true
		if ch == nil {
			t.Fatal("register must return a channel")
		}
	}
	if len(r.pending) != maxPendingPerAgent {
		t.Fatalf("pending = %d, want %d: an entry was overwritten", len(r.pending), maxPendingPerAgent)
	}
}

// Each pending approval costs the operator a DM, and holding many open is what
// makes a short id collide in the first place.
func TestRegisterCapsPendingPerAgent(t *testing.T) {
	r := newApprovalRegistry()
	var ids []string
	for i := 0; i < maxPendingPerAgent; i++ {
		id, _, err := r.register("agent-a")
		if err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	if _, _, err := r.register("agent-a"); err != errApprovalPending {
		t.Fatalf("over the cap: err = %v, want errApprovalPending", err)
	}
	// The cap is per agent, not global.
	if _, _, err := r.register("agent-b"); err != nil {
		t.Fatalf("a second agent must still register: %v", err)
	}
	// An abandoned request keeps its slot: the operator was DMed about it and
	// never got to answer, so it still counts against the flood bound.
	r.clear(ids[0])
	if _, _, err := r.register("agent-a"); err != errApprovalPending {
		t.Fatalf("an abandoned request must keep its slot: err = %v", err)
	}

	// An answered one frees its slot at once.
	if !r.resolve(ids[1], approvalVerdict{approved: true}) {
		t.Fatal("resolve of a live id should return true")
	}
	if _, _, err := r.register("agent-a"); err != nil {
		t.Fatalf("an answered request must free its slot: %v", err)
	}
}

// A retired id is reserved only while a reply could still name it.
func TestRetiredIdsAreForgottenAfterTheReplyWindow(t *testing.T) {
	r := newApprovalRegistry()
	base := time.Now()
	r.now = func() time.Time { return base }

	id, _, err := r.register("agent-a")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	r.clear(id)
	if _, ok := r.agentFor(id); !ok {
		t.Fatal("a just-retired id must still name its agent")
	}
	if got := len(r.retired); got != 1 {
		t.Fatalf("retired = %d, want 1", got)
	}

	// Past the window the record is dropped and the slot returns.
	r.now = func() time.Time { return base.Add(approvalTimeout) }
	if _, _, err := r.register("agent-a"); err != nil {
		t.Fatalf("the slot must return after the reply window: %v", err)
	}
	if _, ok := r.agentFor(id); ok {
		t.Fatal("an expired retired id must be forgotten")
	}
}

// The emergency stop must land even when no request is still waiting to hear it:
// a reply arriving after the timeout, or losing the race with it, still names an
// agent the operator wants blocked.
func TestAgentForFindsRetiredRequests(t *testing.T) {
	r := newApprovalRegistry()
	id, _, err := r.register("agent-a")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if got, ok := r.agentFor(id); !ok || got != "agent-a" {
		t.Fatalf("agentFor while pending = (%q,%v), want agent-a", got, ok)
	}

	r.clear(id) // the request gave up
	got, ok := r.agentFor(id)
	if !ok || got != "agent-a" {
		t.Fatalf("agentFor after the request gave up = (%q,%v), want agent-a", got, ok)
	}
	if _, ok := r.agentFor("zzzz"); ok {
		t.Fatal("an unknown id must name no agent")
	}
}

// resolve must still deliver only to the request the operator named.
func TestResolveDeliversOnlyToTheNamedRequest(t *testing.T) {
	r := newApprovalRegistry()
	id1, ch1, _ := r.register("agent-a")
	_, ch2, _ := r.register("agent-a")

	if !r.resolve(id1, approvalVerdict{approved: true}) {
		t.Fatal("resolve of a live id should return true")
	}
	if v := <-ch1; !v.approved || v.cancelled {
		t.Fatalf("wrong verdict delivered: %+v", v)
	}
	select {
	case v := <-ch2:
		t.Fatalf("the other request must stay pending, got %+v", v)
	default:
	}
}

// The wiring that matters: every stop path (freezeAgent, the tripwire, the
// dashboard's revoke, agent teardown) reaches grants.revoke, so cancelling there
// is what makes an operator's stop reach an approval already waiting.
func TestRevokeCancelsPendingApprovals(t *testing.T) {
	const agent = "revoke-wiring-agent"
	id, ch, err := approvals.register(agent)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer approvals.clear(id)

	// revoke returns false with no grant, and must cancel regardless: the agent
	// may have been revoked already and still be waiting on the operator.
	grants.revoke(agent)

	select {
	case v := <-ch:
		if !v.cancelled {
			t.Fatalf("want a cancelled verdict, got %+v", v)
		}
	default:
		t.Fatal("revoking an agent must fail the approval it is waiting on")
	}
}

func TestRevokeAllCancelsEveryPendingApproval(t *testing.T) {
	const a, b = "revoke-all-a", "revoke-all-b"
	idA, chA, err := approvals.register(a)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer approvals.clear(idA)
	idB, chB, err := approvals.register(b)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer approvals.clear(idB)

	grants.revokeAll()

	for name, ch := range map[string]chan approvalVerdict{a: chA, b: chB} {
		select {
		case v := <-ch:
			if !v.cancelled {
				t.Fatalf("%s: want cancelled, got %+v", name, v)
			}
		default:
			t.Fatalf("%s: the freeze-all kill switch must reach a waiting approval", name)
		}
	}
}

// The id is two bytes so it can be typed back on a phone, which is only safe
// because register never hands out one already in use. Random generation almost
// never collides, so the generator is forced here to make the path reachable.
func TestRegisterRetriesPastACollision(t *testing.T) {
	prev := newApprovalID
	t.Cleanup(func() { newApprovalID = prev })

	ids := []string{"aaaa", "aaaa", "aaaa", "bbbb"}
	i := 0
	newApprovalID = func() (string, error) {
		id := ids[min(i, len(ids)-1)]
		i++
		return id, nil
	}

	r := newApprovalRegistry()
	first, ch1, err := r.register("agent-a")
	if err != nil || first != "aaaa" {
		t.Fatalf("first register = (%q, %v), want aaaa", first, err)
	}
	second, ch2, err := r.register("agent-a")
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if second == first {
		t.Fatal("register handed out an id that was already in use")
	}
	if second != "bbbb" {
		t.Fatalf("second register = %q, want bbbb after retrying past the collision", second)
	}

	// Both waiters are live and distinct: the first was not displaced.
	if !r.resolve(first, approvalVerdict{approved: true}) {
		t.Fatal("the first request must still be resolvable")
	}
	if v := <-ch1; !v.approved {
		t.Fatal("the first waiter must get its own verdict")
	}
	select {
	case v := <-ch2:
		t.Fatalf("the second waiter must be untouched, got %+v", v)
	default:
	}
}

// When every generated id is already taken, register must fail rather than
// replace a live entry.
func TestRegisterGivesUpRatherThanReplacing(t *testing.T) {
	prev := newApprovalID
	t.Cleanup(func() { newApprovalID = prev })
	newApprovalID = func() (string, error) { return "cccc", nil }

	r := newApprovalRegistry()
	if _, _, err := r.register("agent-a"); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, _, err := r.register("agent-b"); err != errApprovalUnreachable {
		t.Fatalf("err = %v, want errApprovalUnreachable", err)
	}
	if len(r.pending) != 1 {
		t.Fatalf("pending = %d, want 1: the live entry was replaced", len(r.pending))
	}
}

// The approval DM offers an emergency stop: reply "no <id> freeze" to deny AND
// block the agent. That promise has to hold when the reply arrives after the
// request stopped waiting, which is the common case on a phone: the request
// times out after two minutes and the operator answers a moment later.
func TestLateFreezeStillBlocksTheAgent(t *testing.T) {
	const agent = "late-freeze-agent"
	grants.set(agent, GrantSpec{
		Accounts:    []uint32{0},
		PerTxAtoms:  1,
		DailyAtoms:  1,
		WriteScopes: []string{scopeStaking},
		Expiry:      time.Now().Add(time.Hour),
	}, time.Now())
	if err := grants.precheckGrant(agent, time.Now()); err != nil {
		t.Fatalf("the agent should start with a live grant: %v", err)
	}

	id, _, err := approvals.register(agent)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	approvals.clear(id) // the request gave up waiting

	handleApprovalReply("no " + id + " freeze")

	if err := grants.precheckGrant(agent, time.Now()); err == nil {
		t.Fatal("a late freeze must still revoke the agent's grant")
	}
}

// A late plain denial has nothing left to deny and must not block anyone.
func TestLatePlainReplyDoesNotBlockTheAgent(t *testing.T) {
	const agent = "late-deny-agent"
	grants.set(agent, GrantSpec{
		Accounts:    []uint32{0},
		PerTxAtoms:  1,
		DailyAtoms:  1,
		WriteScopes: []string{scopeStaking},
		Expiry:      time.Now().Add(time.Hour),
	}, time.Now())

	id, _, err := approvals.register(agent)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	approvals.clear(id)

	handleApprovalReply("no " + id)

	if err := grants.precheckGrant(agent, time.Now()); err != nil {
		t.Fatalf("a late denial must not revoke anything: %v", err)
	}
	grants.revoke(agent)
}

// A retired id must not be handed out again while a reply could still name it,
// or that reply would resolve, or freeze on, a request the operator never saw.
func TestRegisterSkipsRetiredIds(t *testing.T) {
	prev := newApprovalID
	t.Cleanup(func() { newApprovalID = prev })

	r := newApprovalRegistry()
	ids := []string{"dddd", "dddd", "eeee"}
	i := 0
	newApprovalID = func() (string, error) {
		id := ids[min(i, len(ids)-1)]
		i++
		return id, nil
	}

	first, _, err := r.register("agent-a")
	if err != nil || first != "dddd" {
		t.Fatalf("first register = (%q,%v), want dddd", first, err)
	}
	r.resolve(first, approvalVerdict{approved: true}) // retires it

	second, _, err := r.register("agent-a")
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if second == first {
		t.Fatalf("register reissued %q while a reply could still name it", first)
	}
	// The retired record must survive, so a late reply still resolves to nobody
	// rather than to the new request.
	if agent, ok := r.agentFor(first); !ok || agent != "agent-a" {
		t.Fatal("the retired record must outlive the reissue attempt")
	}
}

// Editing a grant moves the reservation forward but changes the accounts, scopes
// and caps a pending spend was validated against, so the agent must ask again.
func TestGrantEditCancelsPendingApproval(t *testing.T) {
	const agent = "grant-edit-agent"
	spec := GrantSpec{
		Accounts:    []uint32{0},
		PerTxAtoms:  100,
		DailyAtoms:  100,
		WriteScopes: []string{scopeStaking},
		Expiry:      time.Now().Add(time.Hour),
	}
	grants.set(agent, spec, time.Now())
	t.Cleanup(func() { grants.revoke(agent) })

	id, ch, err := approvals.register(agent)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer approvals.clear(id)

	// Narrow the grant while the approval waits.
	spec.PerTxAtoms = 1
	grants.set(agent, spec, time.Now())

	select {
	case v := <-ch:
		if !v.cancelled {
			t.Fatalf("want cancelled, got %+v", v)
		}
	default:
		t.Fatal("editing a grant must cancel an approval validated against the old one")
	}
}
