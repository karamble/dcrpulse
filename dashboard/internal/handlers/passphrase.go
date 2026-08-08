// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

// zeroBytes clears a passphrase copy before the handler returns. Best-effort:
// the decoder's intermediate buffers and the request string itself live until
// the garbage collector runs; this keeps the copies we control consistent.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
