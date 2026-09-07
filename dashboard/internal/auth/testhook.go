// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package auth

// PointAtForTest aims the package at a config file and returns a func that
// restores both the path and the loaded state. It is a shipping function rather
// than an export_test.go one so tests in other packages can put auth into a
// password-set state; rpc.SwapDcrlndClients exists for the same reason.
func PointAtForTest(path string) func() {
	prev := cfgPath
	cfgPath = func() string { return path }
	mu.Lock()
	pEn, pHash, pSec, pDis, pErr := enabled, hash, secret, dismissed, loadErr
	mu.Unlock()
	return func() {
		cfgPath = prev
		mu.Lock()
		enabled, hash, secret, dismissed, loadErr = pEn, pHash, pSec, pDis, pErr
		mu.Unlock()
	}
}
