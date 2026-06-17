// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resourceURIs mirrors internal/mcp's resourceCatalog so the phase can report
// which resources are granted vs withheld for the test agent's domains.
var resourceURIs = []string{
	"dcrpulse://node/sync",
	"dcrpulse://wallet/sync",
	"dcrpulse://wallet/balance",
	"dcrpulse://bisonrelay/messages",
	"dcrpulse://lightning/events",
	"dcrpulse://staking/activity",
	"dcrpulse://privacy/mixer",
	"dcrpulse://mcp/audit",
}

const auditResourceURI = "dcrpulse://mcp/audit"

// updateSink records the resource-updated notifications the server pushes.
type updateSink struct {
	mu   sync.Mutex
	uris map[string]int
}

func (s *updateSink) note(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uris[uri]++
}

func (s *updateSink) count(uri string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.uris[uri]
}

// connectWithNotify opens a session with the standalone SSE stream ENABLED (so
// the server can push notifications) and a resource-updated handler. The tool
// phases disable that stream; the subscriptions test needs it.
func connectWithNotify(ctx context.Context, endpoint, token string, sink *updateSink) (*mcp.ClientSession, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearerRT{token: token, base: http.DefaultTransport}},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: clientVersion}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			if req != nil && req.Params != nil {
				sink.note(req.Params.URI)
			}
		},
	})
	return client.Connect(ctx, transport, nil)
}

// runResourcesPhase exercises the resources surface: list (per-domain gating),
// read each granted resource, subscribe, prove an out-of-domain subscribe is
// refused, then trigger a denied wallet_send to fire an audit update and confirm
// the server pushes a resources/updated notification. Returns true on failure.
func runResourcesPhase(ctx context.Context, endpoint, token string) bool {
	section("Resources + subscriptions")
	sink := &updateSink{uris: map[string]int{}}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	s, err := connectWithNotify(cctx, endpoint, token, sink)
	cancel()
	if err != nil {
		fmt.Printf("  %s connect failed: %v\n", red("FAIL"), err)
		return true
	}
	defer s.Close()

	failed := false

	lr, err := s.ListResources(ctx, nil)
	if err != nil {
		fmt.Printf("  %s resources/list failed: %v\n", red("FAIL"), err)
		return true
	}
	exposed := map[string]bool{}
	for _, r := range lr.Resources {
		exposed[r.URI] = true
	}
	fmt.Printf("  %s resources/list -> %d exposed\n", green("OK"), len(lr.Resources))

	for _, uri := range resourceURIs {
		if !exposed[uri] {
			fmt.Printf("    %-30s %s domain not granted\n", uri, yellow("WITHHELD"))
			continue
		}
		rr, rerr := s.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if rerr != nil || len(rr.Contents) == 0 {
			fmt.Printf("    %-30s %s read failed: %v\n", uri, red("FAIL"), rerr)
			failed = true
			continue
		}
		fmt.Printf("    %-30s %s %s\n", uri, green("READ"), preview(rr.Contents[0].Text, 56))
	}

	for _, u := range []string{auditResourceURI, "dcrpulse://node/sync"} {
		if !exposed[u] {
			continue
		}
		if serr := s.Subscribe(ctx, &mcp.SubscribeParams{URI: u}); serr != nil {
			fmt.Printf("  %s subscribe %s failed: %v\n", red("FAIL"), u, serr)
			failed = true
		} else {
			fmt.Printf("  %s subscribed %s\n", green("OK"), u)
		}
	}

	// Subscribe gating: a resource outside the agent's domains must be refused.
	for _, uri := range resourceURIs {
		if exposed[uri] {
			continue
		}
		if serr := s.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); serr == nil {
			fmt.Printf("  %s subscribe to withheld %s was NOT refused\n", red("FAIL"), uri)
			failed = true
		} else {
			fmt.Printf("  %s subscribe to withheld %s refused\n", green("OK"), uri)
		}
		break
	}

	// Trigger a server push: a denied wallet_send (no grant) records an audit
	// entry, which fans out a resources/updated for the audit feed. Needs the
	// wallet + audit domains granted.
	if exposed[auditResourceURI] && hasTool(ctx, s, "wallet_send") {
		res, cerr := call(ctx, s, "wallet_send", map[string]any{"account": 0, "address": "Dsplaceholder0000000000000000000", "amountDcr": 0.0001})
		switch {
		case cerr != nil:
			fmt.Printf("  %s trigger wallet_send transport error: %v\n", red("FAIL"), cerr)
			failed = true
		case res.isError:
			fmt.Printf("  %s wallet_send refused (%s); expecting an audit push\n", green("OK"), preview(res.text, 40))
		default:
			fmt.Printf("  %s wallet_send unexpectedly succeeded\n", red("FAIL"))
			failed = true
		}
		deadline := time.Now().Add(6 * time.Second)
		for time.Now().Before(deadline) && sink.count(auditResourceURI) == 0 {
			time.Sleep(200 * time.Millisecond)
		}
		if n := sink.count(auditResourceURI); n > 0 {
			fmt.Printf("  %s received %d resources/updated push for %s\n", green("PASS"), n, auditResourceURI)
		} else {
			fmt.Printf("  %s no resources/updated push for %s within 6s\n", red("FAIL"), auditResourceURI)
			failed = true
		}
	} else {
		fmt.Printf("  %s skipping push test (grant the wallet+audit domains to run it)\n", yellow("note:"))
	}
	return failed
}

// hasTool reports whether a named tool is exposed to this session.
func hasTool(ctx context.Context, s *mcp.ClientSession, name string) bool {
	tl, err := s.ListTools(ctx, nil)
	if err != nil {
		return false
	}
	for _, t := range tl.Tools {
		if t.Name == name {
			return true
		}
	}
	return false
}
