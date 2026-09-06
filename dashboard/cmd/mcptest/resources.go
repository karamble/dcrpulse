// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// connectWithNotify opens a session with a resource-updated handler. On the
// 2026-07-28 wire pushes arrive over each subscription's listen stream (the
// legacy standalone SSE stream is gone: the stateless server answers GET with
// 405 and new-wire clients never issue it).
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
	// The new wire's Subscribe opens its listen stream asynchronously and
	// swallows a refusal, so the deny is probed with a raw legacy-wire
	// resources/subscribe frame; the new-wire subscription is still opened so
	// the push phase can prove it delivers nothing.
	withheld := ""
	for _, uri := range resourceURIs {
		if !exposed[uri] {
			withheld = uri
			break
		}
	}
	if withheld != "" {
		refused, perr := legacySubscribeRefused(ctx, endpoint, token, withheld)
		switch {
		case perr != nil:
			fmt.Printf("  %s legacy subscribe probe %s: %v\n", red("FAIL"), withheld, perr)
			failed = true
		case refused:
			fmt.Printf("  %s subscribe to withheld %s refused (legacy wire)\n", green("OK"), withheld)
		default:
			fmt.Printf("  %s subscribe to withheld %s was NOT refused\n", red("FAIL"), withheld)
			failed = true
		}
		if serr := s.Subscribe(ctx, &mcp.SubscribeParams{URI: withheld}); serr != nil {
			fmt.Printf("  %s new-wire subscribe to withheld %s errored: %v\n", red("FAIL"), withheld, serr)
			failed = true
		}
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
		// Delivery is proven above, so the withheld subscription's silence is
		// meaningful: its listen stream was refused before registration.
		if withheld != "" {
			if n := sink.count(withheld); n != 0 {
				fmt.Printf("  %s withheld %s received %d pushes\n", red("FAIL"), withheld, n)
				failed = true
			} else {
				fmt.Printf("  %s withheld %s received no pushes\n", green("PASS"), withheld)
			}
		}
	} else {
		fmt.Printf("  %s skipping push test (grant the wallet+audit domains to run it)\n", yellow("note:"))
	}
	return failed
}

// legacySubscribeRefused probes the subscribe gate over the legacy wire, where
// resources/subscribe is synchronous and a refusal comes back as a JSON-RPC
// error. It reports whether the server refused the URI.
func legacySubscribeRefused(ctx context.Context, endpoint, token, uri string) (bool, error) {
	frame := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"resources/subscribe","params":{"uri":%q}}`, uri)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(frame))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	body := string(raw)
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		var sb strings.Builder
		for _, line := range strings.Split(body, "\n") {
			if data, ok := strings.CutPrefix(line, "data:"); ok {
				sb.WriteString(strings.TrimSpace(data))
			}
		}
		body = sb.String()
	}
	var rpc struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &rpc); err != nil {
		return false, fmt.Errorf("undecodable subscribe reply: %v", err)
	}
	return len(rpc.Error) > 0, nil
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
