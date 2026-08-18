// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"sync"
	"testing"

	"github.com/decred/dcrlnd/lnrpc"
	"github.com/decred/dcrlnd/lnrpc/routerrpc"
)

type markerLightning struct{ lnrpc.LightningClient }
type markerRouter struct{ routerrpc.RouterClient }

// TestDcrlndSnapshotPublishesWholeGenerations pins the two properties the
// snapshot exists for: concurrent readers against a swapping writer are
// race-free (run under -race), and a reader only ever observes a complete
// generation - both marker clients set, or both nil - never a mix of two
// publishes.
func TestDcrlndSnapshotPublishesWholeGenerations(t *testing.T) {
	full := DcrlndClients{Lightning: markerLightning{}, Router: markerRouter{}}
	prev := SwapDcrlndClients(DcrlndClients{})
	t.Cleanup(func() { SwapDcrlndClients(prev) })

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				c := Dcrlnd()
				if (c.Lightning == nil) != (c.Router == nil) {
					// Mutation catch: a field-by-field publish exposes a
					// half-written generation here.
					t.Error("observed a mixed snapshot: one of two same-generation clients nil")
					return
				}
			}
		}()
	}
	for i := 0; i < 2000; i++ {
		SwapDcrlndClients(full)
		SwapDcrlndClients(DcrlndClients{})
	}
	close(stop)
	wg.Wait()
}
