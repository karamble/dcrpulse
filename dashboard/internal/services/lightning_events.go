// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"strings"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrlnd/lnrpc"
)

// SubscribeLightningChannelEvents opens dcrlnd's SubscribeChannelEvents
// stream and forwards typed events onto a buffered Go channel. The
// caller closes the context when done. Mirrors Decrediton's
// subscribeChannelEvents at LNActions.js:667-681 — every event triggers
// a list refresh in the UI, so we don't need to over-design the event
// payload; lowercased UpdateType + channel point is enough.
func SubscribeLightningChannelEvents(ctx context.Context) (<-chan types.ChannelEvent, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	stream, err := rpc.LightningClient.SubscribeChannelEvents(ctx, &lnrpc.ChannelEventSubscription{})
	if err != nil {
		return nil, fmt.Errorf("SubscribeChannelEvents: %w", err)
	}

	out := make(chan types.ChannelEvent, 16)
	go func() {
		defer close(out)
		for {
			ev, err := stream.Recv()
			if err != nil {
				return
			}
			typed := types.ChannelEvent{Type: strings.ToLower(ev.GetType().String())}
			if c := ev.GetOpenChannel(); c != nil {
				typed.ChannelPoint = c.GetChannelPoint()
				typed.RemotePubkey = c.GetRemotePubkey()
			}
			if c := ev.GetClosedChannel(); c != nil {
				typed.ChannelPoint = c.GetChannelPoint()
				typed.RemotePubkey = c.GetRemotePubkey()
			}
			if c := ev.GetActiveChannel(); c != nil {
				typed.ChannelPoint = formatChannelPoint(c)
			}
			if c := ev.GetInactiveChannel(); c != nil {
				typed.ChannelPoint = formatChannelPoint(c)
			}
			if c := ev.GetPendingOpenChannel(); c != nil {
				typed.ChannelPoint = fmt.Sprintf("%s:%d", reversedHex(c.GetTxid()), c.GetOutputIndex())
			}
			select {
			case out <- typed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func formatChannelPoint(cp *lnrpc.ChannelPoint) string {
	if cp == nil {
		return ""
	}
	txid := cp.GetFundingTxidStr()
	if txid == "" {
		txid = reversedHex(cp.GetFundingTxidBytes())
	}
	return fmt.Sprintf("%s:%d", txid, cp.GetOutputIndex())
}
