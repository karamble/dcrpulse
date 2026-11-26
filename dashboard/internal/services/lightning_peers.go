// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrlnd/lnrpc"
)

// The reconnect refusals a caller can act on.
var (
	ErrNotChannelPeer = errors.New("no open channel with this node")
	ErrNoPeerAddress  = errors.New("the channel graph has no address for this node")
)

// peerDialTimeout bounds each reconnect attempt, in seconds.
const peerDialTimeout = 20

// ListLightningPeers lists dcrlnd's connected peers and the open-channel
// remotes that are not connected.
func ListLightningPeers(ctx context.Context) (*types.LightningPeers, error) {
	client := rpc.Dcrlnd().Lightning
	if client == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := client.ListPeers(ctx, &lnrpc.ListPeersRequest{LatestError: true})
	if err != nil {
		return nil, fmt.Errorf("ListPeers: %w", err)
	}
	channels, err := openChannelPeers(ctx, client)
	if err != nil {
		return nil, err
	}
	torIPs := torProxyIPs()
	out := &types.LightningPeers{
		Peers:               []types.LightningPeer{},
		OfflineChannelPeers: []types.LightningOfflinePeer{},
	}
	connected := map[string]bool{}
	for _, p := range resp.GetPeers() {
		connected[p.GetPubKey()] = true
		row := types.LightningPeer{
			Pubkey:     p.GetPubKey(),
			Alias:      lightningNodeAlias(ctx, client, p.GetPubKey()),
			Address:    p.GetAddress(),
			Inbound:    p.GetInbound(),
			Tor:        isTorPeerAddr(p.GetAddress(), p.GetInbound(), torIPs),
			PingMicros: p.GetPingTime(),
			BytesSent:  p.GetBytesSent(),
			BytesRecv:  p.GetBytesRecv(),
			AtomsSent:  p.GetAtomsSent(),
			AtomsRecv:  p.GetAtomsRecv(),
			FlapCount:  p.GetFlapCount(),
			HasChannel: channels[p.GetPubKey()] > 0,
		}
		if ns := p.GetLastFlapNs(); ns > 0 {
			row.LastFlapUnix = ns / 1e9
		}
		if errs := p.GetErrors(); len(errs) > 0 {
			last := errs[len(errs)-1]
			row.LastError = last.GetError()
			row.LastErrorUnix = int64(last.GetTimestamp())
		}
		out.Peers = append(out.Peers, row)
	}
	for pubkey, n := range channels {
		if connected[pubkey] {
			continue
		}
		row := types.LightningOfflinePeer{Pubkey: pubkey, Channels: n, Addresses: []string{}}
		if info, err := client.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{PubKey: pubkey}); err == nil {
			row.Alias = info.GetNode().GetAlias()
			for _, a := range info.GetNode().GetAddresses() {
				row.Addresses = append(row.Addresses, a.GetAddr())
			}
		}
		out.OfflineChannelPeers = append(out.OfflineChannelPeers, row)
	}
	sort.Slice(out.Peers, func(i, j int) bool {
		a, b := out.Peers[i], out.Peers[j]
		if a.HasChannel != b.HasChannel {
			return a.HasChannel
		}
		return peerSortKey(a.Alias, a.Pubkey) < peerSortKey(b.Alias, b.Pubkey)
	})
	sort.Slice(out.OfflineChannelPeers, func(i, j int) bool {
		a, b := out.OfflineChannelPeers[i], out.OfflineChannelPeers[j]
		return peerSortKey(a.Alias, a.Pubkey) < peerSortKey(b.Alias, b.Pubkey)
	})
	return out, nil
}

// ReconnectLightningPeer asks dcrlnd to connect to an open-channel remote at
// the addresses the channel graph advertises for it, first answer wins. Only
// graph addresses are dialled, never one the caller names.
func ReconnectLightningPeer(ctx context.Context, pubkeyHex string) error {
	client := rpc.Dcrlnd().Lightning
	if client == nil {
		return fmt.Errorf("dcrlnd not available")
	}
	pubkeyHex = strings.ToLower(strings.TrimSpace(pubkeyHex))
	if _, err := parseNodePubkey(pubkeyHex); err != nil {
		return err
	}
	channels, err := openChannelPeers(ctx, client)
	if err != nil {
		return err
	}
	if channels[pubkeyHex] == 0 {
		return ErrNotChannelPeer
	}
	info, err := client.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{PubKey: pubkeyHex})
	if err != nil {
		if strings.Contains(err.Error(), "unable to find node") {
			return ErrNoPeerAddress
		}
		return fmt.Errorf("GetNodeInfo: %w", err)
	}
	addrs := info.GetNode().GetAddresses()
	if len(addrs) == 0 {
		return ErrNoPeerAddress
	}
	var lastErr error
	for _, a := range addrs {
		_, err := client.ConnectPeer(ctx, &lnrpc.ConnectPeerRequest{
			Addr:    &lnrpc.LightningAddress{Pubkey: pubkeyHex, Host: a.GetAddr()},
			Timeout: peerDialTimeout,
		})
		if err == nil || isAlreadyConnected(err) {
			return nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return fmt.Errorf("ConnectPeer: %w", lastErr)
}

// openChannelPeers counts the open channels per remote pubkey.
func openChannelPeers(ctx context.Context, client lnrpc.LightningClient) (map[string]int, error) {
	resp, err := client.ListChannels(ctx, &lnrpc.ListChannelsRequest{})
	if err != nil {
		return nil, fmt.Errorf("ListChannels: %w", err)
	}
	out := map[string]int{}
	for _, c := range resp.GetChannels() {
		out[c.GetRemotePubkey()]++
	}
	return out, nil
}

// isTorPeerAddr reports a peer reached over Tor: an onion address, or an
// inbound connection arriving from the local tor container.
func isTorPeerAddr(addr string, inbound bool, torIPs map[string]bool) bool {
	if isOnionAddr(addr) {
		return true
	}
	if !inbound {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	return err == nil && torIPs[host]
}

func peerSortKey(alias, pubkey string) string {
	if alias != "" {
		return strings.ToLower(alias)
	}
	return pubkey
}
