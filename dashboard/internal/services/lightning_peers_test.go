// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/rpc"

	"github.com/decred/dcrlnd/lnrpc"
	"google.golang.org/grpc"
)

type fakePeersClient struct {
	lnrpc.LightningClient // embedded: anything unexpected panics

	peers       []*lnrpc.Peer
	peersErr    error
	channels    []*lnrpc.Channel
	channelsErr error
	nodes       map[string]*lnrpc.LightningNode
	connectErr  map[string]error
	dialled     []string
	requests    []*lnrpc.ConnectPeerRequest
}

func (f *fakePeersClient) ListPeers(context.Context, *lnrpc.ListPeersRequest, ...grpc.CallOption) (*lnrpc.ListPeersResponse, error) {
	if f.peersErr != nil {
		return nil, f.peersErr
	}
	return &lnrpc.ListPeersResponse{Peers: f.peers}, nil
}

func (f *fakePeersClient) ListChannels(context.Context, *lnrpc.ListChannelsRequest, ...grpc.CallOption) (*lnrpc.ListChannelsResponse, error) {
	if f.channelsErr != nil {
		return nil, f.channelsErr
	}
	return &lnrpc.ListChannelsResponse{Channels: f.channels}, nil
}

func (f *fakePeersClient) GetNodeInfo(_ context.Context, in *lnrpc.NodeInfoRequest, _ ...grpc.CallOption) (*lnrpc.NodeInfo, error) {
	n, ok := f.nodes[in.GetPubKey()]
	if !ok {
		return nil, errors.New("rpc error: code = Unknown desc = unable to find node")
	}
	return &lnrpc.NodeInfo{Node: n}, nil
}

func (f *fakePeersClient) ConnectPeer(_ context.Context, in *lnrpc.ConnectPeerRequest, _ ...grpc.CallOption) (*lnrpc.ConnectPeerResponse, error) {
	f.dialled = append(f.dialled, in.GetAddr().GetHost())
	f.requests = append(f.requests, in)
	if err := f.connectErr[in.GetAddr().GetHost()]; err != nil {
		return nil, err
	}
	return &lnrpc.ConnectPeerResponse{}, nil
}

func withFakePeers(t *testing.T, f *fakePeersClient) {
	t.Helper()
	resetAliasCache(t)
	prev := rpc.SwapDcrlndClients(rpc.DcrlndClients{Lightning: f})
	t.Cleanup(func() { rpc.SwapDcrlndClients(prev) })
}

// testPubkey builds a valid 33-byte node pubkey from one repeated hex byte.
func testPubkey(b string) string { return "02" + strings.Repeat(b, 32) }

func node(alias string, addrs ...string) *lnrpc.LightningNode {
	n := &lnrpc.LightningNode{Alias: alias}
	for _, a := range addrs {
		n.Addresses = append(n.Addresses, &lnrpc.NodeAddress{Network: "tcp", Addr: a})
	}
	return n
}

func TestListLightningPeersSplitsConnectedAndOfflineChannelPeers(t *testing.T) {
	pkA, pkB, pkC, pkG := testPubkey("aa"), testPubkey("bb"), testPubkey("cc"), testPubkey("dd")
	now := time.Now()
	flap := now.Add(-time.Hour)
	f := &fakePeersClient{
		channels: []*lnrpc.Channel{{RemotePubkey: pkA}, {RemotePubkey: pkB}, {RemotePubkey: pkB}, {RemotePubkey: pkC}},
		peers: []*lnrpc.Peer{
			{PubKey: pkC, Address: "198.51.100.3:9735"},
			{PubKey: pkG, Address: "198.51.100.7:9735"},
			{PubKey: pkA, Address: "198.51.100.1:9735", PingTime: 1500, BytesSent: 10, BytesRecv: 20,
				FlapCount: 3, LastFlapNs: flap.UnixNano(),
				Errors: []*lnrpc.TimestampedError{
					{Timestamp: uint64(now.Add(-2 * time.Hour).Unix()), Error: "older"},
					{Timestamp: uint64(now.Add(-time.Minute).Unix()), Error: "boom"},
				}},
		},
		nodes: map[string]*lnrpc.LightningNode{
			pkA: node("mid"), pkB: node("bee", "bbb.onion:9735", "203.0.113.2:9735"),
			pkC: node("zed"), pkG: node("alpha"),
		},
	}
	withFakePeers(t, f)

	got, err := ListLightningPeers(context.Background())
	if err != nil {
		t.Fatalf("ListLightningPeers: %v", err)
	}
	var order []string
	for _, p := range got.Peers {
		order = append(order, p.Alias)
	}
	// Channel peers first, then by name: "alpha" has no channel, so it is last.
	if want := []string{"mid", "zed", "alpha"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("peer order %v, want %v", order, want)
	}
	if !got.Peers[0].HasChannel || !got.Peers[1].HasChannel || got.Peers[2].HasChannel {
		t.Errorf("channel tags %v %v %v, want true true false", got.Peers[0].HasChannel, got.Peers[1].HasChannel, got.Peers[2].HasChannel)
	}
	a := got.Peers[0]
	if a.PingMicros != 1500 || a.BytesSent != 10 || a.BytesRecv != 20 || a.FlapCount != 3 || a.LastFlapUnix != flap.Unix() {
		t.Errorf("peer A = %+v", a)
	}
	if a.LastError != "boom" || a.LastErrorUnix != now.Add(-time.Minute).Unix() {
		t.Errorf("peer A last error %q at %d, want the newest one", a.LastError, a.LastErrorUnix)
	}
	if got.Peers[1].LastError != "" || got.Peers[1].LastFlapUnix != 0 {
		t.Errorf("peer C without errors or flaps = %+v", got.Peers[1])
	}

	if len(got.OfflineChannelPeers) != 1 {
		t.Fatalf("offline channel peers %+v, want only B", got.OfflineChannelPeers)
	}
	off := got.OfflineChannelPeers[0]
	wantAddrs := []string{"bbb.onion:9735", "203.0.113.2:9735"}
	if off.Pubkey != pkB || off.Alias != "bee" || off.Channels != 2 || !reflect.DeepEqual(off.Addresses, wantAddrs) {
		t.Errorf("offline peer = %+v", off)
	}
}

func TestListLightningPeersMarksTorPeers(t *testing.T) {
	t.Setenv("TOR_PROXY_IP", "127.0.0.1")
	f := &fakePeersClient{
		peers: []*lnrpc.Peer{
			{PubKey: testPubkey("a1"), Address: "abcdefghijklmnop.onion:9735"},
			{PubKey: testPubkey("a2"), Address: "127.0.0.1:41234", Inbound: true},
			{PubKey: testPubkey("a3"), Address: "198.51.100.5:4000", Inbound: true},
			{PubKey: testPubkey("a4"), Address: "127.0.0.1:9735"},
		},
		nodes: map[string]*lnrpc.LightningNode{},
	}
	withFakePeers(t, f)

	got, err := ListLightningPeers(context.Background())
	if err != nil {
		t.Fatalf("ListLightningPeers: %v", err)
	}
	tor := map[string]bool{}
	for _, p := range got.Peers {
		tor[p.Pubkey] = p.Tor
	}
	want := map[string]bool{
		testPubkey("a1"): true,  // onion address
		testPubkey("a2"): true,  // inbound through the onion service
		testPubkey("a3"): false, // inbound over clearnet
		testPubkey("a4"): false, // outbound, so the address is the peer's own
	}
	if !reflect.DeepEqual(tor, want) {
		t.Errorf("tor flags %v, want %v", tor, want)
	}
}

// An offline list built without the channels would read as "every channel
// peer is connected", so either list failing fails the whole read.
func TestListLightningPeersFailsWhenAListFails(t *testing.T) {
	for name, f := range map[string]*fakePeersClient{
		"peers":    {peersErr: errors.New("peers down")},
		"channels": {channelsErr: errors.New("channels down")},
	} {
		withFakePeers(t, f)
		if got, err := ListLightningPeers(context.Background()); err == nil {
			t.Errorf("%s failing: got %+v, want an error", name, got)
		}
	}
}

func TestReconnectLightningPeerRefusesWhatItCannotDial(t *testing.T) {
	pkChan, pkBare, pkOther := testPubkey("aa"), testPubkey("bb"), testPubkey("cc")
	for _, tc := range []struct {
		name   string
		pubkey string
		want   error
	}{
		{"not hex", "zz", ErrInvalidNodePubkey},
		{"no channel", pkOther, ErrNotChannelPeer},
		{"not in the graph", pkBare, ErrNoPeerAddress},
		{"no address", pkChan, ErrNoPeerAddress},
	} {
		f := &fakePeersClient{
			channels: []*lnrpc.Channel{{RemotePubkey: pkChan}, {RemotePubkey: pkBare}},
			nodes:    map[string]*lnrpc.LightningNode{pkChan: node("chan"), pkOther: node("other", "198.51.100.9:9735")},
		}
		withFakePeers(t, f)
		err := ReconnectLightningPeer(context.Background(), tc.pubkey)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err %v, want %v", tc.name, err, tc.want)
		}
		if len(f.dialled) != 0 {
			t.Errorf("%s: dialled %v, want nothing", tc.name, f.dialled)
		}
	}
}

func TestReconnectLightningPeerTriesTheGraphAddressesInOrder(t *testing.T) {
	pk := testPubkey("ab")
	refused := errors.New("connection refused")
	for _, tc := range []struct {
		name    string
		errs    map[string]error
		ok      bool
		dialled []string
	}{
		{"second answers", map[string]error{"h1:9735": refused}, true, []string{"h1:9735", "h2:9735"}},
		{"already connected", map[string]error{"h1:9735": errors.New("already connected to peer: " + pk)}, true, []string{"h1:9735"}},
		{"none answers", map[string]error{"h1:9735": errors.New("timeout"), "h2:9735": errors.New("timeout"), "h3:9735": refused}, false, []string{"h1:9735", "h2:9735", "h3:9735"}},
	} {
		f := &fakePeersClient{
			channels:   []*lnrpc.Channel{{RemotePubkey: pk}},
			nodes:      map[string]*lnrpc.LightningNode{pk: node("ab", "h1:9735", "h2:9735", "h3:9735")},
			connectErr: tc.errs,
		}
		withFakePeers(t, f)
		// Upper case on purpose: the channel list carries the key in lower case.
		err := ReconnectLightningPeer(context.Background(), strings.ToUpper(pk))
		if tc.ok != (err == nil) {
			t.Errorf("%s: err %v, want ok=%v", tc.name, err, tc.ok)
		}
		if !tc.ok && !errors.Is(err, refused) {
			t.Errorf("%s: err %v, want the last address's error", tc.name, err)
		}
		if !reflect.DeepEqual(f.dialled, tc.dialled) {
			t.Errorf("%s: dialled %v, want %v", tc.name, f.dialled, tc.dialled)
		}
		for _, r := range f.requests {
			if r.GetAddr().GetPubkey() != pk || r.GetTimeout() != peerDialTimeout || r.GetPerm() {
				t.Errorf("%s: request %+v, want pubkey %s, timeout %d, not permanent", tc.name, r, pk, peerDialTimeout)
			}
		}
	}
}
