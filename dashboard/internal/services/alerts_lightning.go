// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"dcrpulse/internal/alerts"
	"dcrpulse/internal/rpc"

	"github.com/decred/dcrlnd/lnrpc"
)

// StartLightningWatcher evaluates Lightning alert state on a slow ticker and
// holds a process-lifetime channel-events subscription for force-closes.
// dcrlnd is sentinel-deferred and may never exist for the life of the
// process, so a nil client is tolerated forever; LightningStatus performs the
// lazy ReinitDcrlndClient re-dial and its stage gates every probe (locked /
// starting / never-configured raise nothing).
func StartLightningWatcher(ctx context.Context) {
	go func() {
		var streamUp atomic.Bool
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if SyncPaused() {
				continue
			}
			st := LightningStatus(ctx)
			if st.Stage != "ready" && st.Stage != "syncing" {
				// No reachable node means the peers condition is moot; the
				// unlock/setup stages must not alarm.
				alerts.Resolve("ln_no_peers", "")
				continue
			}
			client := rpc.LightningClient
			if client == nil {
				continue
			}
			if !streamUp.Load() {
				streamUp.Store(true)
				go func() {
					defer streamUp.Store(false)
					watchLnChannelEvents(ctx, client)
				}()
			}
			callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			chans, cerr := client.ListChannels(callCtx, &lnrpc.ListChannelsRequest{})
			peers, perr := client.ListPeers(callCtx, &lnrpc.ListPeersRequest{})
			cancel()
			if cerr != nil || perr != nil {
				continue
			}
			// The 5m debounce rides out the syncedToChain flap dcrlnd goes
			// through right after a restart.
			if len(chans.GetChannels()) > 0 && len(peers.GetPeers()) == 0 {
				alerts.Raise("ln_no_peers", "", "node has channels but no connected peers")
			} else {
				alerts.Resolve("ln_no_peers", "")
			}
		}
	}()
}

// watchLnChannelEvents consumes the raw SubscribeChannelEvents stream (the
// typed wrapper in lightning_events.go drops CloseType, which force-close
// detection needs). Returns when the stream dies; the ticker restarts it
// against the then-current client, so ReinitDcrlndClient repoints are picked
// up.
func watchLnChannelEvents(ctx context.Context, client lnrpc.LightningClient) {
	stream, err := client.SubscribeChannelEvents(ctx, &lnrpc.ChannelEventSubscription{})
	if err != nil {
		return
	}
	for {
		ev, err := stream.Recv()
		if err != nil {
			return
		}
		c := ev.GetClosedChannel()
		if c == nil {
			continue
		}
		switch c.GetCloseType() {
		case lnrpc.ChannelCloseSummary_LOCAL_FORCE_CLOSE,
			lnrpc.ChannelCloseSummary_REMOTE_FORCE_CLOSE,
			lnrpc.ChannelCloseSummary_BREACH_CLOSE:
			alerts.Emit("ln_channel_force_closed",
				fmt.Sprintf("Channel %s force-closed (%s)", c.GetChannelPoint(),
					strings.ToLower(strings.ReplaceAll(c.GetCloseType().String(), "_", " "))),
				c.GetChannelPoint())
		}
	}
}
