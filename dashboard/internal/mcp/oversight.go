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

// maxPendingPerAgent bounds how many approvals one agent can hold open. One is
// the normal case and calls are served concurrently, so a few is plausible;
// beyond that an agent is either malfunctioning or flooding the operator, who
// pays a DM for each.
const maxPendingPerAgent = 3

var (
	// These denials are NOT spend-limit violations, so isOverLimit reports false
	// and the tripwire does not fire: a refused approval is an operator choice,
	// not a compromised agent.
	errApprovalDenied      = errors.New("the operator denied this spend over Bison Relay")
	errApprovalFrozen      = errors.New("the operator denied this spend and blocked this agent over Bison Relay")
	errApprovalTimeout     = errors.New("operator approval timed out over Bison Relay; the spend was not made")
	errApprovalUnreachable = errors.New("could not reach the operator over Bison Relay for approval; the spend was refused")
	errApprovalRevoked     = errors.New("the agent's authority was withdrawn while this spend awaited approval")
	errApprovalPending     = errors.New("this agent already has the most approvals it may keep waiting on the operator")
)

// approvalVerdict is the operator's reply to a fund-move approval request.
// freeze means deny AND block the agent (revoke its grant and block its token,
// the same effect as the tripwire) for an emergency stop over Bison Relay.
type approvalVerdict struct {
	approved bool
	freeze   bool
	// cancelled marks an approval failed because the agent lost its authority
	// while waiting, rather than by any reply from the operator.
	cancelled bool
}

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

// newApprovalID generates an approval id. A variable so a test can force the
// collision the short id makes possible but random generation almost never hits.
var newApprovalID = func() (string, error) { return randomHex(2) } // short, so the operator can type it back

// pendingApproval is one approval awaiting a reply, and the agent it is for.
type pendingApproval struct {
	ch    chan approvalVerdict
	agent string
}

// retiredApproval is an id that has left the pending set, kept for one
// approvalTimeout. It lets a reply arriving after its request gave up still name
// the agent, and keeps the id from being reissued while that can happen.
type retiredApproval struct {
	agent string
	at    time.Time
	// abandoned marks a request that ended with no reply, so it still counts
	// against the agent's cap: the operator was DMed and never got to answer.
	abandoned bool
}

// approvalRegistry tracks fund-move approvals awaiting an operator reply.
type approvalRegistry struct {
	mu      sync.Mutex
	pending map[string]pendingApproval
	retired map[string]retiredApproval
	now     func() time.Time
}

func newApprovalRegistry() *approvalRegistry {
	return &approvalRegistry{
		pending: map[string]pendingApproval{},
		retired: map[string]retiredApproval{},
		now:     time.Now,
	}
}

// pruneLocked forgets ids retired longer ago than a reply could still arrive.
func (r *approvalRegistry) pruneLocked() {
	for id, t := range r.retired {
		if r.now().Sub(t.at) >= approvalTimeout {
			delete(r.retired, id)
		}
	}
}

// retireLocked moves an entry out of the pending set, keeping its id reserved.
func (r *approvalRegistry) retireLocked(id string, p pendingApproval, abandoned bool) {
	delete(r.pending, id)
	r.retired[id] = retiredApproval{agent: p.agent, at: r.now(), abandoned: abandoned}
}

// agentFor names the agent an id belongs to, waiting or recently retired. The
// caller must not hold the lock when acting on the result: freezing takes others.
func (r *approvalRegistry) agentFor(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.pending[id]; ok {
		return p.agent, true
	}
	if t, ok := r.retired[id]; ok {
		return t.agent, true
	}
	return "", false
}

// register reserves a fresh id for this agent. It never replaces a live entry:
// an id that silently changed hands would route the operator's reply to a
// request they were never shown.
func (r *approvalRegistry) register(agentID string) (string, chan approvalVerdict, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	held := 0
	for _, p := range r.pending {
		if p.agent == agentID {
			held++
		}
	}
	for _, t := range r.retired {
		if t.agent == agentID && t.abandoned {
			held++
		}
	}
	if held >= maxPendingPerAgent {
		return "", nil, errApprovalPending
	}
	for i := 0; i < 8; i++ {
		id, err := newApprovalID()
		if err != nil {
			return "", nil, errApprovalUnreachable
		}
		if _, taken := r.pending[id]; taken {
			continue
		}
		if _, reserved := r.retired[id]; reserved {
			continue
		}
		ch := make(chan approvalVerdict, 1)
		r.pending[id] = pendingApproval{ch: ch, agent: agentID}
		return id, ch, nil
	}
	return "", nil, errApprovalUnreachable
}

// cancelAll fails every approval in flight, for the freeze-all kill switch: it
// must reach a waiting approval whether or not that agent still holds a grant.
func (r *approvalRegistry) cancelAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, p := range r.pending {
		r.retireLocked(id, p, false)
		p.ch <- approvalVerdict{cancelled: true}
	}
}

// cancelAgent fails every approval this agent is waiting on. Deleting before the
// send is what keeps it safe: resolve does the same, so a verdict and a cancel
// can never both write to one buffered channel.
func (r *approvalRegistry) cancelAgent(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, p := range r.pending {
		if p.agent != agentID {
			continue
		}
		r.retireLocked(id, p, false)
		p.ch <- approvalVerdict{cancelled: true}
	}
}

// clear retires a request that ended with no reply. A no-op once the entry has
// left, so the deferred call after a resolve does not overwrite its record.
func (r *approvalRegistry) clear(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.pending[id]; ok {
		r.retireLocked(id, p, true)
	}
}

// resolve delivers a verdict to the pending approval with this id. Returns true
// if a matching request was waiting.
func (r *approvalRegistry) resolve(id string, v approvalVerdict) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pending[id]
	if !ok {
		return false
	}
	r.retireLocked(id, p, false)
	p.ch <- v
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
	id, ch, err := approvals.register(agentID)
	if err != nil {
		return err
	}
	defer approvals.clear(id)

	name := reg.name(agentID)
	if name == "" {
		name = agentID
	}
	mins := int(approvalTimeout / time.Minute)
	msg := fmt.Sprintf("dcrpulse approval [%s]: agent %q wants to %s. Reply \"yes %s\" to approve, \"no %s\" to deny, or \"no %s freeze\" to deny and block this agent. Expires in %d min.",
		id, name, action, id, id, id, mins)

	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	sendErr := rpc.BrclientdSendPM(sctx, contact, msg)
	cancel()
	if sendErr != nil {
		return errApprovalUnreachable
	}

	select {
	case v := <-ch:
		if v.cancelled {
			return errApprovalRevoked
		}
		if v.freeze {
			// The freeze itself is applied by the reply consumer, so it lands
			// even when no request is still waiting to hear it.
			return errApprovalFrozen
		}
		if !v.approved {
			return errApprovalDenied
		}
		// The grant was checked before the operator was asked. Revocation
		// cancels a waiting approval, but expiry is lazy and cancels nothing, so
		// confirm the agent may still spend before letting it through.
		if err := grants.precheckGrant(agentID, time.Now()); err != nil {
			return err
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
// so a stale or late reply can never resolve a request it does not name. The
// word "freeze" (or "block") anywhere in the reply denies the spend AND blocks
// the agent, for an emergency stop.
func handleApprovalReply(text string) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(fields) < 2 {
		return
	}
	verdict, ok := parseVerdict(fields[0])
	if !ok {
		return
	}
	freeze := false
	for _, f := range fields[1:] {
		if f == "freeze" || f == "block" {
			freeze = true
			verdict = false // freezing always denies the spend
		}
	}
	for _, f := range fields[1:] {
		if f == "" || f == "freeze" || f == "block" {
			continue
		}
		resolved := approvals.resolve(f, approvalVerdict{approved: verdict, freeze: freeze})
		if freeze {
			// Applied here rather than by the waiting request: a freeze that
			// arrives after the request gave up, or that loses the race with its
			// own timeout, must still block the agent it names.
			if agentID, ok := approvals.agentFor(f); ok {
				freezeAgent(agentID)
				return
			}
		}
		if resolved {
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

// freezeAgent denies-and-blocks an agent: it revokes the agent's spend grant and
// blocks its token (the same effect as the tripwire), so the agent cannot spend
// or reconnect until the user unblocks it in the dashboard. Triggered when an
// approval reply includes "freeze".
func freezeAgent(agentID string) {
	grants.revoke(agentID)
	reg.block(agentID)
	_ = saveAgents()
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

// errOversightContact refuses an agent's attempt to reach the operator's
// oversight contact.
var errOversightContact = errors.New("this contact receives dcrpulse's own approval requests and cannot be addressed by an agent")

// refuseOversightContact reports whether target names the configured oversight
// contact. The BR tools accept a nick, alias or hex uid and let brclientd do the
// resolving, so comparing against the stored hex alone would be bypassed by
// passing the nick; the contact's own names are resolved and matched too.
// Nothing is refused when oversight is off, and a contact lookup that fails
// refuses rather than guessing: only messages to one contact are affected.
func refuseOversightContact(ctx context.Context, target string) error {
	enabled, contact := oversightConfig()
	if !enabled || contact == "" {
		return nil
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	if strings.EqualFold(target, contact) {
		return errOversightContact
	}
	entries, err := brContactEntries(ctx)
	if err != nil {
		return fmt.Errorf("check the oversight contact: %w", err)
	}
	if namesOversightContact(target, contact, entries) {
		return errOversightContact
	}
	return nil
}

// namesOversightContact reports whether target is any name the oversight contact
// answers to. brclientd resolves a nick or alias for us, so matching the stored
// hex alone would leave those as a way around the refusal.
func namesOversightContact(target, contact string, entries []map[string]any) bool {
	// Trimmed here rather than relying on the caller: an untrimmed nick would
	// otherwise slip past. An empty target cannot match, since every name is
	// checked non-empty below.
	target = strings.TrimSpace(target)
	for _, entry := range entries {
		uid, nick, alias, name := brContactStrings(entry)
		if !strings.EqualFold(strings.TrimSpace(uid), strings.TrimSpace(contact)) {
			continue
		}
		for _, own := range []string{uid, nick, alias, name} {
			if own = strings.TrimSpace(own); own != "" && strings.EqualFold(target, own) {
				return true
			}
		}
	}
	return false
}
