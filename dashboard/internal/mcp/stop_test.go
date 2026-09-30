// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"sync/atomic"
	"testing"
	"time"
)

// A dashboard shutdown drops every held passphrase and lets a spend notice on
// its way reach the operator before the process ends.
func TestStopRevokesGrantsAndWaitsForNotices(t *testing.T) {
	grants.set("stop-agent", GrantSpec{WriteScopes: []string{scopeLightning}}, time.Now())
	t.Cleanup(func() { grants.revoke("stop-agent") })
	var sent atomic.Bool
	spendNotices.Add(1)
	go func() {
		defer spendNotices.Done()
		time.Sleep(100 * time.Millisecond)
		sent.Store(true)
	}()

	Stop()

	if _, ok := grants.info("stop-agent"); ok {
		t.Error("a spend grant survived the shutdown")
	}
	if !sent.Load() {
		t.Error("Stop returned before the spend notice was sent")
	}
}
