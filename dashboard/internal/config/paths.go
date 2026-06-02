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

	// DefaultWalletName is the directory name of the default wallet. The
	// default wallet lives at the legacy single-wallet appdata path so
	// existing watch-only deployments keep working untouched across the
	// multi-wallet upgrade. Matches Decrediton's literal default.
	DefaultWalletName = "default-wallet"

	// WalletDataRoot is dcrwallet's appdata mount, shared read-write with
	// the dashboard. Matches WALLET_DIR in dcrwallet/docker-entrypoint.sh.
	WalletDataRoot = "/app-data/dcrwallet"
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

// LegacyWalletAppdata is dcrwallet's original single-wallet appdata path,
// where the default wallet's database lives (WalletDataRoot/<network>/wallet.db).
func LegacyWalletAppdata() string {
	return WalletDataRoot
}

// WalletAppdataRoot is the parent directory of every non-default wallet's
// per-wallet appdata directory.
func WalletAppdataRoot() string {
	return filepath.Join(WalletDataRoot, "wallets")
}

// WalletAppdataDir is one non-default wallet's appdata directory.
func WalletAppdataDir(walletName string) string {
	return filepath.Join(WalletAppdataRoot(), walletName)
}

// ResolveWalletAppdata returns the dcrwallet --appdata path for a wallet.
// The default wallet maps to the legacy path; every other wallet maps to its
// directory under WalletAppdataRoot.
func ResolveWalletAppdata(walletName string) string {
	if walletName == DefaultWalletName {
		return LegacyWalletAppdata()
	}
	return WalletAppdataDir(walletName)
}

// WalletDbPath is the wallet.db file dcrwallet creates under an appdata
// directory for the given network.
func WalletDbPath(appdata, network string) string {
	return filepath.Join(appdata, network, "wallet.db")
}

// WalletControlDir is the directory the dashboard and the dcrwallet entrypoint
// supervisor use to coordinate which wallet is loaded.
func WalletControlDir() string {
	return filepath.Join(WalletDataRoot, "control")
}

// SelectedWalletPath is the pointer file the dashboard writes to tell the
// supervisor which wallet to launch.
func SelectedWalletPath() string {
	return filepath.Join(WalletControlDir(), "selected.json")
}

// WalletStatePath is the file the supervisor writes to report the wallet it
// currently has running.
func WalletStatePath() string {
	return filepath.Join(WalletControlDir(), "state.json")
}
