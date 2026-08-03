// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dcrpulse/internal/config"
)

// Shared-wallet records persist per wallet in msig.json next to the
// wallet's config.json. The wallet database itself keeps the imported
// script, but it cannot enumerate scripts, name cosigners or survive a
// seed restore, so this registry is the durable source of truth for
// everything except keys.

// storeSchemaVersion 2 added the HD ladder fields. Documents are
// forward-readable (unknown JSON fields are dropped on the next save),
// which is exactly why openStore refuses files written by a NEWER build:
// silently stripping a future schema's fields would destroy state.
const storeSchemaVersion = 2

// seenTTL keeps journaled mids and sent outbox items past the BR server's
// 7 day delivery horizon so a replayed frame can never look new.
const seenTTL = 30 * 24 * time.Hour

// Path resolution seams; tests point them at temp directories.
var (
	walletDirFn  = config.WalletDir
	walletsDirFn = config.WalletsDir
)

// Wallet record roles.
const (
	RoleInitiator = "initiator"
	RoleCosigner  = "cosigner"
)

// Wallet record statuses. Initiators move inviting, activating, active.
// Cosigners move invited, accepted, pending_import, active; pending_import
// holds a fully verified roster whose script import waits for the owning
// wallet to become the active one.
const (
	StatusInviting      = "inviting"
	StatusActivating    = "activating"
	StatusActive        = "active"
	StatusInvited       = "invited"
	StatusAccepted      = "accepted"
	StatusPendingImport = "pending_import"
	StatusDeclined      = "declined"
	StatusFailed        = "failed"
)

// Per-peer handshake states tracked by the initiator.
const (
	PeerInvited    = "invited"
	PeerAccepted   = "accepted"
	PeerRosterSent = "roster_sent"
	PeerReady      = "ready"
	PeerDeclined   = "declined"
)

// Outbox item states.
const (
	OutboxSending = "sending"
	OutboxSent    = "sent"
)

// Proposal statuses. A proposer collects signatures, then broadcasts; a
// cosigner sees an incoming request it can sign or reject. Superseded
// marks a proposal whose inputs were spent by a different transaction.
const (
	ProposalCollecting = "collecting"
	ProposalReady      = "ready"
	ProposalBroadcast  = "broadcast"
	ProposalConfirmed  = "confirmed"
	ProposalIncoming   = "incoming"
	ProposalSigned     = "signed"
	ProposalDeclined   = "declined"
	ProposalFailed     = "failed"
	ProposalAborted    = "aborted"
	ProposalSuperseded = "superseded"
)

// Per-hop states in a proposer's cosigner queue.
const (
	HopPending  = "pending"
	HopSent     = "sent"
	HopSigned   = "signed"
	HopDeclined = "declined"
	HopTimeout  = "timeout"
	HopSkipped  = "skipped"
)

// OwnKey records this wallet's contribution and its derivation
// coordinates for the backup card.
type OwnKey struct {
	PubKey  string `json:"pubKey"`
	Address string `json:"address"`
	Account uint32 `json:"account"`
	Branch  uint32 `json:"branch"`
	Index   uint32 `json:"index"`
}

// OwnHDKey records this wallet's HD contribution: the dedicated
// account's number and its extended public key. The xpub doubles as the
// restore-time ownership proof, since the same seed always derives the
// same xpub at the same account.
type OwnHDKey struct {
	Xpub    string `json:"xpub"`
	Account uint32 `json:"account"`
}

// CursorState tracks one branch of an HD wallet's ladder. Next is the
// next index to hand out, ImportedThrough is one past the highest index
// whose script this wallet has imported, and LastUsed is one past the
// highest index observed used on-chain (0 = none). All three count raw
// child indices, skipped holes included.
type CursorState struct {
	Next            uint32 `json:"next"`
	ImportedThrough uint32 `json:"importedThrough"`
	LastUsed        uint32 `json:"lastUsed"`
}

// Peer is one cosigner as seen from this node.
type Peer struct {
	UID        string `json:"uid"`
	Nick       string `json:"nick"`
	PubKey     string `json:"pubkey,omitempty"`
	Xpub       string `json:"xpub,omitempty"`
	State      string `json:"state"`
	LastSeenTs int64  `json:"lastSeenTs,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// OutboxItem is one frame persisted before its first send attempt so a
// crash between the state change and the send never loses the message.
// Bodies are resent byte-identical; receiver mid journals absorb repeats.
type OutboxItem struct {
	MID   string `json:"mid"`
	ToUID string `json:"toUid"`
	Body  string `json:"body"`
	State string `json:"state"`
	Ts    int64  `json:"ts"`
}

// ProposalInput is one shared UTXO a proposal spends. Values are
// recorded so a cosigner can verify the fee without a chain lookup.
type ProposalInput struct {
	TxID  string `json:"txid"`
	Vout  uint32 `json:"vout"`
	Tree  int8   `json:"tree"`
	Atoms int64  `json:"atoms"`
}

// ProposalOutput is one recipient of a proposal.
type ProposalOutput struct {
	Address  string `json:"address"`
	Atoms    int64  `json:"atoms"`
	IsChange bool   `json:"isChange"`
}

// QueueHop is one cosigner's turn in the signing relay.
type QueueHop struct {
	UID      string `json:"uid"`
	Nick     string `json:"nick"`
	State    string `json:"state"`
	SentMID  string `json:"sentMid,omitempty"`
	SentAt   int64  `json:"sentAt,omitempty"`
	Deadline int64  `json:"deadline,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Proposal is one spend awaiting or having gathered signatures. The txid
// is stable from construction because Decred excludes signatures from it,
// so it keys the proposal for its whole life.
type Proposal struct {
	TxID       string           `json:"txid"`
	Role       string           `json:"role"`
	Status     string           `json:"status"`
	RawTx      string           `json:"rawTx"`
	SigCount   int              `json:"sigCount"`
	SignedBy   map[string]bool  `json:"signedBy,omitempty"`
	Inputs     []ProposalInput  `json:"inputs"`
	Outputs    []ProposalOutput `json:"outputs"`
	FeeAtoms   int64            `json:"feeAtoms"`
	Note       string           `json:"note,omitempty"`
	Reason     string           `json:"reason,omitempty"`
	Queue      []*QueueHop      `json:"queue,omitempty"`
	HopTTLSecs int64            `json:"hopTtlSecs,omitempty"`
	FromUID    string           `json:"fromUid,omitempty"`
	FromNick   string           `json:"fromNick,omitempty"`
	CreatedAt  int64            `json:"createdAt"`
	UpdatedAt  int64            `json:"updatedAt"`
}

// Terminal reports whether a proposal can never progress again.
func (p *Proposal) Terminal() bool {
	switch p.Status {
	case ProposalConfirmed, ProposalDeclined, ProposalFailed, ProposalAborted, ProposalSuperseded:
		return true
	}
	return false
}

// Live reports whether a proposal still holds a claim on its inputs.
func (p *Proposal) Live() bool {
	switch p.Status {
	case ProposalCollecting, ProposalReady, ProposalIncoming, ProposalSigned, ProposalBroadcast:
		return true
	}
	return false
}

// WalletRecord is one shared wallet's lifecycle state.
type WalletRecord struct {
	TempID        string   `json:"tempId"`
	Address       string   `json:"address,omitempty"`
	Label         string   `json:"label"`
	M             int      `json:"m"`
	N             int      `json:"n"`
	Network       string   `json:"network"`
	ScriptHex     string   `json:"scriptHex,omitempty"`
	RosterPubKeys []string `json:"rosterPubKeys,omitempty"`

	// HD ladder fields; empty on v1 single-address records. Address then
	// holds the external index-0 address, the wallet's identity.
	HD    bool         `json:"hd,omitempty"`
	Xpubs []string     `json:"xpubs,omitempty"`
	OwnHD *OwnHDKey    `json:"ownHd,omitempty"`
	Ext   *CursorState `json:"ext,omitempty"`
	Int   *CursorState `json:"int,omitempty"`

	Role          string  `json:"role"`
	Status        string  `json:"status"`
	FailReason    string  `json:"failReason,omitempty"`
	CreatedHeight int64   `json:"createdHeight,omitempty"`
	Own           *OwnKey `json:"own,omitempty"`
	InitiatorUID  string  `json:"initiatorUid,omitempty"`
	Peers         []*Peer `json:"peers"`
	CreatedAt     int64   `json:"createdAt"`
	UpdatedAt     int64   `json:"updatedAt"`

	Proposals map[string]*Proposal `json:"proposals,omitempty"`
}

// Terminal reports whether the record can never progress again.
func (r *WalletRecord) Terminal() bool {
	return r.Status == StatusDeclined || r.Status == StatusFailed
}

// peerByUID returns the peer entry for uid, or nil.
func (r *WalletRecord) peerByUID(uid string) *Peer {
	for _, p := range r.Peers {
		if p.UID == uid {
			return p
		}
	}
	return nil
}

type storeFile struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Wallets       map[string]*WalletRecord `json:"wallets"`
	ProcessedMids map[string]int64         `json:"processedMids"`
	Outbox        []*OutboxItem            `json:"outbox"`
}

// Store is one wallet's msig.json. All mutations rewrite the whole
// document atomically under the mutex; documents are a few KB.
type Store struct {
	mu         sync.Mutex
	path       string
	walletName string
	data       *storeFile
}

func openStore(path, walletName string) (*Store, error) {
	s := &Store{path: path, walletName: walletName, data: &storeFile{
		SchemaVersion: storeSchemaVersion,
		Wallets:       make(map[string]*WalletRecord),
		ProcessedMids: make(map[string]int64),
	}}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return s, nil
	}
	var f storeFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %v", path, err)
	}
	if f.SchemaVersion > storeSchemaVersion {
		return nil, fmt.Errorf("%s was written by a newer dcrpulse (schema %d, this build reads %d); upgrade instead of downgrading",
			path, f.SchemaVersion, storeSchemaVersion)
	}
	if f.Wallets == nil {
		f.Wallets = make(map[string]*WalletRecord)
	}
	if f.ProcessedMids == nil {
		f.ProcessedMids = make(map[string]int64)
	}
	f.SchemaVersion = storeSchemaVersion
	s.data = &f
	return s, nil
}

func (s *Store) saveLocked() error {
	return atomicWriteJSON(s.path, s.data)
}

// notifyChange, when set, is invoked with a record's stable id after a
// wallet or proposal mutation persists. The engine points it at the
// browser event bus: pages reload on the frame's arrival event before the
// engine finishes processing it, so without a commit signal they render
// the pre-frame state until a manual refresh.
var notifyChange func(walletID string)

func notifyRecordChanged(r *WalletRecord) {
	if notifyChange == nil || r == nil {
		return
	}
	id := r.Address
	if id == "" {
		id = r.TempID
	}
	if id != "" {
		notifyChange(id)
	}
}

// WalletName returns the owning dcrpulse wallet's name.
func (s *Store) WalletName() string { return s.walletName }

func cloneRecord(r *WalletRecord) *WalletRecord {
	if r == nil {
		return nil
	}
	c := *r
	c.Peers = make([]*Peer, len(r.Peers))
	for i, p := range r.Peers {
		pc := *p
		c.Peers[i] = &pc
	}
	if r.Own != nil {
		oc := *r.Own
		c.Own = &oc
	}
	c.RosterPubKeys = append([]string(nil), r.RosterPubKeys...)
	c.Xpubs = append([]string(nil), r.Xpubs...)
	if r.OwnHD != nil {
		hc := *r.OwnHD
		c.OwnHD = &hc
	}
	if r.Ext != nil {
		ec := *r.Ext
		c.Ext = &ec
	}
	if r.Int != nil {
		ic := *r.Int
		c.Int = &ic
	}
	if r.Proposals != nil {
		c.Proposals = make(map[string]*Proposal, len(r.Proposals))
		for k, p := range r.Proposals {
			c.Proposals[k] = cloneProposal(p)
		}
	}
	return &c
}

func cloneProposal(p *Proposal) *Proposal {
	if p == nil {
		return nil
	}
	c := *p
	c.Inputs = append([]ProposalInput(nil), p.Inputs...)
	c.Outputs = append([]ProposalOutput(nil), p.Outputs...)
	c.Queue = make([]*QueueHop, len(p.Queue))
	for i, h := range p.Queue {
		hc := *h
		c.Queue[i] = &hc
	}
	if p.SignedBy != nil {
		c.SignedBy = make(map[string]bool, len(p.SignedBy))
		for k, v := range p.SignedBy {
			c.SignedBy[k] = v
		}
	}
	return &c
}

// Proposal returns a clone of one proposal of a shared wallet.
func (s *Store) Proposal(walletID, txid string) (*WalletRecord, *Proposal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.lookupLocked(walletID)
	if r == nil {
		return nil, nil, false
	}
	p, ok := r.Proposals[txid]
	if !ok {
		return cloneRecord(r), nil, false
	}
	return cloneRecord(r), cloneProposal(p), true
}

// UpdateProposal mutates one proposal under the store lock, creating it
// when create is true.
func (s *Store) UpdateProposal(walletID, txid string, create bool, fn func(*WalletRecord, *Proposal) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.lookupLocked(walletID)
	if r == nil {
		return fmt.Errorf("unknown shared wallet %s", walletID)
	}
	if r.Proposals == nil {
		r.Proposals = make(map[string]*Proposal)
	}
	p, ok := r.Proposals[txid]
	if !ok {
		if !create {
			return fmt.Errorf("unknown proposal %s", txid)
		}
		now := time.Now().Unix()
		p = &Proposal{TxID: txid, CreatedAt: now}
		r.Proposals[txid] = p
	}
	if err := fn(r, p); err != nil {
		return err
	}
	now := time.Now().Unix()
	p.UpdatedAt = now
	r.UpdatedAt = now
	if err := s.saveLocked(); err != nil {
		return err
	}
	notifyRecordChanged(r)
	return nil
}

// Wallets returns clones of every record.
func (s *Store) Wallets() []*WalletRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*WalletRecord, 0, len(s.data.Wallets))
	for _, r := range s.data.Wallets {
		out = append(out, cloneRecord(r))
	}
	return out
}

// Wallet returns a clone of the record whose tempId or address is id.
func (s *Store) Wallet(id string) (*WalletRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.lookupLocked(id)
	if r == nil {
		return nil, false
	}
	return cloneRecord(r), true
}

func (s *Store) lookupLocked(id string) *WalletRecord {
	if r, ok := s.data.Wallets[id]; ok {
		return r
	}
	for _, r := range s.data.Wallets {
		if r.Address != "" && r.Address == id {
			return r
		}
	}
	return nil
}

// PutWallet inserts a new record keyed by its tempId.
func (s *Store) PutWallet(r *WalletRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.TempID == "" {
		return fmt.Errorf("record has no tempId")
	}
	if _, exists := s.data.Wallets[r.TempID]; exists {
		return fmt.Errorf("record %s already exists", r.TempID)
	}
	now := time.Now().Unix()
	r.CreatedAt = now
	r.UpdatedAt = now
	s.data.Wallets[r.TempID] = r
	if err := s.saveLocked(); err != nil {
		return err
	}
	notifyRecordChanged(r)
	return nil
}

// UpdateWallet mutates the record identified by tempId or address under
// the store lock. The callback sees the live record; returning an error
// abandons the mutation without saving.
func (s *Store) UpdateWallet(id string, fn func(*WalletRecord) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.lookupLocked(id)
	if r == nil {
		return fmt.Errorf("unknown shared wallet %s", id)
	}
	if err := fn(r); err != nil {
		return err
	}
	r.UpdatedAt = time.Now().Unix()
	if err := s.saveLocked(); err != nil {
		return err
	}
	notifyRecordChanged(r)
	return nil
}

// MarkProcessed journals a frame mid and reports whether it was new.
// Mids older than the replay horizon are pruned opportunistically.
func (s *Store) MarkProcessed(mid string, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.data.ProcessedMids[mid]; dup {
		return false, nil
	}
	if len(s.data.ProcessedMids) > 8192 {
		cutoff := now.Add(-seenTTL).Unix()
		for k, ts := range s.data.ProcessedMids {
			if ts < cutoff {
				delete(s.data.ProcessedMids, k)
			}
		}
	}
	s.data.ProcessedMids[mid] = now.Unix()
	return true, s.saveLocked()
}

// AppendOutbox persists a frame before its first send attempt.
func (s *Store) AppendOutbox(item *OutboxItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Outbox = append(s.data.Outbox, item)
	return s.saveLocked()
}

// MarkOutboxSent flips one outbox item to sent and prunes sent items
// older than the replay horizon. Items are keyed by mid AND recipient:
// an invite fans the same mid out to every peer.
func (s *Store) MarkOutboxSent(mid, toUID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-seenTTL).Unix()
	kept := s.data.Outbox[:0]
	for _, it := range s.data.Outbox {
		if it.MID == mid && it.ToUID == toUID {
			it.State = OutboxSent
		}
		if it.State == OutboxSent && it.Ts < cutoff {
			continue
		}
		kept = append(kept, it)
	}
	s.data.Outbox = kept
	return s.saveLocked()
}

// PendingOutbox returns clones of items still awaiting a successful send.
func (s *Store) PendingOutbox() []*OutboxItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*OutboxItem
	for _, it := range s.data.Outbox {
		if it.State != OutboxSent {
			c := *it
			out = append(out, &c)
		}
	}
	return out
}

// atomicWriteJSON writes doc to path via a temp file rename. Kept local so
// the package stays self-contained, mirroring the timestamp store.
func atomicWriteJSON(path string, doc any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(raw)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmpName, 0o600)
	}
	if werr == nil {
		werr = os.Rename(tmpName, path)
	}
	if werr != nil {
		os.Remove(tmpName)
		return werr
	}
	return nil
}

// Manager opens and caches the per-wallet stores of one network and
// routes inbound frames to the store holding the referenced record.
type Manager struct {
	mu      sync.Mutex
	network string
	stores  map[string]*Store
}

var (
	mgrMu sync.Mutex
	mgr   *Manager
)

// manager returns the process-wide store manager for the network,
// resetting it if the network ever changes.
func manager(network string) *Manager {
	mgrMu.Lock()
	defer mgrMu.Unlock()
	if mgr == nil || mgr.network != network {
		mgr = &Manager{network: network, stores: make(map[string]*Store)}
	}
	return mgr
}

func msigPath(network, walletName string) string {
	return filepath.Join(walletDirFn(network, walletName), "msig.json")
}

// StoreFor opens (or returns the cached) store of one wallet.
func (m *Manager) StoreFor(walletName string) (*Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.stores[walletName]; ok {
		return s, nil
	}
	s, err := openStore(msigPath(m.network, walletName), walletName)
	if err != nil {
		return nil, err
	}
	m.stores[walletName] = s
	return s, nil
}

// Route finds the store and record for a tempId or shared address across
// every wallet on this network, opening stores lazily from disk. Records
// are returned as clones. The match filter disambiguates rounds that span
// two local wallets (both sides of a handshake share the tempId when a
// user cosigns between their own wallets); the active wallet's store is
// preferred for the same reason.
func (m *Manager) Route(id string, match func(*WalletRecord) bool) (*Store, *WalletRecord) {
	if match == nil {
		match = func(*WalletRecord) bool { return true }
	}
	try := func(s *Store) *WalletRecord {
		if r, ok := s.Wallet(id); ok && match(r) {
			return r
		}
		return nil
	}
	if s, err := m.StoreFor(activeWalletSeam()); err == nil {
		if r := try(s); r != nil {
			return s, r
		}
	}
	scan := func() (*Store, *WalletRecord) {
		m.mu.Lock()
		stores := make([]*Store, 0, len(m.stores))
		for _, s := range m.stores {
			stores = append(stores, s)
		}
		m.mu.Unlock()
		for _, s := range stores {
			if r := try(s); r != nil {
				return s, r
			}
		}
		return nil, nil
	}
	if s, r := scan(); r != nil {
		return s, r
	}
	m.OpenExisting()
	return scan()
}

// OpenExisting opens every wallet store that already has an msig.json on
// disk, without creating any.
func (m *Manager) OpenExisting() {
	dirs, _ := filepath.Glob(filepath.Join(walletsDirFn(m.network), "*", "msig.json"))
	for _, p := range dirs {
		name := filepath.Base(filepath.Dir(p))
		if _, err := m.StoreFor(name); err != nil {
			continue
		}
	}
}

// Stores returns every currently opened store.
func (m *Manager) Stores() []*Store {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Store, 0, len(m.stores))
	for _, s := range m.stores {
		out = append(out, s)
	}
	return out
}
