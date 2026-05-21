// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package config mirrors Decrediton's per-wallet config storage. Paths,
// key names, and on-disk JSON shape match Decrediton's electron-store
// layout (decrediton/app/main_dev/paths.js,
// decrediton/app/constants/config.js, decrediton/app/wallet/config.js)
// so a user's config.json can shuttle between the two tools.
package config

import "path/filepath"

const (
	// AppDataDir is the writable mount inside the dashboard container.
	// In docker-compose this is the dashboard-data named volume; in
	// Umbrel it bind-mounts to ${APP_DATA_DIR}/dashboard.
	AppDataDir = "/dashboard-data"

	// DefaultWalletName is the wallet directory name used while
	// dcrpulse is single-wallet. Matches Decrediton's literal default.
	DefaultWalletName = "default-wallet"
)

// GlobalCfgPath is the on-disk location of the cross-wallet config.
// Reserved for future use (active wallet, currency display, etc.).
func GlobalCfgPath() string {
	return filepath.Join(AppDataDir, "config.json")
}

// WalletsDir is the directory containing every wallet's config dir for
// the given network ("mainnet" or "testnet").
func WalletsDir(network string) string {
	return filepath.Join(AppDataDir, "wallets", network)
}

// WalletDir is one wallet's directory.
func WalletDir(network, walletName string) string {
	return filepath.Join(WalletsDir(network), walletName)
}

// WalletCfgPath is the per-wallet config.json file.
func WalletCfgPath(network, walletName string) string {
	return filepath.Join(WalletDir(network, walletName), "config.json")
}
