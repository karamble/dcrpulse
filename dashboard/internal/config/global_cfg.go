// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"sync"
)

// GlobalCfg is the cross-wallet config document. Reserved for future use
// (active wallet name, currency display preference, etc.); Phase 1 ships
// the load/save scaffolding but no keys are written yet.
type GlobalCfg struct {
	mu    sync.Mutex
	path  string
	raw   map[string]json.RawMessage
	dirty map[string]bool // keys this instance wrote, flushed by Save
}

// LoadGlobalCfg reads /dashboard-data/config.json. Absent file → empty doc.
func LoadGlobalCfg() (*GlobalCfg, error) {
	return loadGlobalCfgAt(GlobalCfgPath())
}

func loadGlobalCfgAt(path string) (*GlobalCfg, error) {
	raw, err := readRawJSON(path)
	if err != nil {
		return nil, err
	}
	return &GlobalCfg{path: path, raw: raw, dirty: map[string]bool{}}, nil
}

// Get / Set / Has follow the same convention as WalletCfg.
func (c *GlobalCfg) Get(key string, out any) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.raw[key]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(v, out); err != nil {
		return true, err
	}
	return true, nil
}

func (c *GlobalCfg) Set(key string, value any) error {
	enc, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.raw[key] = enc
	c.dirty[key] = true
	return nil
}

func (c *GlobalCfg) Has(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.raw[key]
	return ok
}

// AllowedExternalRequests returns the persisted allowlist map. Absent
// keys are treated as allowed by callers; this preserves backward
// compatibility when the file does not yet exist.
func (c *GlobalCfg) AllowedExternalRequests() (map[string]bool, error) {
	m := map[string]bool{}
	ok, err := c.Get(KeyAllowedExternalRequests, &m)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return m, nil
}

// SetAllowedExternalRequests stages the full allowlist map.
func (c *GlobalCfg) SetAllowedExternalRequests(m map[string]bool) error {
	return c.Set(KeyAllowedExternalRequests, m)
}

// Save merges this instance's writes into the global config on disk and
// rewrites it atomically. Keys another writer added since Load are kept.
func (c *GlobalCfg) Save() error {
	cfgWriteMu.Lock()
	defer cfgWriteMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	// No entry staging: the global document has no incrementally merged maps.
	merged, err := mergeSave(c.path, c.raw, c.dirty, nil)
	if err != nil {
		return err
	}
	c.raw = merged
	c.dirty = map[string]bool{}
	return nil
}
