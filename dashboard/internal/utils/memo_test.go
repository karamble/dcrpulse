// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import (
	"strconv"
	"sync"
	"testing"
)

func TestMemoGetPutRetain(t *testing.T) {
	var m Memo[string, int]

	if _, ok := m.Get("absent"); ok {
		t.Fatal("the zero value returned a hit")
	}
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)
	if got, ok := m.Get("b"); !ok || got != 2 {
		t.Fatalf("Get(b) = %v,%v want 2,true", got, ok)
	}
	if m.Len() != 3 {
		t.Fatalf("Len = %d, want 3", m.Len())
	}

	// Retain is what bounds the cache; without it nothing is ever released.
	if evicted := m.Retain(map[string]bool{"a": true, "c": true}); evicted != 1 {
		t.Fatalf("Retain evicted %d, want 1", evicted)
	}
	if _, ok := m.Get("b"); ok {
		t.Error("a key outside the keep set survived")
	}
	if _, ok := m.Get("a"); !ok {
		t.Error("a key inside the keep set was dropped")
	}
	if m.Len() != 2 {
		t.Fatalf("Len after retain = %d, want 2", m.Len())
	}

	// A key that was never stored must not be resurrected by keeping it.
	if evicted := m.Retain(map[string]bool{"a": true, "c": true, "zz": true}); evicted != 0 {
		t.Fatalf("a no-op retain evicted %d, want 0", evicted)
	}
	if m.Len() != 2 {
		t.Fatalf("Len = %d, want 2: retain must not add keys", m.Len())
	}

	// Retaining nothing empties it.
	m.Retain(nil)
	if m.Len() != 0 {
		t.Fatalf("Len after retaining nothing = %d, want 0", m.Len())
	}
}

// Both mempool pages poll on their own timers, so reads, writes and prunes
// overlap. Meaningful only under -race.
func TestMemoIsConcurrencySafe(t *testing.T) {
	var m Memo[string, int]
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				k := strconv.Itoa(j % 32)
				m.Put(k, n)
				m.Get(k)
				if j%50 == 0 {
					m.Retain(map[string]bool{"1": true, "2": true})
				}
				m.Len()
			}
		}(i)
	}
	wg.Wait()
}
