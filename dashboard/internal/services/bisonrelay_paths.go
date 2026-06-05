// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"os"
	"path/filepath"

	"dcrpulse/internal/config"

	"github.com/decred/dcrd/dcrutil/v4"
)

// BrclientdDataDir returns the dashboard-side view of the ACTIVE wallet's
// brclientd appdata. Each wallet has its own Bison Relay identity: the default
// wallet uses the brclientd root, named wallets use <root>/wallets/<name>.
// Root resolution: $BRCLIENTD_DATA_DIR if set (typical for docker / Umbrel),
// otherwise the OS-natural ~/.brclientd/ via dcrutil.AppDataDir.
func BrclientdDataDir() string {
	return config.ResolveServiceDir(brclientdDataRoot(), CurrentWalletName())
}

func brclientdDataRoot() string {
	if v := os.Getenv("BRCLIENTD_DATA_DIR"); v != "" {
		return v
	}
	return dcrutil.AppDataDir("brclientd", false)
}

// BrclientdEmbedsDir is where BR's clientdb writes inlined embed payloads
// extracted from incoming PMs.
func BrclientdEmbedsDir(network string) string {
	return filepath.Join(BrclientdDataDir(), "data", network, "db", "embeds")
}

// BrclientdDownloadsDir is the root of brclientd's completed file-transfer
// downloads, organized by sender nick underneath.
func BrclientdDownloadsDir(network string) string {
	return filepath.Join(BrclientdDataDir(), "data", network, "downloads")
}

// BrclientdLogPath is the rotating log file brclientd writes to.
func BrclientdLogPath(network string) string {
	return filepath.Join(BrclientdDataDir(), "logs", network, "brclientd.log")
}

// BrclientdDaemonCertPaths resolves the mTLS server/client cert and client key
// for the brclientd instance the daemon supervisors are currently running,
// reading the same control pointer they read. Used at startup and after a
// wallet switch so the dashboard pins the active wallet's brclientd cert (each
// wallet has its own Bison Relay identity and cert). Network comes from the
// pointer; absent that, a best-effort dcrd lookup, then a mainnet default.
func BrclientdDaemonCertPaths(ctx context.Context) (server, client, key string) {
	name, network := DaemonWalletSelection()
	if network == "" {
		if n, err := CurrentNetwork(ctx); err == nil && n != "" {
			network = n
		} else {
			network = "mainnet"
		}
	}
	return config.BrclientdServerCert(name, network),
		config.BrclientdClientCert(name, network),
		config.BrclientdClientKey(name, network)
}
