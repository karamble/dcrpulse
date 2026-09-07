// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"testing"

	"dcrpulse/internal/types"
	"dcrpulse/internal/utils"
)

// The browser path used to collapse a send-all request to its first recipient
// and drop the rest without a word. The refusal runs before the address lookup,
// so this needs no wallet.
func TestResolveTxOutputsRejectsSendAllWithManyOutputs(t *testing.T) {
	req := &types.ConstructTransactionRequest{
		SendAll: true,
		Outputs: []types.TxRecipient{
			{Address: "DsAlpha", AmountAtoms: 100000000},
			{Address: "DsBeta", AmountAtoms: 0},
		},
	}
	got, err := resolveTxOutputs(context.Background(), req)
	if err == nil {
		t.Fatalf("send-all with two recipients resolved to %v, want a refusal", got)
	}
	if !errors.Is(err, utils.ErrSendAllSingleRecipient) {
		t.Fatalf("refusal = %v, want the shared sentinel", err)
	}
}
