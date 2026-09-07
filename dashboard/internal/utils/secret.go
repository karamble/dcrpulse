// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import "errors"

// Zero clears a secret the caller is done with. Best-effort: a Go string cannot
// be zeroed at all, so secrets that outlive the call producing them travel as
// []byte and are wiped here, while the request string they were copied from
// lives until the garbage collector runs.
func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ErrSendAllSingleRecipient is the shared refusal for a send-all request that
// names more than one recipient. Send-all sweeps the whole balance through the
// change destination, so a second recipient and a stated amount cannot both be
// honoured. Shared so the wallet, the shared-wallet and the HTTP paths refuse it
// in the same words.
var ErrSendAllSingleRecipient = errors.New("send all pays a single recipient")
