// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/types"
)

const (
	brmcpBotA = "a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0"
	brmcpBotB = "b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1"
)

// stubBRMCP replaces the daemon read and the address book for one test.
func stubBRMCP(t *testing.T, st brmcpState, err error, contacts []map[string]any) {
	t.Helper()
	prevFetch, prevContacts := fetchBRMCPState, loadBRContacts
	fetchBRMCPState = func(context.Context) (brmcpState, error) { return st, err }
	loadBRContacts = func(context.Context) ([]map[string]any, error) { return contacts, nil }
	t.Cleanup(func() { fetchBRMCPState, loadBRContacts = prevFetch, prevContacts })
}

// brmcpGoldenState is an enabled approval-mode bridge with one parked payment
// and three settled or failing spends, oldest first as brclientd lists them.
func brmcpGoldenState() brmcpState {
	return brmcpState{
		settings: types.BRMCPSettings{
			Enabled:             true,
			Token:               "super-secret",
			Mode:                "approval",
			PerCallCapDcr:       0.5,
			PerDayCapDcr:        2,
			AllowedBots:         []string{brmcpBotA},
			AllowedIPs:          []string{"10.0.0.7"},
			ApprovalTimeoutSecs: 120,
			TipWaitSecs:         30,
			LastDenied:          &types.BRMCPDenied{IP: "9.9.9.9", At: "2026-09-01T10:00:00Z"},
		},
		pending: []types.BRMCPPending{
			{ID: "p1", Bot: brmcpBotA, Tool: "quote", AmountDcr: 0.1, Created: 1700000000},
		},
		spend: types.BRMCPSpend{
			Entries: []types.BRMCPSpendEntry{
				{TS: 1700000100, Bot: brmcpBotA, Tool: "quote", Rail: "ln", AmountDcr: 0.1, Status: "ok"},
				{TS: 1700000200, Bot: brmcpBotB, Tool: "image", Rail: "ln", AmountDcr: 0.25, Status: "failed", Err: "no route"},
				{TS: 1700000300, Bot: brmcpBotA, Tool: "quote", Rail: "ln", AmountDcr: 0.1, Status: "pending"},
			},
			TodayDcr: 0.45,
		},
	}
}

func readBRMCPJSON(t *testing.T) (brmcpView, string) {
	t.Helper()
	v, err := readBRMCP(context.Background())
	if err != nil {
		t.Fatalf("readBRMCP: %v", err)
	}
	view, ok := v.(brmcpView)
	if !ok {
		t.Fatalf("readBRMCP returned %T", v)
	}
	b, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return view, string(b)
}

// The bridge resource is granted by its own domain: a brmcp agent lists it and
// nothing from the other resource-only domain, and (TestNodeOnlyAgentSeesOnly
// NodeResources) a node-only agent never sees it.
func TestBRMCPAgentSeesBridgeResource(t *testing.T) {
	a := testAgent("rb", "bridge-watcher", map[string]bool{domainBRMCP: true})
	uris := resourceURIs(t, connectTo(t, a))
	if !uris[resBRMCP] {
		t.Errorf("brmcp agent missing %s", resBRMCP)
	}
	if uris[resAudit] {
		t.Errorf("brmcp agent must NOT expose %s", resAudit)
	}
	if d, ok := domainForResourceURI(resBRMCP); !ok || d != domainBRMCP {
		t.Errorf("resource domain = %q, %v; want %q", d, ok, domainBRMCP)
	}
}

// The golden view: settings without the token and allow lists, the parked
// payment with its expiry (created plus the approval timeout), the spend log
// newest first, and bots named from the address book where it knows them.
func TestReadBRMCPGoldenView(t *testing.T) {
	stubBRMCP(t, brmcpGoldenState(), nil, []map[string]any{
		contactEntry(brmcpBotA, "quotebot-nick", "quotebot", "Quote Bot"),
	})
	_, js := readBRMCPJSON(t)

	want := strings.NewReplacer("<A>", brmcpBotA, "<B>", brmcpBotB).Replace(
		`{"enabled":true,"mode":"approval","perCallCapDcr":0.5,"perDayCapDcr":2,` +
			`"approvalTimeoutSecs":120,"todayDcr":0.45,` +
			`"lastDenied":{"ip":"9.9.9.9","at":"2026-09-01T10:00:00Z"},` +
			`"pending":[{"id":"p1","bot":"<A>","botNick":"quotebot","tool":"quote","amountDcr":0.1,` +
			`"created":"2023-11-14T22:13:20Z","expiresAt":"2023-11-14T22:15:20Z"}],` +
			`"spend":[` +
			`{"ts":"2023-11-14T22:18:20Z","bot":"<A>","botNick":"quotebot","tool":"quote","rail":"ln","amountDcr":0.1,"status":"pending"},` +
			`{"ts":"2023-11-14T22:16:40Z","bot":"<B>","tool":"image","rail":"ln","amountDcr":0.25,"status":"failed","err":"no route"},` +
			`{"ts":"2023-11-14T22:15:00Z","bot":"<A>","botNick":"quotebot","tool":"quote","rail":"ln","amountDcr":0.1,"status":"ok"}` +
			`]}`)
	if js != want {
		t.Errorf("view JSON\n got: %s\nwant: %s", js, want)
	}
	for _, leak := range []string{"token", "super-secret", "allowedBots", "allowedIps", "10.0.0.7", "tipWaitSecs"} {
		if strings.Contains(js, leak) {
			t.Errorf("view JSON carries %q", leak)
		}
	}
}

// The spend log is bounded and reversed: brclientd's oldest-first list of 60
// comes back as the newest 50, newest first. A bridge with nothing parked
// still serializes "pending":[] rather than null.
func TestReadBRMCPSpendNewestFirstAndBounded(t *testing.T) {
	st := brmcpGoldenState()
	st.pending = nil
	st.spend.Entries = nil
	for i := range 60 {
		st.spend.Entries = append(st.spend.Entries, types.BRMCPSpendEntry{
			TS: 1700000000 + int64(i), Bot: brmcpBotA, Tool: "quote", Rail: "ln", AmountDcr: 0.01, Status: "ok",
		})
	}
	stubBRMCP(t, st, nil, nil)
	view, js := readBRMCPJSON(t)
	if len(view.Spend) != brmcpSpendMax {
		t.Fatalf("spend len = %d, want %d", len(view.Spend), brmcpSpendMax)
	}
	if view.Spend[0].TS != brmcpTime(1700000059) {
		t.Errorf("first entry TS = %s, want the newest %s", view.Spend[0].TS, brmcpTime(1700000059))
	}
	if last := view.Spend[brmcpSpendMax-1].TS; last != brmcpTime(1700000010) {
		t.Errorf("last entry TS = %s, want %s", last, brmcpTime(1700000010))
	}
	if !strings.Contains(js, `"pending":[]`) {
		t.Errorf("empty pending must serialize as []: %s", js)
	}
}

// Bots are named from the address book: the operator's local alias first, else
// the contact's nick, matched without regard to uid case. The book is read
// only when the view has a bot to name, and a failed read names nobody rather
// than failing the resource.
func TestBRMCPBotNicks(t *testing.T) {
	calls := 0
	prev := loadBRContacts
	loadBRContacts = func(context.Context) ([]map[string]any, error) {
		calls++
		return []map[string]any{
			contactEntry(brmcpBotA, "quotebot-nick", "quotebot", "Quote Bot"),
			contactEntry(brmcpBotB, "imagebot", "", "Image Bot"),
			contactEntry("cccc1111cccc1111cccc1111cccc1111cccc1111cccc1111cccc1111cccc1111", "bystander", "", ""),
		}, nil
	}
	t.Cleanup(func() { loadBRContacts = prev })
	ctx := context.Background()

	st := brmcpGoldenState()
	st.spend.Entries[1].Bot = strings.ToUpper(brmcpBotB)
	got := brmcpBotNicks(ctx, st)
	want := map[string]string{brmcpBotA: "quotebot", brmcpBotB: "imagebot"}
	if !maps.Equal(got, want) {
		t.Errorf("nicks = %v, want %v", got, want)
	}
	if calls != 1 {
		t.Errorf("address book read %d times, want 1", calls)
	}
	if v := buildBRMCPView(st, got); v.Spend[1].BotNick != "imagebot" {
		t.Errorf("upper-case uid not named: %+v", v.Spend[1])
	}

	if n := brmcpBotNicks(ctx, brmcpState{settings: st.settings}); n != nil || calls != 1 {
		t.Errorf("nothing to name: nicks = %v, reads = %d; want nil and 1", n, calls)
	}

	loadBRContacts = func(context.Context) ([]map[string]any, error) { return nil, errors.New("brclientd down") }
	if n := brmcpBotNicks(ctx, st); len(n) != 0 {
		t.Errorf("failed lookup must name nobody, got %v", n)
	}
	if v := buildBRMCPView(st, nil); v.Pending[0].BotNick != "" || v.Pending[0].Bot != brmcpBotA {
		t.Errorf("unnamed bot must keep its uid: %+v", v.Pending[0])
	}
}

// An unreachable brclientd (down, or the bridge not built yet) is a state the
// widget renders, not an MCP error: the read succeeds with Error set, Enabled
// false and both lists present but empty, end to end through the SDK.
func TestReadBRMCPUnreachableIsAView(t *testing.T) {
	stubBRMCP(t, brmcpState{}, errors.New("brclientd /settings/mcpclient: HTTP 503: MCP engine not yet running"), nil)
	view, js := readBRMCPJSON(t)
	if view.Enabled {
		t.Error("unreachable bridge reported enabled")
	}
	if !strings.Contains(view.Error, "MCP engine not yet running") {
		t.Errorf("Error = %q", view.Error)
	}
	for _, want := range []string{`"pending":[]`, `"spend":[]`, `"enabled":false`} {
		if !strings.Contains(js, want) {
			t.Errorf("view JSON lacks %s: %s", want, js)
		}
	}

	a := testAgent("rbu", "bridge-watcher", map[string]bool{domainBRMCP: true})
	cs := connectTo(t, a)
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: resBRMCP})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(res.Contents) != 1 || res.Contents[0].URI != resBRMCP || res.Contents[0].MIMEType != "application/json" {
		t.Fatalf("contents = %+v", res.Contents)
	}
	if res.Contents[0].Text != js {
		t.Errorf("SDK read = %s\nwant %s", res.Contents[0].Text, js)
	}
}

// The poller wakes subscribers once per change: not on an identical re-read,
// not when only the error text of an unreachable daemon moves, and again after
// a reset. tick reports the fast cadence only for an enabled, answering bridge.
func TestBRMCPPollerNotifiesOncePerChange(t *testing.T) {
	var (
		st   brmcpState
		ferr error
	)
	prev := fetchBRMCPState
	fetchBRMCPState = func(context.Context) (brmcpState, error) { return st, ferr }
	t.Cleanup(func() { fetchBRMCPState = prev })

	notified := 0
	p := &brmcpPoller{notify: func(uri string) {
		if uri != resBRMCP {
			t.Errorf("notified %s, want %s", uri, resBRMCP)
		}
		notified++
	}}
	ctx := context.Background()
	expect := func(step string, wantFast bool, wantNotified int) {
		t.Helper()
		if fast := p.tick(ctx); fast != wantFast {
			t.Errorf("%s: tick fast = %v, want %v", step, fast, wantFast)
		}
		if notified != wantNotified {
			t.Errorf("%s: notified %d times, want %d", step, notified, wantNotified)
		}
	}

	st, ferr = brmcpGoldenState(), nil
	expect("first tick", true, 1)
	expect("same state", true, 1)

	st.pending = append(st.pending, types.BRMCPPending{ID: "p2", Bot: brmcpBotB, Tool: "image", AmountDcr: 0.2, Created: 1700000400})
	expect("new pending", true, 2)

	st.pending = []types.BRMCPPending{st.pending[1], st.pending[0]}
	expect("pending reordered", true, 2)

	st.pending = st.pending[:1]
	st.spend.Entries = append(st.spend.Entries, types.BRMCPSpendEntry{TS: 1700000500, Bot: brmcpBotB, Tool: "image", Rail: "ln", AmountDcr: 0.2, Status: "ok"})
	st.spend.TodayDcr += 0.2
	expect("pending settled", true, 3)

	st.settings.LastDenied = &types.BRMCPDenied{IP: "8.8.8.8", At: "2026-09-02T10:00:00Z"}
	expect("new denial", true, 4)

	st, ferr = brmcpState{}, errors.New("dial tcp: connection refused")
	expect("unreachable", false, 5)
	ferr = errors.New("brclientd /settings/mcpclient: HTTP 503: MCP engine not yet running")
	expect("unreachable, other text", false, 5)

	st, ferr = brmcpGoldenState(), nil
	st.settings.Enabled = false
	expect("bridge disabled", false, 6)

	p.reset()
	expect("after reset", false, 7)
	expect("settled after reset", false, 7)
}

// start and stop are idempotent and the loop ticks on start; a stopped poller
// can be started again, as the listener toggle does.
func TestBRMCPPollerStartStop(t *testing.T) {
	prev := fetchBRMCPState
	fetchBRMCPState = func(context.Context) (brmcpState, error) { return brmcpState{}, errors.New("down") }
	t.Cleanup(func() { fetchBRMCPState = prev })

	ticked := make(chan struct{}, 8)
	p := &brmcpPoller{notify: func(string) { ticked <- struct{}{} }}
	running := func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.cancel != nil
	}
	waitTick := func(step string) {
		t.Helper()
		select {
		case <-ticked:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: poller did not tick", step)
		}
	}

	p.start()
	p.start()
	waitTick("start")
	if !running() {
		t.Fatal("start must leave the poller running")
	}
	p.stop()
	p.stop()
	if running() {
		t.Fatal("stop must clear the poller")
	}

	// The first tick after a restart re-notifies only if the state moved; the
	// fingerprint is kept across stop, so a reset is what forces it.
	p.reset()
	p.start()
	waitTick("restart")
	p.stop()
	if running() {
		t.Fatal("second stop must clear the poller")
	}
}
