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
	ch := r.register("ab12")
	if !r.resolve("ab12", true) {
		t.Fatal("resolve of a registered id should return true")
	}
	if v := <-ch; !v {
		t.Fatal("channel should carry the verdict")
	}
	if r.resolve("ab12", true) {
		t.Fatal("resolve of an already-resolved id should return false")
	}
	if r.resolve("nope", false) {
		t.Fatal("resolve of an unknown id should return false")
	}
}

func TestHandleApprovalReply(t *testing.T) {
	// "yes <id>" / "no <id>" resolve the named approval (case-insensitive).
	ch := approvals.register("ab12")
	defer approvals.clear("ab12")
	handleApprovalReply("yes ab12")
	if v := <-ch; !v {
		t.Fatal(`"yes <id>" should approve`)
	}

	ch2 := approvals.register("cd34")
	defer approvals.clear("cd34")
	handleApprovalReply("DENY cd34")
	if v := <-ch2; v {
		t.Fatal(`"deny <id>" should deny`)
	}

	// A bare verdict (no id) must NOT resolve anything: the id is required so a
	// stale or late reply cannot resolve a request it does not name.
	ch3 := approvals.register("ef56")
	defer approvals.clear("ef56")
	handleApprovalReply("yes")
	assertPending(t, ch3, "bare yes (no id)")

	// A verdict naming a different (unknown) id must not resolve this request.
	ch4 := approvals.register("gh78")
	defer approvals.clear("gh78")
	handleApprovalReply("yes zzzz")
	assertPending(t, ch4, "wrong id")

	// A non-verdict reply leaves the approval pending.
	ch5 := approvals.register("ij90")
	defer approvals.clear("ij90")
	handleApprovalReply("hello there")
	assertPending(t, ch5, "non-verdict reply")
}

func assertPending(t *testing.T, ch <-chan bool, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s must not resolve an approval", what)
	default:
	}
}
