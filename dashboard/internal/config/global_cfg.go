// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// GlobalCfg is the cross-wallet config document. Reserved for future use
// (active wallet name, currency display preference, etc.); Phase 1 ships
// the load/save scaffolding but no keys are written yet.
type GlobalCfg struct {
	mu  sync.Mutex
	raw map[string]json.RawMessage
}

// LoadGlobalCfg reads /dashboard-data/config.json. Absent file → empty doc.
func LoadGlobalCfg() (*GlobalCfg, error) {
	c := &GlobalCfg{raw: map[string]json.RawMessage{}}
	data, err := os.ReadFile(GlobalCfgPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		return nil, fmt.Errorf("read global config: %w", err)
	}
	if len(data) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(data, &c.raw); err != nil {
		return nil, fmt.Errorf("parse global config: %w", err)
	}
	return c, nil
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

// Save atomically rewrites the global config.
func (c *GlobalCfg) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(GlobalCfgPath()), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.raw, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteJSON(GlobalCfgPath(), data)
}
