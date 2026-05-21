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

// WalletCfg is one wallet's electron-store-shaped JSON document, held as
// a map of raw JSON values so unknown keys (Decrediton internals such as
// theme, dex_account, etc.) round-trip unchanged on save.
type WalletCfg struct {
	mu   sync.Mutex
	path string
	raw  map[string]json.RawMessage
}

// LoadWalletCfg reads the per-wallet config.json. An absent or empty
// file yields an empty config; the file is created lazily on first Save.
func LoadWalletCfg(network, walletName string) (*WalletCfg, error) {
	p := WalletCfgPath(network, walletName)
	c := &WalletCfg{path: p, raw: map[string]json.RawMessage{}}

	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		return nil, fmt.Errorf("read wallet config %s: %w", p, err)
	}
	if len(data) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(data, &c.raw); err != nil {
		return nil, fmt.Errorf("parse wallet config %s: %w", p, err)
	}
	return c, nil
}

// Path returns the on-disk file path.
func (c *WalletCfg) Path() string { return c.path }

// Has reports whether key is present in the document.
func (c *WalletCfg) Has(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.raw[key]
	return ok
}

// Get decodes key into out. Returns (false, nil) when the key is absent.
func (c *WalletCfg) Get(key string, out any) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.raw[key]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(v, out); err != nil {
		return true, fmt.Errorf("decode %q: %w", key, err)
	}
	return true, nil
}

// Set marshals value and stages it in memory. Call Save to flush.
func (c *WalletCfg) Set(key string, value any) error {
	enc, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %q: %w", key, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.raw[key] = enc
	return nil
}

// Delete removes key from the in-memory document. Survives until Save.
func (c *WalletCfg) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.raw, key)
}

// Save atomically rewrites the entire document via temp file + rename.
// Unknown keys preserved by Load are kept verbatim.
func (c *WalletCfg) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(c.raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encode wallet config: %w", err)
	}
	return atomicWriteJSON(c.path, data)
}

// atomicWriteJSON writes data to path via temp file + rename, mode 0600.
func atomicWriteJSON(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// ---- Typed accessors (autobuyer + VSP) ------------------------------------

// AutobuyerSettings is the on-disk shape Decrediton writes under the
// autobuyer_settings key. balanceToMaintain is in atoms (int64).
type AutobuyerSettings struct {
	BalanceToMaintain int64  `json:"balanceToMaintain"`
	Account           string `json:"account"`
	MaxFeePercentage  int    `json:"maxFeePercentage"`
}

// VSPMetadata is one entry inside the used_vsps map.
type VSPMetadata struct {
	Host     string `json:"host"`
	Pubkey   string `json:"pubkey"`
	Label    string `json:"label,omitempty"`
	LastUsed int64  `json:"lastUsed,omitempty"`
}

// AutobuyerSettings returns the autobuyer_settings entry, or nil if absent.
func (c *WalletCfg) AutobuyerSettings() (*AutobuyerSettings, error) {
	var s AutobuyerSettings
	ok, err := c.Get(KeyAutobuyerSettings, &s)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return &s, nil
}

// SetAutobuyerSettings stages the autobuyer_settings entry.
func (c *WalletCfg) SetAutobuyerSettings(s *AutobuyerSettings) error {
	return c.Set(KeyAutobuyerSettings, s)
}

// RememberedVSPHost returns the remembered_vsp_host string, or "" if absent.
func (c *WalletCfg) RememberedVSPHost() string {
	var s string
	_, _ = c.Get(KeyRememberedVSPHost, &s)
	return s
}

// SetRememberedVSPHost stages remembered_vsp_host.
func (c *WalletCfg) SetRememberedVSPHost(host string) error {
	return c.Set(KeyRememberedVSPHost, host)
}

// UsedVSPs returns the used_vsps map (may be nil if absent).
func (c *WalletCfg) UsedVSPs() (map[string]VSPMetadata, error) {
	m := map[string]VSPMetadata{}
	ok, err := c.Get(KeyUsedVSPs, &m)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return m, nil
}

// UpsertUsedVSP merges meta into the used_vsps map under meta.Host.
func (c *WalletCfg) UpsertUsedVSP(meta VSPMetadata) error {
	m, err := c.UsedVSPs()
	if err != nil {
		return err
	}
	if m == nil {
		m = map[string]VSPMetadata{}
	}
	m[meta.Host] = meta
	return c.Set(KeyUsedVSPs, m)
}

// PoliteiaVotes returns the per-wallet cache of Politeia vote choices.
// Map keyed by proposal token; value is "yes" | "no" | "abstain".
func (c *WalletCfg) PoliteiaVotes() (map[string]string, error) {
	m := map[string]string{}
	ok, err := c.Get(KeyPoliteiaVotes, &m)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return m, nil
}

// UpsertPoliteiaVote records the vote choice for a proposal token.
func (c *WalletCfg) UpsertPoliteiaVote(token, choice string) error {
	m, err := c.PoliteiaVotes()
	if err != nil {
		return err
	}
	if m == nil {
		m = map[string]string{}
	}
	m[token] = choice
	return c.Set(KeyPoliteiaVotes, m)
}

// SetLastAccess records a unix-seconds timestamp under last_access.
func (c *WalletCfg) SetLastAccess(ts int64) error {
	return c.Set(KeyLastAccess, ts)
}

// LastAccess returns the last_access timestamp, or 0 if absent.
func (c *WalletCfg) LastAccess() int64 {
	var v int64
	_, _ = c.Get(KeyLastAccess, &v)
	return v
}
