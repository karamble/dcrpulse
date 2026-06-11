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

	// AppDataRoot is the shared services volume, mounted read-write in the
	// dashboard. Holds each daemon's data plus the stack control directory.
	AppDataRoot = "/app-data"

	// WalletDataRoot is dcrwallet's appdata mount, shared read-write with
	// the dashboard. Matches WALLET_DIR in dcrwallet/docker-entrypoint.sh.
	WalletDataRoot = "/app-data/dcrwallet"

	// DcrlndDataRoot / BrclientdDataRoot / DcrdexDataRoot are each service's
	// data root as seen from the dashboard container. The default wallet uses
	// these legacy paths; named wallets live under <root>/wallets/<name>. (Inside
	// the dcrdex container the same tree is mounted at /dex/.dexc.)
	DcrlndDataRoot    = "/app-data/dcrlnd"
	BrclientdDataRoot = "/app-data/brclientd"
	DcrdexDataRoot    = "/app-data/dcrdex"
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

// StackControlDir is the shared control directory every service supervisor
// reads the selected wallet from. The dashboard writes it (it mounts /app-data
// read-write); the daemons mount it read-only.
func StackControlDir() string {
	return filepath.Join(AppDataRoot, "control")
}

// SelectedWalletPath is the single pointer file the dashboard writes to tell
// every service supervisor which wallet to run.
func SelectedWalletPath() string {
	return filepath.Join(StackControlDir(), "selected.json")
}

// Per-service state files. Each supervisor writes the wallet it currently has
// running to a fixed (not per-wallet) path so the dashboard can poll it during a
// switch. dcrwallet keeps its original control/state.json location.
func WalletStatePath() string    { return filepath.Join(WalletDataRoot, "control", "state.json") }
func DcrlndStatePath() string    { return filepath.Join(DcrlndDataRoot, "control-state.json") }
func BrclientdStatePath() string { return filepath.Join(BrclientdDataRoot, "control-state.json") }
func DcrdexStatePath() string    { return filepath.Join(DcrdexDataRoot, "control-state.json") }

// ResolveServiceDir returns a service's per-wallet data directory: the legacy
// root for the default wallet, root/wallets/<name> for any other wallet.
func ResolveServiceDir(root, walletName string) string {
	if walletName == DefaultWalletName {
		return root
	}
	return filepath.Join(root, "wallets", walletName)
}

// Per-wallet service directories and the cert/macaroon files the dashboard dials
// each daemon with. The certs/macaroons live under the active wallet's dir.
func DcrlndDir(walletName string) string { return ResolveServiceDir(DcrlndDataRoot, walletName) }
func DcrlndTLSCert(walletName string) string {
	return filepath.Join(DcrlndDir(walletName), "tls.cert")
}
func DcrlndMacaroon(walletName string) string {
	return filepath.Join(DcrlndDir(walletName), "admin.macaroon")
}

func DcrdexDir(walletName string) string { return ResolveServiceDir(DcrdexDataRoot, walletName) }
func DcrdexCert(walletName string) string {
	return filepath.Join(DcrdexDir(walletName), "rpc.cert")
}
func DcrdexWSCert(walletName string) string {
	return filepath.Join(DcrdexDir(walletName), "web.cert")
}

func BrclientdDir(walletName string) string {
	return ResolveServiceDir(BrclientdDataRoot, walletName)
}

// brclientd writes its mTLS certs under <appdata>/data/<network>/rpc/. These
// resolve the active wallet's cert/key files, mirroring DcrlndTLSCert/DcrdexCert.
func BrclientdServerCert(walletName, network string) string {
	return filepath.Join(BrclientdDir(walletName), "data", network, "rpc", "rpc.cert")
}
func BrclientdClientCert(walletName, network string) string {
	return filepath.Join(BrclientdDir(walletName), "data", network, "rpc", "rpc-client.cert")
}
func BrclientdClientKey(walletName, network string) string {
	return filepath.Join(BrclientdDir(walletName), "data", network, "rpc", "rpc-client.key")
}
