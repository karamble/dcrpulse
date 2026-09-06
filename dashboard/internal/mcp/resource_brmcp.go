// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// The BR-MCP bridge resource is a read-only window onto brclientd's Bison
// Relay MCP client engine: the bot tool payments parked for the operator's
// approval and the payments already made. It exists so a widget holding an
// MCP token can show the operator what is waiting and what was spent. It is
// deliberately a resource and not a tool: an agent can watch, but approving or
// denying a payment stays in the dashboard, and the bridge's bearer token and
// allow lists never leave the settings proxy.

const (
	// brmcpSpendMax bounds the spend log in the view; brclientd keeps up to
	// 1000 entries and a watcher only needs the recent ones.
	brmcpSpendMax = 50

	// brmcpReadTimeout bounds one read of the three bridge endpoints so a
	// stalled brclientd cannot park a resource read or the poller.
	brmcpReadTimeout = 10 * time.Second

	// Poll cadence: fast while the bridge is enabled and answering, since a
	// pending approval is time-limited and the widget should catch it early;
	// slow when it is off or unreachable, where nothing changes.
	brmcpPollFast = 3 * time.Second
	brmcpPollSlow = 30 * time.Second
)

// brmcpPendingView is one payment awaiting the operator's approval. Created
// and ExpiresAt are RFC3339 UTC; the payment is denied automatically at
// ExpiresAt (created plus the bridge's approval timeout).
type brmcpPendingView struct {
	ID        string  `json:"id"`
	Bot       string  `json:"bot"`
	BotNick   string  `json:"botNick,omitempty"`
	Tool      string  `json:"tool"`
	AmountDcr float64 `json:"amountDcr"`
	Created   string  `json:"created"`
	ExpiresAt string  `json:"expiresAt"`
}

// brmcpSpendView is one spend log entry. TS is RFC3339 UTC.
type brmcpSpendView struct {
	TS        string  `json:"ts"`
	Bot       string  `json:"bot"`
	BotNick   string  `json:"botNick,omitempty"`
	Tool      string  `json:"tool"`
	Rail      string  `json:"rail"`
	AmountDcr float64 `json:"amountDcr"`
	Status    string  `json:"status,omitempty"`
	Err       string  `json:"err,omitempty"`
}

// brmcpView is the resource payload. Error is set, with Enabled false and
// the lists empty, when brclientd could not be read (down, or the bridge not
// built yet); the token and allow lists are not part of the view.
type brmcpView struct {
	Enabled             bool               `json:"enabled"`
	Mode                string             `json:"mode,omitempty"`
	PerCallCapDcr       float64            `json:"perCallCapDcr"`
	PerDayCapDcr        float64            `json:"perDayCapDcr"`
	ApprovalTimeoutSecs int                `json:"approvalTimeoutSecs"`
	TodayDcr            float64            `json:"todayDcr"`
	LastDenied          *types.BRMCPDenied `json:"lastDenied,omitempty"`
	Pending             []brmcpPendingView `json:"pending"`
	Spend               []brmcpSpendView   `json:"spend"`
	Error               string             `json:"error,omitempty"`
}

// brmcpState is one consistent read of the bridge as brclientd reports it.
type brmcpState struct {
	settings types.BRMCPSettings
	pending  []types.BRMCPPending
	spend    types.BRMCPSpend
}

// fetchBRMCPState reads the three bridge endpoints, settings first: a
// brclientd whose bridge is not built yet refuses that one with a 503, so the
// read fails there rather than after two more round trips. A variable so the
// resource and the poller can be tested without a daemon.
var fetchBRMCPState = func(ctx context.Context) (brmcpState, error) {
	var st brmcpState
	var err error
	if st.settings, err = services.FetchBRMCPSettings(ctx); err != nil {
		return brmcpState{}, err
	}
	if st.pending, err = services.FetchBRMCPPending(ctx); err != nil {
		return brmcpState{}, err
	}
	if st.spend, err = services.FetchBRMCPSpend(ctx); err != nil {
		return brmcpState{}, err
	}
	return st, nil
}

// loadBRContacts reads the address book used to name the bots. A variable for
// the same reason as fetchBRMCPState.
var loadBRContacts = brContactEntries

// readBRMCP serves the resource. It never returns an error: the widget
// watching it needs to render "bridge unreachable" as a state, not receive an
// MCP error it cannot tell apart from a revoked grant, so a failed read
// becomes a view with Error set and everything else empty.
func readBRMCP(ctx context.Context) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, brmcpReadTimeout)
	defer cancel()
	st, err := fetchBRMCPState(ctx)
	if err != nil {
		return brmcpView{
			Pending: []brmcpPendingView{},
			Spend:   []brmcpSpendView{},
			Error:   clampAuditField(err.Error(), auditDetailMax),
		}, nil
	}
	return buildBRMCPView(st, brmcpBotNicks(ctx, st)), nil
}

// buildBRMCPView shapes the daemon state for a watcher: settings without the
// token and allow lists, pending payments with their expiry spelled out, and
// the spend log newest first and bounded. nicks maps a bot uid to a display
// name; a bot with no entry is shown by uid alone. The lists are never nil.
func buildBRMCPView(st brmcpState, nicks map[string]string) brmcpView {
	s := st.settings
	v := brmcpView{
		Enabled:             s.Enabled,
		Mode:                s.Mode,
		PerCallCapDcr:       s.PerCallCapDcr,
		PerDayCapDcr:        s.PerDayCapDcr,
		ApprovalTimeoutSecs: s.ApprovalTimeoutSecs,
		TodayDcr:            st.spend.TodayDcr,
		LastDenied:          s.LastDenied,
		Pending:             make([]brmcpPendingView, 0, len(st.pending)),
		Spend:               make([]brmcpSpendView, 0, min(len(st.spend.Entries), brmcpSpendMax)),
	}
	for _, p := range st.pending {
		v.Pending = append(v.Pending, brmcpPendingView{
			ID:        p.ID,
			Bot:       p.Bot,
			BotNick:   nicks[brmcpNickKey(p.Bot)],
			Tool:      p.Tool,
			AmountDcr: p.AmountDcr,
			Created:   brmcpTime(p.Created),
			ExpiresAt: brmcpTime(p.Created + int64(s.ApprovalTimeoutSecs)),
		})
	}
	entries := st.spend.Entries
	for i := len(entries) - 1; i >= 0 && len(v.Spend) < brmcpSpendMax; i-- {
		e := entries[i]
		v.Spend = append(v.Spend, brmcpSpendView{
			TS:        brmcpTime(e.TS),
			Bot:       e.Bot,
			BotNick:   nicks[brmcpNickKey(e.Bot)],
			Tool:      e.Tool,
			Rail:      e.Rail,
			AmountDcr: e.AmountDcr,
			Status:    e.Status,
			Err:       e.Err,
		})
	}
	return v
}

func brmcpTime(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

// brmcpNickKey normalises a hex uid for the nick map, since the bridge and the
// address book need not agree on letter case.
func brmcpNickKey(uid string) string { return strings.ToLower(uid) }

// brmcpBotNicks names the bots that appear in the view from the address book,
// so the widget can show "quotebot" rather than a hex uid. The local alias
// wins when the operator set one, else the contact's own nick, matching how
// the Bison Relay UI labels a contact. The address book is read once per view
// and only when there is a bot to name; a failed read leaves every bot unnamed
// rather than failing the resource.
func brmcpBotNicks(ctx context.Context, st brmcpState) map[string]string {
	want := map[string]bool{}
	for _, p := range st.pending {
		if p.Bot != "" {
			want[brmcpNickKey(p.Bot)] = true
		}
	}
	entries := st.spend.Entries
	for i := len(entries) - 1; i >= 0 && i >= len(entries)-brmcpSpendMax; i-- {
		if entries[i].Bot != "" {
			want[brmcpNickKey(entries[i].Bot)] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	contacts, err := loadBRContacts(ctx)
	if err != nil {
		return nil
	}
	nicks := make(map[string]string, len(want))
	for _, c := range contacts {
		uid, nick, alias, _ := brContactStrings(c)
		key := brmcpNickKey(uid)
		if !want[key] {
			continue
		}
		switch {
		case alias != "":
			nicks[key] = alias
		case nick != "":
			nicks[key] = nick
		}
	}
	return nicks
}

// brmcpFingerprint condenses a read to what a watcher would act on, so the
// poller notifies on a real change and stays quiet across identical polls.
// Every failed read hashes the same: a daemon that is down does not become
// news again because its error text moved.
func brmcpFingerprint(st brmcpState, err error) [32]byte {
	if err != nil {
		return sha256.Sum256([]byte("err"))
	}
	h := sha256.New()
	s := st.settings
	deniedAt := ""
	if s.LastDenied != nil {
		deniedAt = s.LastDenied.At
	}
	fmt.Fprintf(h, "%t\x00%s\x00%g\x00%g\x00%d\x00%s\x00",
		s.Enabled, s.Mode, s.PerCallCapDcr, s.PerDayCapDcr, s.ApprovalTimeoutSecs, deniedAt)
	ids := make([]string, 0, len(st.pending))
	for _, p := range st.pending {
		ids = append(ids, p.ID)
	}
	slices.Sort(ids)
	for _, id := range ids {
		fmt.Fprintf(h, "%s\x00", id)
	}
	entries := st.spend.Entries
	fmt.Fprintf(h, "\x01%d\x00%g\x00", len(entries), st.spend.TodayDcr)
	if n := len(entries); n > 0 {
		e := entries[n-1]
		fmt.Fprintf(h, "%d\x00%s\x00%s\x00%s\x00%s\x00", e.TS, e.Bot, e.Tool, e.Status, e.Err)
	}
	unsettled := 0
	for _, e := range entries {
		if e.Status == "pending" {
			unsettled++
		}
	}
	fmt.Fprintf(h, "%d", unsettled)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// brmcpPoller polls brclientd for the bridge state while the MCP listener is
// up and fans out a resource-updated notification when the fingerprint moves.
// It polls rather than subscribes because brclientd exposes no bridge event
// stream, and it runs whenever the listener does because the SDK keeps
// resource subscriptions private and drops them silently on disconnect, so
// there is no subscriber count to gate on; a notify with no subscriber is a
// no-op.
type brmcpPoller struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	last   [32]byte
	primed bool
	notify func(uri string)
}

var brmcpFeed = &brmcpPoller{notify: notifyResourceUpdated}

// start begins polling. A no-op while already running.
func (p *brmcpPoller) start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.run(ctx)
}

// stop ends the poll loop. Idempotent.
func (p *brmcpPoller) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel == nil {
		return
	}
	p.cancel()
	p.cancel = nil
}

// reset forgets the last fingerprint so the next tick notifies whatever it
// reads: after a wallet change brclientd serves a different bridge, and a
// subscriber's last read describes the old one.
func (p *brmcpPoller) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.primed = false
}

func (p *brmcpPoller) run(ctx context.Context) {
	for {
		wait := brmcpPollSlow
		if p.tick(ctx) {
			wait = brmcpPollFast
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// tick reads the bridge once and notifies when the fingerprint differs from
// the previous tick's, or on the first tick after start or reset. It reports
// whether the bridge is enabled and answering, which sets the poll cadence.
func (p *brmcpPoller) tick(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, brmcpReadTimeout)
	defer cancel()
	st, err := fetchBRMCPState(ctx)
	fp := brmcpFingerprint(st, err)
	p.mu.Lock()
	changed := !p.primed || fp != p.last
	p.last, p.primed = fp, true
	p.mu.Unlock()
	if changed {
		p.notify(resBRMCP)
	}
	return err == nil && st.settings.Enabled
}
