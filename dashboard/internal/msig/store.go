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

const storeSchemaVersion = 1

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

// OwnKey records this wallet's contribution and its derivation
// coordinates for the backup card.
type OwnKey struct {
	PubKey  string `json:"pubKey"`
	Address string `json:"address"`
	Account uint32 `json:"account"`
	Branch  uint32 `json:"branch"`
	Index   uint32 `json:"index"`
}

// Peer is one cosigner as seen from this node.
type Peer struct {
	UID        string `json:"uid"`
	Nick       string `json:"nick"`
	PubKey     string `json:"pubkey,omitempty"`
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
	Role          string   `json:"role"`
	Status        string   `json:"status"`
	FailReason    string   `json:"failReason,omitempty"`
	CreatedHeight int64    `json:"createdHeight,omitempty"`
	Own           *OwnKey  `json:"own,omitempty"`
	InitiatorUID  string   `json:"initiatorUid,omitempty"`
	Peers         []*Peer  `json:"peers"`
	CreatedAt     int64    `json:"createdAt"`
	UpdatedAt     int64    `json:"updatedAt"`
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
	return &c
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
	return s.saveLocked()
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
	return s.saveLocked()
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
