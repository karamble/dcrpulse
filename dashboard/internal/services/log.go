// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import dcrlog "dcrpulse/internal/log"

// One logger per domain rather than one for the package: services spans every
// area of the dashboard, and a single tag would be unfilterable.
var (
	alrtLog = dcrlog.ALRT
	brelLog = dcrlog.BREL
	govnLog = dcrlog.GOVN
	lghtLog = dcrlog.LGHT
	nodeLog = dcrlog.NODE
	settLog = dcrlog.SETT
	stkeLog = dcrlog.STKE
	wlltLog = dcrlog.WLLT
)
