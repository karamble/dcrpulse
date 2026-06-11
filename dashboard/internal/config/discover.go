// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"errors"
	"io/fs"
	"os"
)

// WalletEntry is one discovered wallet on disk.
type WalletEntry struct {
	Name        string `json:"name"`
	Network     string `json:"network"`
	LastAccess  int64  `json:"lastAccess,omitempty"`
	IsWatchOnly bool   `json:"isWatchOnly,omitempty"`
}

// GetAvailableWallets scans WalletsDir(network) and returns one entry
// per subdirectory. Mirrors Decrediton's getAvailableWallets.
func GetAvailableWallets(network string) ([]WalletEntry, error) {
	entries, err := os.ReadDir(WalletsDir(network))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]WalletEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		w := WalletEntry{Name: e.Name(), Network: network}
		if cfg, err := LoadWalletCfg(network, e.Name()); err == nil {
			w.LastAccess = cfg.LastAccess()
			_, _ = cfg.Get(KeyIsWatchOnly, &w.IsWatchOnly)
		}
		out = append(out, w)
	}
	return out, nil
}
