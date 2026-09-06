// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/decred/dcrlnd/lnrpc"
	"google.golang.org/grpc"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
)

// stubLightning fails at whichever step the test is exercising. Embedding the
// interface leaves every other method unimplemented, which is fine: reaching one
// would be a bug in the code under test.
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

func withLightning(t *testing.T, c lnrpc.LightningClient) {
	t.Helper()
	prev := rpc.SwapDcrlndClients(rpc.DcrlndClients{Lightning: c})
	t.Cleanup(func() { rpc.SwapDcrlndClients(prev) })
}

const testPeerURI = "03aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899@127.0.0.1:9735"

// TestOpenChannelMarksOnlyCommittedFailures pins which failures release a spend
// reservation. Once dcrlnd has been asked to fund, its OpenChannelSync stops
// watching the caller's context, so a failure there cannot be read as "nothing
// was spent" - but everything before it can.
func TestOpenChannelMarksOnlyCommittedFailures(t *testing.T) {
	req := &types.OpenChannelRequest{PeerURI: testPeerURI, LocalAtoms: 1e8}

	t.Run("a funding failure is marked as possibly spent", func(t *testing.T) {
		withLightning(t, stubLightning{openErr: errors.New("boom")})
		_, err := OpenLightningChannel(context.Background(), req)
		if !errors.Is(err, ErrSpendStarted) {
			t.Fatalf("OpenChannelSync failure not marked as started: %v", err)
		}
	})

	t.Run("a cancelled call is marked as possibly spent", func(t *testing.T) {
		withLightning(t, stubLightning{openErr: context.Canceled})
		_, err := OpenLightningChannel(context.Background(), req)
		if !errors.Is(err, ErrSpendStarted) {
			t.Fatalf("cancelled funding not marked as started: %v", err)
		}
	})

	t.Run("failures before funding are not marked", func(t *testing.T) {
		withLightning(t, stubLightning{connectErr: errors.New("no route to host")})
		if _, err := OpenLightningChannel(context.Background(), req); errors.Is(err, ErrSpendStarted) {
			t.Errorf("ConnectPeer failure wrongly marked as started: %v", err)
		}
		withLightning(t, stubLightning{})
		if _, err := OpenLightningChannel(context.Background(), &types.OpenChannelRequest{PeerURI: "nonsense"}); errors.Is(err, ErrSpendStarted) {
			t.Errorf("bad peer URI wrongly marked as started: %v", err)
		}
	})

	t.Run("an absent daemon is not marked", func(t *testing.T) {
		prev := rpc.SwapDcrlndClients(rpc.DcrlndClients{})
		t.Cleanup(func() { rpc.SwapDcrlndClients(prev) })
		if _, err := OpenLightningChannel(context.Background(), req); errors.Is(err, ErrSpendStarted) {
			t.Errorf("absent daemon wrongly marked as started: %v", err)
		}
	})
}
