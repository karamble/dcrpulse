// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"sync"
	"testing"
)

// A wallet switch rewrites the brclientd cert paths, and ends by dropping the
// WS so it redials with them - so it wakes a reader of the same config struct
// immediately after writing it. The dialers copy that struct by value, which is
// six strings and not atomic.
func TestWalletSwitchRacesCertReaders(t *testing.T) {
	saved := BrclientdCfg
	t.Cleanup(func() { BrclientdCfg = saved })

	InitBrclientdConfig(BrclientdConfig{
		Host:       "127.0.0.1",
		Port:       "7676",
		StatusPort: "7677",
	})

	rv := mustID(t, testRV)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 2000; i++ {
			UpdateBrclientdCerts("server.cert", "client.cert", "client.key")
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 2000; i++ {
			// Fails on the missing cert files, but only after it has copied the
			// config the switch is rewriting.
			_, _, _ = BrclientdRTDTAudioDial(rv)
		}
	}()
	close(start)
	wg.Wait()
}
