// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"sync/atomic"

	"dcrpulse/internal/config"
)

// externalCfgPath locates the global config; tests point it at a temp file.
var externalCfgPath = config.GlobalCfgPath

// externalGateDegraded is set while the settings cannot be read, so the refusal
// is logged once per episode rather than once per request.
var externalGateDegraded atomic.Bool

// ExternalRequestAllowed reports whether the external-request toggle name is on.
// An absent config, list or entry means on; a config or list that cannot be read
// means off, so a damaged file never turns a service back on.
func ExternalRequestAllowed(name string) bool {
	gc, err := config.LoadGlobalCfgAt(externalCfgPath())
	var allowed map[string]bool
	if err == nil {
		allowed, err = gc.AllowedExternalRequests()
	}
	if err != nil {
		if externalGateDegraded.CompareAndSwap(false, true) {
			settLog.Warnf("External request settings are unreadable (%v); outside services stay off until they can be read", err)
		}
		return false
	}
	if externalGateDegraded.CompareAndSwap(true, false) {
		settLog.Infof("External request settings are readable again")
	}
	v, ok := allowed[name]
	return !ok || v
}
