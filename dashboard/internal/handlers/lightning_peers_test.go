// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dcrpulse/internal/rpc"

	"github.com/decred/dcrlnd/lnrpc"
	"google.golang.org/grpc"
)

// peerRoutesClient answers the reads the peer routes make; anything else panics.
type peerRoutesClient struct {
	lnrpc.LightningClient

	channels   []*lnrpc.Channel
	nodes      map[string]*lnrpc.LightningNode
	connectErr error
	dials      int
}

func (f *peerRoutesClient) ListPeers(context.Context, *lnrpc.ListPeersRequest, ...grpc.CallOption) (*lnrpc.ListPeersResponse, error) {
	return &lnrpc.ListPeersResponse{}, nil
}

func (f *peerRoutesClient) ListChannels(context.Context, *lnrpc.ListChannelsRequest, ...grpc.CallOption) (*lnrpc.ListChannelsResponse, error) {
	return &lnrpc.ListChannelsResponse{Channels: f.channels}, nil
}

func (f *peerRoutesClient) GetNodeInfo(_ context.Context, in *lnrpc.NodeInfoRequest, _ ...grpc.CallOption) (*lnrpc.NodeInfo, error) {
	if n, ok := f.nodes[in.GetPubKey()]; ok {
		return &lnrpc.NodeInfo{Node: n}, nil
	}
	return nil, errors.New("unable to find node")
}

func (f *peerRoutesClient) ConnectPeer(context.Context, *lnrpc.ConnectPeerRequest, ...grpc.CallOption) (*lnrpc.ConnectPeerResponse, error) {
	f.dials++
	if f.connectErr != nil {
		return nil, f.connectErr
	}
	return &lnrpc.ConnectPeerResponse{}, nil
}

func withPeerRoutes(t *testing.T, f *peerRoutesClient) {
	t.Helper()
	prev := rpc.SwapDcrlndClients(rpc.DcrlndClients{Lightning: f})
	t.Cleanup(func() { rpc.SwapDcrlndClients(prev) })
}

var channelPeer = "02" + strings.Repeat("ab", 32)

func newPeerRoutesClient() *peerRoutesClient {
	return &peerRoutesClient{
		channels: []*lnrpc.Channel{{RemotePubkey: channelPeer}},
		nodes: map[string]*lnrpc.LightningNode{
			channelPeer: {Alias: "ab", Addresses: []*lnrpc.NodeAddress{{Network: "tcp", Addr: "198.51.100.4:9735"}}},
		},
	}
}

func TestLightningPeersHandlerSendsEmptyListsNotNull(t *testing.T) {
	withPeerRoutes(t, &peerRoutesClient{})
	rec := httptest.NewRecorder()
	LightningPeersHandler(rec, httptest.NewRequest(http.MethodGet, "/api/wallet/ln/peers", nil))
	want := `{"peers":[],"offlineChannelPeers":[]}`
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("%d %s, want 200 %s", rec.Code, rec.Body.String(), want)
	}
}

func TestLightningReconnectPeerHandlerAnswers(t *testing.T) {
	other := "02" + strings.Repeat("cd", 32)
	for _, tc := range []struct {
		name       string
		body       string
		connectErr error
		code       int
		dials      int
		says       string
	}{
		{"bad body", `{`, nil, http.StatusBadRequest, 0, "invalid request body"},
		{"no pubkey", `{"pubKeyHex":" "}`, nil, http.StatusBadRequest, 0, "pubKeyHex required"},
		{"bad pubkey", `{"pubKeyHex":"zz"}`, nil, http.StatusBadRequest, 0, "invalid pubkey"},
		{"no channel", `{"pubKeyHex":"` + other + `"}`, nil, http.StatusBadRequest, 0, "no open channel"},
		{"connected", `{"pubKeyHex":"` + channelPeer + `"}`, nil, http.StatusNoContent, 1, ""},
		{"dial fails", `{"pubKeyHex":"` + channelPeer + `"}`, errors.New("handshake failed"), http.StatusInternalServerError, 1, "handshake failed"},
	} {
		f := newPeerRoutesClient()
		f.connectErr = tc.connectErr
		withPeerRoutes(t, f)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/wallet/ln/peers/reconnect", strings.NewReader(tc.body))
		LightningReconnectPeerHandler(rec, req)
		if rec.Code != tc.code || f.dials != tc.dials || !strings.Contains(rec.Body.String(), tc.says) {
			t.Errorf("%s: %d %q after %d dials, want %d saying %q after %d", tc.name, rec.Code, rec.Body.String(), f.dials, tc.code, tc.says, tc.dials)
		}
	}
}
