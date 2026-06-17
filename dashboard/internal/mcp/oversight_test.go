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
	// resolveSingle only fires when exactly one approval is pending.
	r.register("only")
	if !r.resolveSingle(false) {
		t.Fatal("resolveSingle with one pending should succeed")
	}
	r.register("x")
	r.register("y")
	if r.resolveSingle(true) {
		t.Fatal("resolveSingle with two pending should fail")
	}
}

func TestHandleApprovalReply(t *testing.T) {
	// handleApprovalReply resolves against the package-global registry.
	ch := approvals.register("ab12")
	defer approvals.clear("ab12")
	handleApprovalReply("yes ab12")
	if v := <-ch; !v {
		t.Fatal(`"yes <id>" should approve`)
	}

	ch2 := approvals.register("cd34")
	defer approvals.clear("cd34")
	handleApprovalReply("DENY cd34") // case-insensitive
	if v := <-ch2; v {
		t.Fatal(`"deny <id>" should deny`)
	}

	// A bare verdict resolves the sole pending approval.
	ch3 := approvals.register("ef56")
	defer approvals.clear("ef56")
	handleApprovalReply("yes")
	if v := <-ch3; !v {
		t.Fatal("bare yes should approve the single pending approval")
	}

	// A non-verdict reply leaves the approval pending.
	ch4 := approvals.register("gh78")
	defer approvals.clear("gh78")
	handleApprovalReply("hello there")
	select {
	case <-ch4:
		t.Fatal("a non-verdict reply must not resolve an approval")
	default:
	}
}
