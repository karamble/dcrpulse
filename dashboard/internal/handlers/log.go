// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import dcrlog "dcrpulse/internal/log"

// Handlers are the HTTP edge of every domain, so the tag follows the domain
// the endpoint serves rather than the package.
var (
	alrtLog = dcrlog.ALRT
	brelLog = dcrlog.BREL
	dexcLog = dcrlog.DEXC
	govnLog = dcrlog.GOVN
	lghtLog = dcrlog.LGHT
	nodeLog = dcrlog.NODE
	settLog = dcrlog.SETT
	stkeLog = dcrlog.STKE
	tstpLog = dcrlog.TSTP
	wlltLog = dcrlog.WLLT
)
