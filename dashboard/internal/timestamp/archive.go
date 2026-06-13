// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package timestamp

import (
	"sync"

	"dcrpulse/internal/config"
)

var (
	archiveOnce sync.Once
	archiveDB   *Store
	archiveErr  error
)

// Archive returns the process-wide timestamp store, opening it on first use at
// config.TimestampArchivePath(). The archive is global (not per-wallet) because
// proofs are about files, not wallet keys, and must survive wallet switches.
func Archive() (*Store, error) {
	archiveOnce.Do(func() {
		archiveDB, archiveErr = OpenStore(config.TimestampArchivePath())
	})
	return archiveDB, archiveErr
}
