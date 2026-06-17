// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// Bison Relay oversight loop. When the user enables it and selects a contact,
// every fund-moving agent action must be approved by the user over a BR DM
// before it executes, and successful fund moves (and tripwire blocks) are
// reported to that contact. The contact is a hex peer UID the dashboard has
// already completed key exchange with. Disabled by default; spend caps and the
// tripwire are unchanged and apply on top of (never replaced by) approval.

const approvalTimeout = 2 * time.Minute

var (
	// These denials are NOT spend-limit violations, so isOverLimit reports false
	// and the tripwire does not fire: a refused approval is an operator choice,
	// not a compromised agent.
	errApprovalDenied      = errors.New("the operator denied this spend over Bison Relay")
	errApprovalTimeout     = errors.New("operator approval timed out over Bison Relay; the spend was not made")
	errApprovalUnreachable = errors.New("could not reach the operator over Bison Relay for approval; the spend was refused")
)

// oversightConfig reports whether the BR oversight loop is enabled and the hex
// UID of the contact that receives approval requests and spend notifications.
func oversightConfig() (enabled bool, contact string) {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return false, ""
	}
	_, _ = gc.Get(config.KeyMCPNotifyEnabled, &enabled)
	_, _ = gc.Get(config.KeyMCPNotifyContact, &contact)
	return enabled, strings.TrimSpace(contact)
}

// SetOversightConfig persists the BR oversight on/off state and target contact.
func SetOversightConfig(enabled bool, contact string) error {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	if err := gc.Set(config.KeyMCPNotifyEnabled, enabled); err != nil {
		return err
	}
	if err := gc.Set(config.KeyMCPNotifyContact, strings.TrimSpace(contact)); err != nil {
		return err
	}
	return gc.Save()
}

// OversightConfig is the dashboard-facing view of the oversight settings.
type OversightConfig struct {
	Enabled bool   `json:"enabled"`
	Contact string `json:"contact"`
}

// Oversight returns the current oversight settings for the Settings UI.
func Oversight() OversightConfig {
	enabled, contact := oversightConfig()
	return OversightConfig{Enabled: enabled, Contact: contact}
}

// approvalRegistry tracks fund-move approvals awaiting an operator reply.
type approvalRegistry struct {
	mu      sync.Mutex
	pending map[string]chan bool
}

func newApprovalRegistry() *approvalRegistry {
	return &approvalRegistry{pending: map[string]chan bool{}}
}

func (r *approvalRegistry) register(id string) chan bool {
	ch := make(chan bool, 1)
	r.mu.Lock()
	r.pending[id] = ch
	r.mu.Unlock()
	return ch
}

func (r *approvalRegistry) clear(id string) {
	r.mu.Lock()
	delete(r.pending, id)
	r.mu.Unlock()
}

// resolve delivers a verdict to the pending approval with this id. Returns true
// if a matching request was waiting.
func (r *approvalRegistry) resolve(id string, ok bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := r.pending[id]
	if ch == nil {
		return false
	}
	delete(r.pending, id)
	ch <- ok
	return true
}

var approvals = newApprovalRegistry()

// gateApproval blocks for the operator's approval over Bison Relay before a fund
// move. It is a no-op (proceed) when oversight is disabled or no contact is set.
// When enabled it DMs the contact and waits for an approve/deny reply, the
// timeout, or the agent's request being cancelled; it fails closed (refuse) if
// the operator cannot be reached. The caller must hold no locks: this blocks.
func gateApproval(ctx context.Context, agentID, action string) error {
	enabled, contact := oversightConfig()
	if !enabled || contact == "" {
		return nil
	}
	id, err := randomHex(2) // short, so the operator can type it back
	if err != nil {
		return errApprovalUnreachable
	}
	ch := approvals.register(id)
	defer approvals.clear(id)

	name := reg.name(agentID)
	if name == "" {
		name = agentID
	}
	mins := int(approvalTimeout / time.Minute)
	msg := fmt.Sprintf("dcrpulse approval [%s]: agent %q wants to %s. Reply \"yes %s\" to approve or \"no %s\" to deny within %d min.",
		id, name, action, id, id, mins)

	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	sendErr := rpc.BrclientdSendPM(sctx, contact, msg)
	cancel()
	if sendErr != nil {
		return errApprovalUnreachable
	}

	select {
	case ok := <-ch:
		if !ok {
			return errApprovalDenied
		}
		return nil
	case <-time.After(approvalTimeout):
		return errApprovalTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}

// startOversightConsumer watches incoming Bison Relay PMs for approve/deny
// replies from the configured contact and resolves pending approvals. Runs for
// the process lifetime; started once alongside the resource feeds.
func startOversightConsumer() {
	ch, _ := services.Bisonrelay().Subscribe(64)
	for evt := range ch {
		if evt.Type != "pm" {
			continue
		}
		var p struct {
			From    string `json:"from"`
			Message string `json:"message"`
		}
		if json.Unmarshal(evt.Payload, &p) != nil {
			continue
		}
		_, contact := oversightConfig()
		if contact == "" || !strings.EqualFold(strings.TrimSpace(p.From), contact) {
			continue
		}
		handleApprovalReply(p.Message)
	}
}

// handleApprovalReply parses an operator reply and resolves the pending approval
// whose id the reply carries. The id is REQUIRED: "yes <id>" / "no <id>" (also
// approve/deny/ok/y/n, case-insensitive). A bare verdict with no id is ignored,
// so a stale or late reply can never resolve a request it does not name.
func handleApprovalReply(text string) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(fields) < 2 {
		return
	}
	verdict, ok := parseVerdict(fields[0])
	if !ok {
		return
	}
	for _, f := range fields[1:] {
		if f != "" && approvals.resolve(f, verdict) {
			return
		}
	}
}

func parseVerdict(s string) (verdict bool, ok bool) {
	switch s {
	case "yes", "y", "approve", "approved", "ok", "accept":
		return true, true
	case "no", "n", "deny", "denied", "reject", "cancel":
		return false, true
	}
	return false, false
}

// notifySpend reports a spend outcome to the configured contact when oversight is
// on. Only fund movements (result "ok" with a positive amount) and tripwire
// blocks are reported; routine denials are not, since the operator already saw
// the approval request. Sent in the background so it never blocks the spend path.
func notifySpend(e AuditEntry) {
	enabled, contact := oversightConfig()
	if !enabled || contact == "" {
		return
	}
	var msg string
	switch {
	case e.Result == "ok" && e.AmountDCR > 0:
		msg = fmt.Sprintf("dcrpulse: agent %q sent %.8f DCR via %s%s. %s",
			e.Agent, e.AmountDCR, e.Tool, targetSuffix(e.Target), detailSuffix(e.Detail))
	case e.Result == "blocked":
		msg = fmt.Sprintf("dcrpulse ALERT: agent %q was BLOCKED attempting %s (%.8f DCR%s): %s",
			e.Agent, e.Tool, e.AmountDCR, targetSuffix(e.Target), e.Detail)
	default:
		return
	}
	go func(to, body string) {
		sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = rpc.BrclientdSendPM(sctx, to, body)
	}(contact, msg)
}

func targetSuffix(t string) string {
	if t == "" {
		return ""
	}
	return " to " + t
}

func detailSuffix(d string) string {
	if d == "" {
		return ""
	}
	return "txid " + d
}

// dcrAmountStr formats atoms as a DCR string for approval and notification DMs.
func dcrAmountStr(atoms int64) string {
	return fmt.Sprintf("%.8f DCR", dcrutil.Amount(atoms).ToCoin())
}
