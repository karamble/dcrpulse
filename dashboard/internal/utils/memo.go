// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import "sync"

// Memo is a keyed cache for values that cannot change while their key is live:
// a mempool transaction by hash, a node alias by pubkey. It holds entries until
// Retain drops the ones whose keys are gone, so it is for callers that can name
// the live set. Anything needing expiry, negative caching or single-flight wants
// its own type, not this one.
//
// The zero value is ready to use and safe for concurrent use.
type Memo[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]V
}

// Get returns the stored value for k, if there is one.
func (m *Memo[K, V]) Get(k K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.entries[k]
	return v, ok
}

// Put stores v under k, replacing any previous value.
func (m *Memo[K, V]) Put(k K, v V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = make(map[K]V)
	}
	m.entries[k] = v
}

// Retain drops every entry whose key is not in keep, and reports how many went.
// This is what bounds the cache: without it entries accumulate for the lifetime
// of the process.
func (m *Memo[K, V]) Retain(keep map[K]bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	evicted := 0
	for k := range m.entries {
		if !keep[k] {
			delete(m.entries, k)
			evicted++
		}
	}
	return evicted
}

// RetainWhere drops every entry keep rejects, for callers whose live set is a
// property of the value (a block's height) rather than a list of keys.
func (m *Memo[K, V]) RetainWhere(keep func(K, V) bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	evicted := 0
	for k, v := range m.entries {
		if !keep(k, v) {
			delete(m.entries, k)
			evicted++
		}
	}
	return evicted
}

// Len reports how many entries are held.
func (m *Memo[K, V]) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}
