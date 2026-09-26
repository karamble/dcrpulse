package gamingfunds

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
	"golang.org/x/sys/unix"
)

// Scope is established from authenticated bridge state, never supplied by a
// game. Wallet binds the records to a wallet fingerprint, not an account name.
type Scope struct {
	Game    string `json:"game"`
	Network string `json:"network"`
	Wallet  string `json:"wallet"`
	Account uint32 `json:"account"`
}

type Deposit struct {
	FundingHeight int64  `json:"fundingHeight"`
	FundingBlock  string `json:"fundingBlock"`
	Confirmations int64  `json:"confirmations"`
	SpendingTx    string `json:"spendingTx,omitempty"`
	ID            string `json:"id"`
	Scope         Scope  `json:"scope"`
	Terms         Terms  `json:"terms"`
	Script        string `json:"script"`
	Address       string `json:"address"`
	PkScript      string `json:"pkScript"`
	Outpoint      string `json:"outpoint,omitempty"`
	FundingTx     string `json:"fundingTx,omitempty"`
	State         string `json:"state"`
	Closed        bool   `json:"closed"`
	Error         string `json:"error,omitempty"`
}

type Operation struct {
	Confirmations int64    `json:"confirmations"`
	BlockHash     string   `json:"blockHash"`
	Height        int64    `json:"height"`
	ID            string   `json:"id"`
	Scope         Scope    `json:"scope"`
	Kind          string   `json:"kind"`
	DepositIDs    []string `json:"depositIDs"`
	Inputs        []string `json:"inputs"`
	Raw           string   `json:"raw"`
	Approved      bool     `json:"approved"`
	State         string   `json:"state"`
}

// WalletKey is a public locator. Private keys remain in dcrwallet.
type WalletKey struct {
	Scope   Scope  `json:"scope"`
	Table   string `json:"table"`
	Address string `json:"address"`
	Public  string `json:"public"`
}
type keyRecord = WalletKey

var ErrKeyNotRegistered = errors.New("wallet financial key not registered")

type diskState struct {
	RosterCommits map[string]map[string]string      `json:"rosterCommits"`
	Settlements   map[string]Settlement             `json:"settlements"`
	Peers         map[string]map[string]Participant `json:"peers"`
	Tables        map[string]TableAuthorization     `json:"tables"`
	Version       uint32                            `json:"version"`
	Keys          map[string]keyRecord              `json:"keys"`
	Deposits      map[string]Deposit                `json:"deposits"`
	Operations    map[string]Operation              `json:"operations"`
	Previews      map[string]PaymentPreview         `json:"previews"`
	Quotes        map[string]RecoveryQuote          `json:"quotes"`
	// Seated is each table's seated financial keys, as the game's signed
	// roster names them. Candidates are key announcements waiting for it,
	// in arrival order.
	Seated map[string][]string `json:"seated,omitempty"`
	// Proofs is this bridge's signed proof, per table, that the table's
	// financial key announces for its own Bison Relay identity.
	Proofs     map[string]string        `json:"proofs,omitempty"`
	Candidates map[string][]Participant `json:"candidates,omitempty"`
}

// Store serializes and durably records financial authority before any external
// side effect. The private store is never returned through game or UI APIs.
type Store struct {
	mu   sync.Mutex
	dir  string
	lock *os.File
}

func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("financial store directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, ".authority.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("financial store is already owned: %w", err)
	}
	s := &Store{dir: dir, lock: f}
	marker := filepath.Join(dir, "initialized")
	_, markerErr := os.Stat(marker)
	if _, ledgerErr := os.Stat(filepath.Join(dir, "authority.json")); os.IsNotExist(ledgerErr) {
		if markerErr == nil {
			f.Close()
			return nil, fmt.Errorf("financial ledger missing; restore its backup")
		}
		if !os.IsNotExist(markerErr) {
			f.Close()
			return nil, markerErr
		}
		if err = s.save(emptyState()); err != nil {
			f.Close()
			return nil, err
		}
	}
	if os.IsNotExist(markerErr) {
		mf, createErr := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if createErr != nil {
			f.Close()
			return nil, createErr
		}
		_, writeErr := mf.Write([]byte("financial-authority-v2\n"))
		if writeErr == nil {
			writeErr = mf.Sync()
		}
		closeErr := mf.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr == nil {
			directory, openErr := os.Open(dir)
			if openErr != nil {
				writeErr = openErr
			} else {
				writeErr = directory.Sync()
				directory.Close()
			}
		}
		if writeErr != nil {
			f.Close()
			return nil, writeErr
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.load()
	if err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}
func emptyState() diskState {
	return diskState{RosterCommits: map[string]map[string]string{}, Settlements: map[string]Settlement{}, Peers: map[string]map[string]Participant{}, Version: Version, Keys: map[string]keyRecord{}, Deposits: map[string]Deposit{}, Operations: map[string]Operation{}, Previews: map[string]PaymentPreview{}, Quotes: map[string]RecoveryQuote{}, Tables: map[string]TableAuthorization{}}
}
func (s *Store) load() (diskState, error) {
	if s.lock == nil {
		return diskState{}, fmt.Errorf("financial ledger is closed")
	}
	b, err := os.ReadFile(filepath.Join(s.dir, "authority.json"))
	if os.IsNotExist(err) {
		return diskState{}, fmt.Errorf("financial ledger missing; restore its backup")
	}
	if err != nil {
		return diskState{}, err
	}
	return decodeState(b)
}

func decodeState(b []byte) (diskState, error) {
	var d diskState
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, fmt.Errorf("financial store corrupt: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return d, fmt.Errorf("financial store corrupt: trailing data")
	}
	if d.RosterCommits == nil || d.Settlements == nil || d.Peers == nil || d.Version != Version || d.Keys == nil || d.Deposits == nil || d.Operations == nil || d.Tables == nil || d.Previews == nil || d.Quotes == nil {
		return d, fmt.Errorf("incomplete or unsupported financial store")
	}
	if d.Seated == nil {
		d.Seated = map[string][]string{}
	}
	if d.Candidates == nil {
		d.Candidates = map[string][]Participant{}
	}
	if d.Proofs == nil {
		d.Proofs = map[string]string{}
	}
	return d, nil
}

const backupFormat = 1
const maxBackupBytes = 64 << 20

type backupEnvelope struct {
	Format    uint32          `json:"format"`
	CreatedAt int64           `json:"createdAt"`
	Ledger    json.RawMessage `json:"ledger"`
	Checksum  string          `json:"checksum"`
}

func backupChecksum(ledger []byte) string {
	h := sha256.New()
	h.Write([]byte("dcrpulse/gaming-authority-backup/v1\x00"))
	h.Write(ledger)
	return hex.EncodeToString(h.Sum(nil))
}

// ExportBackup returns a self-checking copy of the complete public financial
// authority ledger. It contains wallet locators and transaction data, never
// wallet private keys. Callers must still protect it as financial metadata.
func (s *Store) ExportBackup() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	ledger, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	envelope := backupEnvelope{Format: backupFormat, CreatedAt: time.Now().Unix(), Ledger: ledger, Checksum: backupChecksum(ledger)}
	return json.Marshal(envelope)
}

func decodeBackup(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > maxBackupBytes {
		return nil, fmt.Errorf("financial backup is empty or too large")
	}
	var envelope backupEnvelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("financial backup corrupt: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("financial backup corrupt: trailing data")
	}
	if envelope.Format != backupFormat || envelope.CreatedAt <= 0 || len(envelope.Ledger) == 0 || envelope.Checksum != backupChecksum(envelope.Ledger) {
		return nil, fmt.Errorf("financial backup checksum or format mismatch")
	}
	if _, err := decodeState(envelope.Ledger); err != nil {
		return nil, err
	}
	return append([]byte(nil), envelope.Ledger...), nil
}

// RestoreBackup restores a missing authority ledger while holding the same
// exclusive directory lock as Open. It never overwrites an existing ledger;
// an operator must preserve and inspect a damaged file before replacing it.
func RestoreBackup(dir string, raw []byte) error {
	ledger, err := decodeBackup(raw)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, ".authority.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("financial store is already owned: %w", err)
	}
	if _, err = os.Stat(filepath.Join(dir, "authority.json")); err == nil {
		return fmt.Errorf("financial ledger already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(dir, ".authority-restore-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(ledger)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(dir, "authority.json"))
	}
	if err != nil {
		return err
	}
	marker := filepath.Join(dir, "initialized")
	if _, statErr := os.Stat(marker); os.IsNotExist(statErr) {
		if err = os.WriteFile(marker, []byte("financial-authority-v2\n"), 0600); err != nil {
			return err
		}
	} else if statErr != nil {
		return statErr
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func (s *Store) save(d diskState) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".authority-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(s.dir, "authority.json")); err != nil {
		return err
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func scopeKey(scope Scope, table string) (string, error) {
	if scope.Game == "" || scope.Network == "" || scope.Wallet == "" {
		return "", fmt.Errorf("financial scope is incomplete")
	}
	b, err := json.Marshal(struct {
		Scope Scope
		Table string
	}{scope, table})
	return string(b), err
}

// RegisterKey is called by the wallet adapter, never by a game RPC.
func (s *Store) RegisterKey(key WalletKey) error {
	k, err := scopeKey(key.Scope, key.Table)
	if err != nil {
		return err
	}
	pub, err := finance.PublicKey(key.Public)
	if err != nil || key.Address == "" || key.Table == "" {
		return fmt.Errorf("invalid wallet key locator")
	}
	key.Public = hex.EncodeToString(pub)
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	if old, ok := d.Keys[k]; ok {
		if old != key {
			return fmt.Errorf("wallet key is immutable")
		}
		return nil
	}
	table, ok := d.Tables[k]
	if !ok || table.Closed {
		return fmt.Errorf("wallet key requires an accepted active table")
	}
	d.Keys[k] = key
	return s.save(d)
}
func (s *Store) WalletKey(scope Scope, table string) (WalletKey, error) {
	k, err := scopeKey(scope, table)
	if err != nil {
		return WalletKey{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return WalletKey{}, err
	}
	key, ok := d.Keys[k]
	if !ok {
		return WalletKey{}, ErrKeyNotRegistered
	}
	if _, err = finance.PublicKey(key.Public); err != nil || key.Scope != scope || key.Table != table || key.Address == "" {
		return WalletKey{}, fmt.Errorf("invalid persisted wallet key")
	}
	return key, nil
}
func (s *Store) PublicKey(scope Scope, table string) (string, error) {
	key, err := s.WalletKey(scope, table)
	return key.Public, err
}
func randomID() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}

// Register accepts only a descriptor derived by the caller's verifier and
// checks that its recovery authority actually belongs to this bridge scope.
func (s *Store) Register(scope Scope, terms Terms, params stdaddr.AddressParams) (Deposit, error) {
	var empty Deposit
	k, err := scopeKey(scope, terms.Table)
	if err != nil {
		return empty, err
	}
	if terms.Game != scope.Game || terms.Network != scope.Network || terms.Account != scope.Account {
		return empty, fmt.Errorf("deposit scope mismatch")
	}
	terms, err = terms.Canonical()
	if err != nil {
		return empty, err
	}
	script, err := terms.Script()
	if err != nil {
		return empty, err
	}
	address, pk, err := terms.Output(params)
	if err != nil {
		return empty, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return empty, err
	}
	rec, ok := d.Keys[k]
	if !ok {
		return empty, fmt.Errorf("no bridge financial authority for this table")
	}
	if rec.Public != terms.Recovery || rec.Scope != scope || rec.Table != terms.Table {
		return empty, fmt.Errorf("recovery authority is not the registered wallet key")
	}
	table, ok := d.Tables[k]
	if !ok || table.Closed {
		return empty, fmt.Errorf("table is not active")
	}
	if err = checkRoster(d, scope, terms); err != nil {
		return empty, err
	}
	if err = table.CheckDeposit(terms); err != nil {
		return empty, err
	}
	// A table/kind is an obligation, not an invitation to repeatedly pay it.
	for _, old := range d.Deposits {
		if old.Scope == scope && old.Terms.Table == terms.Table && old.Terms.Kind == terms.Kind {
			a, _ := json.Marshal(old.Terms)
			b, _ := json.Marshal(terms)
			if string(a) != string(b) || old.Script != hex.EncodeToString(script) || old.Address != address || old.PkScript != hex.EncodeToString(pk) {
				return empty, fmt.Errorf("deposit terms are immutable")
			}
			return old, nil
		}
	}
	id, err := randomID()
	if err != nil {
		return empty, err
	}
	out := Deposit{ID: id, Scope: scope, Terms: terms, Script: hex.EncodeToString(script), Address: address, PkScript: hex.EncodeToString(pk), State: "prepared"}
	d.Deposits[id] = out
	if err = s.save(d); err != nil {
		return empty, err
	}
	return out, nil
}
func (s *Store) Deposits(scope Scope) ([]Deposit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	out := []Deposit{}
	for _, v := range d.Deposits {
		if v.Scope == scope {
			out = append(out, v)
		}
	}
	return out, nil
}

// Reserve is called before signing or publishing. Only a dashboard approval
// can set Approved. Exact retries retain the same bytes and input reservation.
func (s *Store) Reserve(op Operation) error {
	if !op.Approved || op.ID == "" || op.Raw == "" || len(op.Inputs) == 0 {
		return fmt.Errorf("operation lacks explicit approval or transaction")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	for _, id := range op.DepositIDs {
		v, ok := d.Deposits[id]
		if !ok || v.Scope != op.Scope {
			return fmt.Errorf("operation references an unknown or foreign deposit")
		}
	}
	if old, ok := d.Operations[op.ID]; ok {
		a, _ := json.Marshal(old)
		b, _ := json.Marshal(op)
		if string(a) != string(b) {
			return fmt.Errorf("operation already reserved with different terms")
		}
		return nil
	}
	for _, old := range d.Operations {
		for _, a := range old.Inputs {
			for _, b := range op.Inputs {
				if a == b {
					return fmt.Errorf("input reserved by operation %s", old.ID)
				}
			}
		}
	}
	d.Operations[op.ID] = op
	return s.save(d)
}

// PaymentPreview pins the exact transaction shown to the operator before any
// signature is requested. It is private state, not part of the game report.
type PaymentPreview struct {
	RequestedAt int64  `json:"requestedAt"`
	ExpiresAt   int64  `json:"expiresAt"`
	DepositID   string `json:"depositID"`
	Unsigned    string `json:"unsigned"`
	Reason      string `json:"reason,omitempty"`
	FeeAtoms    int64  `json:"feeAtoms"`
}

// FundingApproval is the exact dashboard request durably reserved by the
// financial authority. It contains no private key material.
type FundingApproval struct {
	ID      string
	Scope   Scope
	Deposit Deposit
	Preview PaymentPreview
}

func validatePreview(id string, dep Deposit, scope Scope, preview PaymentPreview) error {
	if id == "" || preview.DepositID == "" || preview.RequestedAt <= 0 || preview.ExpiresAt <= preview.RequestedAt || preview.FeeAtoms < 0 {
		return fmt.Errorf("incomplete funding approval")
	}
	if dep.ID == "" || dep.ID != preview.DepositID || dep.Scope != scope {
		return fmt.Errorf("unknown preview deposit")
	}
	raw, err := hex.DecodeString(preview.Unsigned)
	if err != nil {
		return err
	}
	if _, err = finance.DecodeTransaction(raw); err != nil {
		return err
	}
	return nil
}

func fundingApproval(id string, dep Deposit, preview PaymentPreview) FundingApproval {
	return FundingApproval{ID: id, Scope: dep.Scope, Deposit: dep, Preview: preview}
}

// reservePreview records a new approval. When recoverExisting is true, an
// already-reserved approval for this deposit is returned verbatim. That is the
// response-loss and crash-recovery path: a retry never creates another wallet
// transaction or another approval id.
func (s *Store) reservePreview(id string, scope Scope, preview PaymentPreview, recoverExisting bool) (FundingApproval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return FundingApproval{}, err
	}
	dep, ok := d.Deposits[preview.DepositID]
	if !ok || dep.Scope != scope {
		return FundingApproval{}, fmt.Errorf("unknown preview deposit")
	}
	if dep.Closed || dep.FundingTx != "" {
		return FundingApproval{}, fmt.Errorf("deposit cannot be funded")
	}
	if old, ok := d.Previews[id]; ok && old != preview {
		return FundingApproval{}, fmt.Errorf("approval preview is immutable")
	}
	if old, ok := d.Previews[id]; ok {
		if err := validatePreview(id, dep, scope, old); err != nil {
			return FundingApproval{}, err
		}
		return fundingApproval(id, dep, old), nil
	}
	if err := validatePreview(id, dep, scope, preview); err != nil {
		return FundingApproval{}, err
	}
	if preview.ExpiresAt <= time.Now().Unix() {
		return FundingApproval{}, fmt.Errorf("funding approval expired")
	}
	raw, err := hex.DecodeString(preview.Unsigned)
	if err != nil {
		return FundingApproval{}, err
	}
	tx, err := finance.DecodeTransaction(raw)
	if err != nil {
		return FundingApproval{}, err
	}
	inputs := map[wire.OutPoint]bool{}
	for _, in := range tx.TxIn {
		if inputs[in.PreviousOutPoint] {
			return FundingApproval{}, fmt.Errorf("duplicate funding input")
		}
		inputs[in.PreviousOutPoint] = true
	}
	if len(inputs) == 0 {
		return FundingApproval{}, fmt.Errorf("empty funding transaction")
	}
	for priorID, prior := range d.Previews {
		if priorID == id || prior.ExpiresAt <= time.Now().Unix() {
			continue
		}
		if prior.DepositID == preview.DepositID {
			if recoverExisting {
				if err := validatePreview(priorID, dep, scope, prior); err != nil {
					return FundingApproval{}, err
				}
				return fundingApproval(priorID, dep, prior), nil
			}
			return FundingApproval{}, fmt.Errorf("deposit already has a pending funding approval")
		}
		raw, err := hex.DecodeString(prior.Unsigned)
		if err != nil {
			return FundingApproval{}, err
		}
		held, err := finance.DecodeTransaction(raw)
		if err != nil {
			return FundingApproval{}, err
		}
		for _, in := range held.TxIn {
			if inputs[in.PreviousOutPoint] {
				return FundingApproval{}, fmt.Errorf("wallet input reserved by another approval")
			}
		}
	}
	for _, op := range d.Operations {
		for _, in := range tx.TxIn {
			for _, held := range op.Inputs {
				if in.PreviousOutPoint.String() == held {
					return FundingApproval{}, fmt.Errorf("wallet input already committed")
				}
			}
		}
	}
	d.Previews[id] = preview
	if err := s.save(d); err != nil {
		return FundingApproval{}, err
	}
	return fundingApproval(id, dep, preview), nil
}

// SavePreview is the strict insert used by low-level callers and tests.
func (s *Store) SavePreview(id string, scope Scope, preview PaymentPreview) error {
	_, err := s.reservePreview(id, scope, preview, false)
	return err
}

// EnsurePreview returns the original immutable approval after response loss or
// a crash between the authority-ledger write and the dashboard audit write.
func (s *Store) EnsurePreview(id string, scope Scope, preview PaymentPreview) (FundingApproval, error) {
	return s.reservePreview(id, scope, preview, true)
}

// FundingApprovals returns every authority-owned preview so the presentation
// audit can recover missing requests after restart. Corrupt references fail the
// whole read closed.
func (s *Store) FundingApprovals() ([]FundingApproval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]FundingApproval, 0, len(d.Previews))
	for id, preview := range d.Previews {
		dep, ok := d.Deposits[preview.DepositID]
		if !ok {
			return nil, fmt.Errorf("funding approval %s references an unknown deposit", id)
		}
		if err := validatePreview(id, dep, dep.Scope, preview); err != nil {
			return nil, fmt.Errorf("funding approval %s: %w", id, err)
		}
		out = append(out, fundingApproval(id, dep, preview))
	}
	return out, nil
}
func (s *Store) Preview(id, deposit string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	p, ok := d.Previews[id]
	if !ok || p.DepositID != deposit {
		return nil, fmt.Errorf("verified transaction preview missing")
	}
	return hex.DecodeString(p.Unsigned)
}

// CommitFunding persists both signed bytes and the expected funded output before
// publication. An ambiguous network result cannot expose another approval.
func (s *Store) CommitFunding(requestID string, scope Scope, depositID string, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	dep, ok := d.Deposits[depositID]
	if !ok || dep.Scope != scope || dep.Closed {
		return fmt.Errorf("unknown or closed funding obligation")
	}
	preview, ok := d.Previews[requestID]
	if !ok || preview.DepositID != depositID || preview.ExpiresAt <= time.Now().Unix() {
		return fmt.Errorf("funding has no approved preview")
	}
	before, err := hex.DecodeString(preview.Unsigned)
	if err != nil {
		return err
	}
	expected, err := finance.DecodeTransaction(before)
	if err != nil {
		return fmt.Errorf("invalid approved funding bytes: %w", err)
	}
	tx, err := finance.DecodeTransaction(raw)
	if err != nil || !finance.SameIntent(expected, tx) {
		return fmt.Errorf("funding differs from approved bytes")
	}
	inputs := []string{}
	seen := map[wire.OutPoint]bool{}
	for i, in := range tx.TxIn {
		if in.ValueIn != expected.TxIn[i].ValueIn || len(in.SignatureScript) == 0 || seen[in.PreviousOutPoint] {
			return fmt.Errorf("invalid funding witness or duplicated input")
		}
		seen[in.PreviousOutPoint] = true
		inputs = append(inputs, in.PreviousOutPoint.String())
	}
	id := tx.TxHash().String()
	hexed := hex.EncodeToString(raw)
	if old, ok := d.Operations[id]; ok {
		if old.Raw != hexed || old.Scope != scope {
			return fmt.Errorf("conflicting funding retry")
		}
		return nil
	}
	if dep.FundingTx != "" {
		return fmt.Errorf("funding already reserved")
	}
	for _, old := range d.Operations {
		for _, a := range old.Inputs {
			for _, b := range inputs {
				if a == b {
					return fmt.Errorf("funding input already reserved")
				}
			}
		}
	}
	count := 0
	for i, out := range tx.TxOut {
		if hex.EncodeToString(out.PkScript) == dep.PkScript && out.Value == dep.Terms.Atoms && out.Version == 0 {
			dep.Outpoint = fmt.Sprintf("%s:%d", id, i)
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("funding output differs from registered deposit")
	}
	d.Operations[id] = Operation{ID: id, Scope: scope, Kind: "funding", DepositIDs: []string{depositID}, Inputs: inputs, Raw: hexed, Approved: true, State: "publishing"}
	dep.FundingTx = id
	dep.State = "publishing"
	d.Deposits[depositID] = dep
	return s.save(d)
}

// ApprovedBroadcast only recognizes bytes previously approved and journaled by
// the bridge. A game's claim that a transaction is cooperative grants nothing.
func (s *Store) ApprovedBroadcast(scope Scope, raw []byte) error {
	var tx wire.MsgTx
	if tx.FromBytes(raw) != nil {
		return fmt.Errorf("invalid financial transaction")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	op, ok := d.Operations[tx.TxHash().String()]
	if !ok || op.Scope != scope || !op.Approved || op.Raw != hex.EncodeToString(raw) || op.Kind == "funding" {
		return fmt.Errorf("transaction has no matching dashboard approval")
	}
	return nil
}

func (s *Store) AllDeposits() ([]Deposit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]Deposit, 0, len(d.Deposits))
	for _, dep := range d.Deposits {
		out = append(out, dep)
	}
	return out, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	err := s.lock.Close()
	s.lock = nil
	return err
}

// TableAuthorization records the invitation the dashboard operator accepted.
// It is not written by a game RPC.
type TableAuthorization struct {
	Scope           Scope  `json:"scope"`
	Table           string `json:"table"`
	StakeAtoms      int64  `json:"stakeAtoms"`
	CSVBlocks       uint32 `json:"csvBlocks"`
	AdmissionAtoms  int64  `json:"admissionAtoms"`
	AdmissionBlocks uint32 `json:"admissionBlocks"`
	TableBondAtoms  int64  `json:"tableBondAtoms"`
	TableBondBlocks uint32 `json:"tableBondBlocks"`
	Group           string `json:"group"`
	Seats           uint32 `json:"seats"`
	Until           uint32 `json:"until"`
	Closed          bool   `json:"closed"`
}

func (s *Store) AuthorizeTable(t TableAuthorization) error {
	key, err := scopeKey(t.Scope, t.Table)
	if err != nil {
		return err
	}
	if t.AdmissionAtoms <= 0 || t.AdmissionAtoms > finance.MaxAtoms || t.AdmissionBlocks == 0 || t.AdmissionBlocks > MaxLockBlocks || t.TableBondAtoms < 0 || t.TableBondAtoms > finance.MaxAtoms || (t.TableBondAtoms > 0 && (t.TableBondBlocks == 0 || t.TableBondBlocks > MaxLockBlocks)) || t.Table == "" || t.StakeAtoms <= 0 || t.StakeAtoms > finance.MaxAtoms || t.CSVBlocks == 0 || t.CSVBlocks > MaxLockBlocks || t.Seats < 2 || t.Seats > finance.MaxMembers || t.Until == 0 {
		return fmt.Errorf("invalid table financial terms")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	if old, ok := d.Tables[key]; ok {
		if old != t {
			return fmt.Errorf("accepted table terms cannot change")
		}
		return nil
	}
	d.Tables[key] = t
	return s.save(d)
}
func (s *Store) AuthorizedTable(scope Scope, table string) (TableAuthorization, error) {
	key, err := scopeKey(scope, table)
	if err != nil {
		return TableAuthorization{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return TableAuthorization{}, err
	}
	t, ok := d.Tables[key]
	if !ok || t.Closed {
		return t, fmt.Errorf("table was not accepted by the operator or has been closed")
	}
	return t, nil
}

func (t TableAuthorization) CheckDeposit(d Terms) error {
	var atoms int64
	var blocks uint32
	switch d.Kind {
	case "seatbond":
		atoms, blocks = t.AdmissionAtoms, t.AdmissionBlocks
	case "stake":
		atoms, blocks = t.StakeAtoms, t.CSVBlocks
	case "tablebond":
		atoms, blocks = t.TableBondAtoms, t.TableBondBlocks
	default:
		return fmt.Errorf("unsupported deposit template")
	}
	if atoms <= 0 || d.Atoms != atoms || d.LockBlocks != blocks {
		return fmt.Errorf("deposit differs from accepted table terms")
	}
	if d.Kind != "seatbond" && len(d.Members) != int(t.Seats) {
		return fmt.Errorf("deposit roster differs from table size")
	}
	return nil
}
