// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

// Zero clears a secret the caller is done with. Best-effort: a Go string cannot
// be zeroed at all, so secrets that outlive the call producing them travel as
// []byte and are wiped here, while the request string they were copied from
// lives until the garbage collector runs.
func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
