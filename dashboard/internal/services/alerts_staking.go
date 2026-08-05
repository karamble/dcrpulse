// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"dcrpulse/internal/alerts"
	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

// ticketAlertState is the per-wallet hash -> status baseline. primed stays
// false until one full listing succeeds, so the first pass (and the first
// after a wallet switch) seeds silently instead of flooding history with the
// wallet's whole ticket past.
var ticketAlertState struct {
	sync.Mutex
	statuses       map[string]string
	primed         bool
	lastDiffHeight int64
}

func resetTicketAlertBaseline() {
	st := &ticketAlertState
	st.Lock()
	st.statuses = nil
	st.primed = false
	st.lastDiffHeight = 0
	st.Unlock()
}

// StartTicketWatcher diffs the wallet's ticket set at most once per new block
// (only while synced) and emits purchase / vote / miss / expiry / revocation
// events. Always-on promotion of the autobuyer's purchase diff, using the raw
// GetTickets stream rather than ListTickets to skip its per-ticket VSP fee
// lookups.
func StartTicketWatcher(ctx context.Context) {
	go func() {
		events, cancel := SubscribeNodeSyncEvents()
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case snap, ok := <-events:
				if !ok {
					return
				}
				if snap.Status != "running" || SyncPaused() || RestoreDiscoveryActive() {
					continue
				}
				st := &ticketAlertState
				st.Lock()
				skip := snap.Blocks <= st.lastDiffHeight
				if !skip {
					st.lastDiffHeight = snap.Blocks
				}
				st.Unlock()
				if skip {
					continue
				}
				diffTicketAlerts(ctx)
			}
		}
	}()
}

func diffTicketAlerts(ctx context.Context) {
	client := rpc.WalletGrpcClient
	if client == nil {
		return
	}
	listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stream, err := client.GetTickets(listCtx, &pb.GetTicketsRequest{})
	if err != nil {
		log.Printf("alerts: ticket watcher: %v", err)
		return
	}
	next := make(map[string]string, 64)
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			// A partial listing must never become the baseline: missing
			// hashes would re-emit as "purchased" on the next pass.
			log.Printf("alerts: ticket watcher: %v", err)
			return
		}
		rec := ticketRecordFromResponse(resp)
		if rec.Hash == "" {
			continue
		}
		next[rec.Hash] = rec.Status
	}

	st := &ticketAlertState
	st.Lock()
	prev, primed := st.statuses, st.primed
	st.statuses = next
	st.primed = true
	st.Unlock()
	if !primed {
		return
	}

	for hash, status := range next {
		old, existed := prev[hash]
		if !existed {
			alerts.Emit("ticket_purchased", "Ticket "+shortTicketHash(hash), hash+"|purchased")
			continue
		}
		if old == status {
			continue
		}
		var code string
		switch status {
		case "VOTED":
			code = "ticket_voted"
		case "MISSED":
			code = "ticket_missed"
		case "EXPIRED":
			code = "ticket_expired"
		case "REVOKED":
			code = "ticket_revoked"
		default:
			continue
		}
		alerts.Emit(code, "Ticket "+shortTicketHash(hash), hash+"|"+status)
	}
}

func shortTicketHash(hash string) string {
	if len(hash) <= 16 {
		return hash
	}
	return fmt.Sprintf("%s…%s", hash[:8], hash[len(hash)-6:])
}
