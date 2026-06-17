// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/services"
)

// MCP resource URIs. Agents subscribe to these to get a notifications/resources/
// updated message when the underlying state changes, then re-read for the new
// value (the notification carries only the URI, per the MCP model). Each is
// gated by the same per-agent domain grant as the tools.
const (
	resNodeSync   = "dcrpulse://node/sync"
	resWalletSync = "dcrpulse://wallet/sync"
	resWalletBal  = "dcrpulse://wallet/balance"
	resBRMessages = "dcrpulse://bisonrelay/messages"
	resLightning  = "dcrpulse://lightning/events"
	resStaking    = "dcrpulse://staking/activity"
	resMixer      = "dcrpulse://privacy/mixer"
	resAudit      = "dcrpulse://mcp/audit"

	// domainAudit gates the cross-agent spend-audit resource. It is a
	// resource-only domain (no tools), granted like any other capability so the
	// user opts in before an agent can watch other agents' spends.
	domainAudit = "audit"

	// resourceRingMax bounds each event-feed resource so a long-running session
	// cannot grow memory without limit.
	resourceRingMax = 100
)

// resourceDef tags an MCP resource with its capability domain so the per-agent
// server registers only the resources that agent may read. read returns the
// current value to marshal as JSON; resources are inherently read-only.
type resourceDef struct {
	domain string
	uri    string
	name   string
	desc   string
	read   func(context.Context) (any, error)
}

// eventRing is a bounded, newest-last buffer of recent feed events. snapshot
// returns them newest-first to match the audit feed.
type eventRing struct {
	mu      sync.Mutex
	entries []any
}

func (r *eventRing) add(v any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, v)
	if len(r.entries) > resourceRingMax {
		r.entries = r.entries[len(r.entries)-resourceRingMax:]
	}
}

func (r *eventRing) snapshot() []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]any, len(r.entries))
	for i := range r.entries {
		out[i] = r.entries[len(r.entries)-1-i]
	}
	return out
}

var (
	brRing      = &eventRing{}
	lnRing      = &eventRing{}
	stakingRing = &eventRing{}
	mixerRing   = &eventRing{}
)

// resourceCatalog is the master list of MCP resources. State resources read
// fresh from services on demand; event-feed resources return a bounded ring fed
// by the subscription goroutines in startResourceFeeds.
var resourceCatalog = []resourceDef{
	{
		domain: "node", uri: resNodeSync, name: "Node sync status",
		desc: "Current dcrd sync state and chain tip. Updates on each new block.",
		read: func(context.Context) (any, error) { return services.GetNodeSyncSnapshot(), nil },
	},
	{
		domain: "wallet", uri: resWalletSync, name: "Wallet sync status",
		desc: "Current dcrwallet sync progress. Updates as the wallet syncs.",
		read: func(context.Context) (any, error) { return services.GetSyncSnapshot(), nil },
	},
	{
		domain: "wallet", uri: resWalletBal, name: "Wallet balances",
		desc: "Per-account balances. Updates on wallet-sync events and new blocks.",
		read: func(ctx context.Context) (any, error) { return services.FetchAllAccounts(ctx) },
	},
	{
		domain: "bisonrelay", uri: resBRMessages, name: "Bison Relay messages",
		desc: "Recent incoming private and group-chat messages, newest first. Updates as messages arrive.",
		read: func(context.Context) (any, error) { return brRing.snapshot(), nil },
	},
	{
		domain: "lightning", uri: resLightning, name: "Lightning events",
		desc: "Recent Lightning channel and invoice events, newest first.",
		read: func(context.Context) (any, error) { return lnRing.snapshot(), nil },
	},
	{
		domain: "staking", uri: resStaking, name: "Staking activity",
		desc: "Recent ticket-purchase and autobuyer events, newest first.",
		read: func(context.Context) (any, error) { return stakingRing.snapshot(), nil },
	},
	{
		domain: "privacy", uri: resMixer, name: "Mixer activity",
		desc: "Recent wallet mixer (privacy) events, newest first.",
		read: func(context.Context) (any, error) { return mixerRing.snapshot(), nil },
	},
	{
		domain: domainAudit, uri: resAudit, name: "Agent spend audit",
		desc: "Recent MCP agent spend attempts across all agents, newest first: allowed, denied, blocked.",
		read: func(context.Context) (any, error) { return AuditLog(auditMax), nil },
	},
}

// register adds the resource to a per-agent server. The read handler ignores the
// request (each resource is a whole snapshot) and serves JSON text.
func (rd resourceDef) register(s *mcp.Server) {
	s.AddResource(&mcp.Resource{
		URI:         rd.uri,
		Name:        rd.name,
		Description: rd.desc,
		MIMEType:    "application/json",
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		v, err := rd.read(ctx)
		if err != nil {
			return nil, err
		}
		return jsonResourceResult(rd.uri, v)
	})
}

// jsonResourceResult wraps a value as a single JSON text resource content.
func jsonResourceResult(uri string, v any) (*mcp.ReadResourceResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(b),
		}},
	}, nil
}

// domainForResourceURI maps a resource URI to its capability domain.
func domainForResourceURI(uri string) (string, bool) {
	for _, rd := range resourceCatalog {
		if rd.uri == uri {
			return rd.domain, true
		}
	}
	return "", false
}

// allowResourceSub permits a subscription only to a resource whose domain the
// agent is granted, so an agent cannot subscribe to a URI it cannot read.
func allowResourceSub(a *agent, uri string) error {
	domain, ok := domainForResourceURI(uri)
	if !ok || !a.allows(domain) {
		return fmt.Errorf("resource not available: %s", uri)
	}
	return nil
}

// agentResourceURIs lists the resource URIs visible to the agent, for the
// capabilities report.
func agentResourceURIs(a *agent) []string {
	var out []string
	for _, rd := range resourceCatalog {
		if a.allows(rd.domain) {
			out = append(out, rd.uri)
		}
	}
	return out
}

// notifyResourceUpdated tells every per-agent server that a resource changed.
// The SDK delivers the notification only to sessions actually subscribed to the
// URI, so a server without a subscriber (or without the resource registered) is
// a no-op. Servers are snapshotted under the lock, then notified outside it.
func notifyResourceUpdated(uri string) {
	serversMu.Lock()
	srvs := make([]*mcp.Server, 0, len(servers))
	for _, s := range servers {
		srvs = append(srvs, s)
	}
	serversMu.Unlock()
	params := &mcp.ResourceUpdatedNotificationParams{URI: uri}
	for _, s := range srvs {
		_ = s.ResourceUpdated(context.Background(), params)
	}
}

var resourceFeedsOnce sync.Once

// startResourceFeeds bridges the in-process event buses into MCP resource-updated
// notifications. Called once when MCP is first enabled; the goroutines run for
// the process lifetime, which is cheap because a notify with no subscriber is a
// no-op. The buses themselves are started at boot (cmd/dcrpulse/main.go), so a
// resource updates even with no browser open.
func startResourceFeeds() {
	resourceFeedsOnce.Do(func() {
		go feedNodeSync()
		go feedWalletSync()
		go feedBRMessages()
		go feedLightning()
		go feedStaking()
		go feedMixer()
		// The BR oversight loop also consumes the Bison Relay event bus, for
		// operator approve/deny replies.
		go startOversightConsumer()
	})
}

func feedNodeSync() {
	ch, cancel := services.SubscribeNodeSyncEvents()
	defer cancel()
	for range ch {
		// A new block changes the tip and may change balances.
		notifyResourceUpdated(resNodeSync)
		notifyResourceUpdated(resWalletBal)
	}
}

func feedWalletSync() {
	ch, cancel := services.SubscribeSyncEvents()
	defer cancel()
	for range ch {
		notifyResourceUpdated(resWalletSync)
		notifyResourceUpdated(resWalletBal)
	}
}

func feedBRMessages() {
	ch, cancel := services.Bisonrelay().Subscribe(64)
	defer cancel()
	for evt := range ch {
		switch evt.Type {
		case "pm", "gcm", "gc-message":
			brRing.add(evt)
			notifyResourceUpdated(resBRMessages)
		}
	}
}

func feedStaking() {
	pch, pcancel := services.SubscribePurchaseEvents()
	defer pcancel()
	ach, acancel := services.SubscribeAutobuyerEvents()
	defer acancel()
	for pch != nil || ach != nil {
		select {
		case e, ok := <-pch:
			if !ok {
				pch = nil
				continue
			}
			stakingRing.add(e)
			notifyResourceUpdated(resStaking)
		case e, ok := <-ach:
			if !ok {
				ach = nil
				continue
			}
			stakingRing.add(e)
			notifyResourceUpdated(resStaking)
		}
	}
}

func feedMixer() {
	ch, cancel := services.SubscribeMixerEvents()
	defer cancel()
	for e := range ch {
		mixerRing.add(e)
		notifyResourceUpdated(resMixer)
	}
}

// feedLightning opens dcrlnd's channel- and invoice-event streams and re-opens
// them with a backoff: unlike the other buses, each subscribe dials dcrlnd and
// can fail when Lightning is not set up or the node is locked.
func feedLightning() {
	for {
		ctx, cancel := context.WithCancel(context.Background())
		chCh, cErr := services.SubscribeLightningChannelEvents(ctx)
		inCh, iErr := services.StreamLightningInvoiceEvents(ctx)
		if cErr != nil && iErr != nil {
			cancel()
			time.Sleep(60 * time.Second)
			continue
		}
		for chCh != nil || inCh != nil {
			select {
			case e, ok := <-chCh:
				if !ok {
					chCh = nil
					continue
				}
				lnRing.add(e)
				notifyResourceUpdated(resLightning)
			case e, ok := <-inCh:
				if !ok {
					inCh = nil
					continue
				}
				lnRing.add(e)
				notifyResourceUpdated(resLightning)
			}
		}
		cancel()
		time.Sleep(60 * time.Second)
	}
}
