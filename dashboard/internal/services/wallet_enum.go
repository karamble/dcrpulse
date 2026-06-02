// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"sort"

	"dcrpulse/internal/config"
)

// WalletInfo describes one wallet available on disk. Mirrors Decrediton's
// getAvailableWallets entry shape.
type WalletInfo struct {
	Name        string `json:"name"`
	Network     string `json:"network"`
	HasDB       bool   `json:"hasDb"`
	IsDefault   bool   `json:"isDefault"`
	IsWatchOnly bool   `json:"isWatchOnly"`
	IsPrivacy   bool   `json:"isPrivacy"`
	LastAccess  int64  `json:"lastAccess,omitempty"`
	Active      bool   `json:"active"`
}

// ListWallets enumerates every wallet dcrwallet can load: the default wallet at
// the legacy appdata path (so upgraded watch-only deployments see their existing
// wallet) plus each per-wallet directory under WalletAppdataRoot. Each entry is
// enriched with the dashboard-side per-wallet config and marked active when it
// matches the currently selected wallet.
func ListWallets(ctx context.Context) ([]WalletInfo, error) {
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return nil, err
	}
	active := ActiveWalletName()

	var out []WalletInfo
	seen := map[string]bool{}

	add := func(name string, isDefault bool) {
		if seen[name] {
			return
		}
		seen[name] = true
		appdata := config.ResolveWalletAppdata(name)
		info := WalletInfo{
			Name:      name,
			Network:   network,
			IsDefault: isDefault,
			HasDB:     fileExists(config.WalletDbPath(appdata, network)),
			Active:    name == active,
		}
		if cfg, err := config.LoadWalletCfg(network, name); err == nil {
			info.LastAccess = cfg.LastAccess()
			_, _ = cfg.Get(config.KeyIsWatchOnly, &info.IsWatchOnly)
			_, _ = cfg.Get(config.KeyEnablePrivacy, &info.IsPrivacy)
		}
		out = append(out, info)
	}

	// Legacy default wallet, present iff its database exists in place.
	if fileExists(config.WalletDbPath(config.LegacyWalletAppdata(), network)) {
		add(config.DefaultWalletName, true)
	}

	// Per-wallet directories under the new layout.
	entries, err := os.ReadDir(config.WalletAppdataRoot())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		add(e.Name(), e.Name() == config.DefaultWalletName)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].LastAccess != out[j].LastAccess {
			return out[i].LastAccess > out[j].LastAccess
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// walletExistsByName reports whether a wallet with the given name has a
// database on disk for the given network.
func walletExistsByName(name, network string) bool {
	return fileExists(config.WalletDbPath(config.ResolveWalletAppdata(name), network))
}
