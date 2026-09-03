// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/decred/dcrlnd/lnrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"

	"dcrpulse/internal/rpc"
)

type stubLightning struct {
	lnrpc.LightningClient
	connectErr error
	openErr    error
}

func (s stubLightning) ConnectPeer(context.Context, *lnrpc.ConnectPeerRequest, ...grpc.CallOption) (*lnrpc.ConnectPeerResponse, error) {
	return &lnrpc.ConnectPeerResponse{}, s.connectErr
}

func (s stubLightning) OpenChannelSync(context.Context, *lnrpc.OpenChannelRequest, ...grpc.CallOption) (*lnrpc.ChannelPoint, error) {
	return nil, s.openErr
}

const lnTestPeer = "03aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899@127.0.0.1:9735"

// TestOpenChannelKeepsReservationOnCommittedFailure is the regression guard for
// the refund defect: dcrlnd detaches the funding workflow from the caller's
// context, so releasing the cap on a failed - or cancelled - open would let an
// agent fund channels indefinitely inside a fixed daily cap.
func TestOpenChannelKeepsReservationOnCommittedFailure(t *testing.T) {
	spentAfter := func(t *testing.T, client lnrpc.LightningClient) int64 {
		t.Helper()
		prev := rpc.SwapDcrlndClients(rpc.DcrlndClients{Lightning: client})
		t.Cleanup(func() { rpc.SwapDcrlndClients(prev) })

		const agentID = "ln-refund"
		grants.set(agentID, GrantSpec{
			WriteScopes: []string{scopeLightning},
			PerTxAtoms:  5e8,
			DailyAtoms:  5e8,
		}, time.Now())
		t.Cleanup(func() { grants.revoke(agentID) })

		cs := connectTo(t, testAgent(agentID, "ln", map[string]bool{"lightning": true}))
		if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "ln_open_channel",
			Arguments: map[string]any{"peerUri": lnTestPeer, "localDcr": 1.0},
		}); err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		info, ok := SpendGrantInfo(agentID)
		if !ok {
			t.Fatal("grant vanished")
		}
		return info.SpentAtoms
	}

	t.Run("a funding failure keeps the reservation", func(t *testing.T) {
		if got := spentAfter(t, stubLightning{openErr: errors.New("boom")}); got == 0 {
			t.Fatal("reservation was refunded after dcrlnd had been asked to fund")
		}
	})

	t.Run("a failure before funding refunds", func(t *testing.T) {
		if got := spentAfter(t, stubLightning{connectErr: errors.New("no route to host")}); got != 0 {
			t.Fatalf("spent %d atoms after a pre-funding failure, want the reservation released", got)
		}
	})
}
