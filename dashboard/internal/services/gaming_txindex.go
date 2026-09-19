// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"

	"dcrpulse/internal/rpc"
)

// DcrdHasTxIndex reports whether dcrd is running its transaction index.
//
// The gaming bridge needs it. Approving a payout does not broadcast it - the
// reconcile pass does, and that pass starts with getrawtransaction. Without the
// index dcrd refuses that call with an error the worker cannot read as "not
// found", so a payout every seat signed is assembled and then never sent: it
// sits at publishing, with nothing above debug to say why. dcrpulse enables the
// index by default, so the only operator this catches is one running their own
// dcrd - which is exactly the operator with nobody to tell them.
//
// Asked live, and deliberately not cached, unlike CurrentNetwork. A chain's
// identity cannot change while the process runs; the index can. Fixing it means
// setting txindex=1 and restarting dcrd, and dcrpulse keeps running across that.
// A cached "no" would go on refusing after the operator had already done the
// work, which is a worse failure than the one this prevents. One getinfo per
// settings read and per settings write costs nothing.
func DcrdHasTxIndex(ctx context.Context) (bool, error) {
	if rpc.DcrdClient == nil {
		return false, fmt.Errorf("dcrd is not connected")
	}
	info, err := rpc.DcrdClient.GetInfo(ctx)
	if err != nil {
		return false, fmt.Errorf("asking dcrd which indexes it runs: %w", err)
	}
	return info.TxIndex, nil
}

// TxIndexActive is DcrdHasTxIndex for a caller that has to decide either way.
//
// An unreachable dcrd reads as "no". Refusing to switch the bridge on is
// recoverable and says so; switching it on because the node could not be asked
// parks the first payout at publishing.
func TxIndexActive(ctx context.Context) bool {
	ok, err := DcrdHasTxIndex(ctx)
	return err == nil && ok
}
