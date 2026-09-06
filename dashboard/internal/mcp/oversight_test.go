// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import "testing"

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		in      string
		verdict bool
		ok      bool
	}{
		{"yes", true, true},
		{"y", true, true},
		{"approve", true, true},
		{"ok", true, true},
		{"no", false, true},
		{"n", false, true},
		{"deny", false, true},
		{"cancel", false, true},
		{"maybe", false, false},
		{"", false, false},
	}
	for _, c := range cases {
		v, ok := parseVerdict(c.in)
		if v != c.verdict || ok != c.ok {
			t.Errorf("parseVerdict(%q) = (%v,%v), want (%v,%v)", c.in, v, ok, c.verdict, c.ok)
		}
	}
}

func TestApprovalRegistry(t *testing.T) {
	r := newApprovalRegistry()
	id, ch, err := r.register("agent-a")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !r.resolve(id, approvalVerdict{approved: true}) {
		t.Fatal("resolve of a registered id should return true")
	}
	if v := <-ch; !v.approved {
		t.Fatal("channel should carry the verdict")
	}
	if r.resolve(id, approvalVerdict{}) {
		t.Fatal("resolve of an already-resolved id should return false")
	}
	if r.resolve("nope", approvalVerdict{}) {
		t.Fatal("resolve of an unknown id should return false")
	}
}

func TestHandleApprovalReply(t *testing.T) {
	// "yes <id>" / "no <id>" resolve the named approval (case-insensitive).
	idA, ch, erra := approvals.register("agent-a")
	if erra != nil {
		t.Fatalf("register: %v", erra)
	}
	defer approvals.clear(idA)
	handleApprovalReply("yes " + idA)
	if v := <-ch; !v.approved || v.freeze {
		t.Fatal(`"yes <id>" should approve without freeze`)
	}

	idB, ch2, errb := approvals.register("agent-b")
	if errb != nil {
		t.Fatalf("register: %v", errb)
	}
	defer approvals.clear(idB)
	handleApprovalReply("DENY " + idB)
	if v := <-ch2; v.approved || v.freeze {
		t.Fatal(`"no <id>" should deny without freeze`)
	}

	// "no <id> freeze" denies the spend AND flags an agent freeze.
	idC, chFreeze, errc := approvals.register("agent-c")
	if errc != nil {
		t.Fatalf("register: %v", errc)
	}
	defer approvals.clear(idC)
	handleApprovalReply("no " + idC + " freeze")
	if v := <-chFreeze; v.approved || !v.freeze {
		t.Fatal(`"no <id> freeze" should deny and freeze`)
	}

	// "freeze" forces a deny even if the verb says yes.
	idD, chForce, errd := approvals.register("agent-d")
	if errd != nil {
		t.Fatalf("register: %v", errd)
	}
	defer approvals.clear(idD)
	handleApprovalReply("yes " + idD + " freeze")
	if v := <-chForce; v.approved || !v.freeze {
		t.Fatal("freeze must force a deny")
	}

	// A bare verdict (no id) must NOT resolve anything: the id is required so a
	// stale or late reply cannot resolve a request it does not name.
	idE, ch3, erre := approvals.register("agent-e")
	if erre != nil {
		t.Fatalf("register: %v", erre)
	}
	defer approvals.clear(idE)
	handleApprovalReply("yes")
	assertPending(t, ch3, "bare yes (no id)")

	// A verdict naming a different (unknown) id must not resolve this request.
	idF, ch4, errf := approvals.register("agent-f")
	if errf != nil {
		t.Fatalf("register: %v", errf)
	}
	defer approvals.clear(idF)
	handleApprovalReply("yes zzzz")
	assertPending(t, ch4, "wrong id")

	// A non-verdict reply leaves the approval pending.
	idG, ch5, errg := approvals.register("agent-g")
	if errg != nil {
		t.Fatalf("register: %v", errg)
	}
	defer approvals.clear(idG)
	handleApprovalReply("hello there")
	assertPending(t, ch5, "non-verdict reply")
}

func assertPending(t *testing.T, ch <-chan approvalVerdict, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s must not resolve an approval", what)
	default:
	}
}
