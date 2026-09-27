// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"time"

	dcrlog "dcrpulse/internal/log"
)

var gameLog = dcrlog.GAME

// ackTimeout bounds one acknowledgement to Bison Relay.
const ackTimeout = 30 * time.Second

// importedXpubAccountBase is the first account number dcrwallet gives an
// imported xpub account.
const importedXpubAccountBase = uint32(1) << 31
