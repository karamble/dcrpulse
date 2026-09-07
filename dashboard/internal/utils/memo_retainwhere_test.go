// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import "testing"

func TestMemoRetainWhere(t *testing.T) {
	var m Memo[string, int]
	m.Put("a", 1)
	m.Put("b", 5)
	m.Put("c", 9)
	if got := m.RetainWhere(func(_ string, v int) bool { return v >= 5 }); got != 1 {
		t.Fatalf("evicted %d, want 1", got)
	}
	if _, ok := m.Get("a"); ok {
		t.Fatal("an entry the predicate rejected survived")
	}
	if _, ok := m.Get("b"); !ok {
		t.Fatal("an entry the predicate accepted was dropped")
	}
	if m.Len() != 2 {
		t.Fatalf("len = %d, want 2", m.Len())
	}
}
