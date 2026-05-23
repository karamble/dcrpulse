// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"os"
	"path/filepath"

	"github.com/decred/dcrd/dcrutil/v4"
)

// BrclientdDataDir returns the dashboard-side view of brclientd's appdata
// root. Resolution order: $BRCLIENTD_DATA_DIR if set (typical for docker /
// Umbrel deployments that mount the volume at a known path), otherwise the
// OS-natural ~/.brclientd/ via dcrutil.AppDataDir (same path brclientd
// itself uses when run standalone without --appdata).
func BrclientdDataDir() string {
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

// BrclientdDefaultCertPath returns the default location for one of
// brclientd's mTLS cert files when no explicit env override is set.
// The cert basename is supplied by the caller (rpc.cert / rpc-client.cert /
// rpc-client.key) since each end of the mTLS handshake needs a different one.
func BrclientdDefaultCertPath(network, basename string) string {
	return filepath.Join(BrclientdDataDir(), "data", network, "rpc", basename)
}
