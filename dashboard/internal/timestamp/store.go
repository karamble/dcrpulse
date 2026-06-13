// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package timestamp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status is the lifecycle stage of an archived timestamp record. The three
// pre-anchor states mirror the dcrtime anchoring stages; "failed" marks a
// submission that errored and can be retried (the digest is safe locally).
type Status string

const (
	StatusSubmitted Status = "submitted" // accepted; anchor stage not yet known
	StatusAwaiting  Status = "awaiting"  // queued for the next hourly anchor
	StatusPending   Status = "pending"   // in an anchor tx, not yet confirmed
	StatusAnchored  Status = "anchored"  // committed to the chain (terminal)
	StatusFailed    Status = "failed"    // submission failed; retryable
)

var (
	// ErrDuplicate is returned by Create when the digest is already archived.
	ErrDuplicate = errors.New("digest already in archive")
	// ErrNotFound is returned when a digest is not present in the archive.
	ErrNotFound = errors.New("record not found")
)

// Record is one archived timestamp: file metadata plus, once anchored, the
// cryptographic proof. The original file is never stored.
type Record struct {
	Digest      string   `json:"digest"`
	Filename    string   `json:"filename"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	FileSize    int64    `json:"fileSize"`
	MimeType    string   `json:"mimeType,omitempty"`
	FileMtime   string   `json:"fileMtime,omitempty"`
	Tags        []string `json:"tags,omitempty"`

	Status      Status `json:"status"`
	SubmittedAt string `json:"submittedAt"`
	FailReason  string `json:"failReason,omitempty"`

	// Anchor proof - zero until the digest is anchored.
	AnchorTime       int64           `json:"anchorTime,omitempty"` // chaintimestamp, unix seconds
	MerkleRoot       string          `json:"merkleRoot,omitempty"`
	MerklePath       json.RawMessage `json:"merklePath,omitempty"` // verbatim dcrtime proof
	TxID             string          `json:"txId,omitempty"`
	Confirmations    int32           `json:"confirmations,omitempty"`
	MinConfirmations int32           `json:"minConfirmations,omitempty"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Query filters and orders a List call.
type Query struct {
	Text   string // case-insensitive substring of filename | title | description
	Status Status // "" matches any
	Tag    string // "" matches any
	Sort   string // "newest" (default) | "oldest" | "title"
}

// Store is the on-disk timestamp archive. It holds a single mutex (no bbolt or
// nested locking) and rewrites the whole JSON document on each mutation, which
// is ample for a personal archive of hundreds to low-thousands of records.
type Store struct {
	mu       sync.Mutex
	path     string
	clientID string
	records  map[string]*Record
}

// archiveFile is the on-disk JSON shape.
type archiveFile struct {
	SchemaVersion int                `json:"schemaVersion"`
	ClientID      string             `json:"clientId"`
	Records       map[string]*Record `json:"records"`
}

// OpenStore loads (or initializes) the archive at path. A stable per-install
// client id is generated and persisted on first open.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, records: map[string]*Record{}}

	data, err := os.ReadFile(path)
	switch {
	case err == nil && len(data) > 0:
		var af archiveFile
		if err := json.Unmarshal(data, &af); err != nil {
			return nil, fmt.Errorf("parse timestamp archive %s: %w", path, err)
		}
		s.clientID = af.ClientID
		if af.Records != nil {
			s.records = af.Records
		}
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("read timestamp archive %s: %w", path, err)
	}

	if s.clientID == "" {
		s.clientID = newClientID()
		s.mu.Lock()
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// ClientID returns the persisted dcrtime client id for this install.
func (s *Store) ClientID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clientID
}

// Create inserts r, returning ErrDuplicate if the digest already exists. The
// store keeps its own copy; the caller's pointer is not retained.
func (s *Store) Create(r *Record) error {
	now := nowRFC3339()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[r.Digest]; ok {
		return ErrDuplicate
	}
	cp := cloneRecord(r)
	cp.CreatedAt, cp.UpdatedAt = now, now
	if cp.SubmittedAt == "" {
		cp.SubmittedAt = now
	}
	s.records[cp.Digest] = cp
	return s.saveLocked()
}

// Update applies fn to the stored record under one transaction-equivalent lock,
// so the worker and API edits never lose each other's writes.
func (s *Store) Update(digest string, fn func(*Record) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[digest]
	if !ok {
		return ErrNotFound
	}
	if err := fn(r); err != nil {
		return err
	}
	r.UpdatedAt = nowRFC3339()
	return s.saveLocked()
}

// Delete removes a record, returning ErrNotFound if absent.
func (s *Store) Delete(digest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[digest]; !ok {
		return ErrNotFound
	}
	delete(s.records, digest)
	return s.saveLocked()
}

// Get returns a copy of one record.
func (s *Store) Get(digest string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[digest]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneRecord(r), nil
}

// List returns copies of the records matching q, ordered per q.Sort.
func (s *Store) List(q Query) []*Record {
	s.mu.Lock()
	out := make([]*Record, 0, len(s.records))
	for _, r := range s.records {
		if matches(r, q) {
			out = append(out, cloneRecord(r))
		}
	}
	s.mu.Unlock()
	sortRecords(out, q.Sort)
	return out
}

// PendingDigests returns the digests of records awaiting anchoring, for the
// background verify worker.
func (s *Store) PendingDigests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for d, r := range s.records {
		switch r.Status {
		case StatusSubmitted, StatusAwaiting, StatusPending:
			out = append(out, d)
		}
	}
	return out
}

// All returns copies of every record.
func (s *Store) All() []*Record {
	return s.List(Query{})
}

// saveLocked rewrites the whole archive atomically. Caller holds s.mu.
func (s *Store) saveLocked() error {
	af := archiveFile{SchemaVersion: 1, ClientID: s.clientID, Records: s.records}
	data, err := json.MarshalIndent(af, "", "  ")
	if err != nil {
		return fmt.Errorf("encode timestamp archive: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}
	return atomicWriteJSON(s.path, data)
}

func matches(r *Record, q Query) bool {
	if q.Status != "" && r.Status != q.Status {
		return false
	}
	if q.Tag != "" && !containsTag(r.Tags, q.Tag) {
		return false
	}
	if q.Text != "" {
		needle := strings.ToLower(q.Text)
		hay := strings.ToLower(r.Filename + " " + r.Title + " " + r.Description)
		if !strings.Contains(hay, needle) {
			return false
		}
	}
	return true
}

func sortRecords(rs []*Record, mode string) {
	switch mode {
	case "oldest":
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].SubmittedAt < rs[j].SubmittedAt })
	case "title":
		sort.SliceStable(rs, func(i, j int) bool {
			return strings.ToLower(titleOf(rs[i])) < strings.ToLower(titleOf(rs[j]))
		})
	default: // newest
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].SubmittedAt > rs[j].SubmittedAt })
	}
}

func titleOf(r *Record) string {
	if r.Title != "" {
		return r.Title
	}
	return r.Filename
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func cloneRecord(r *Record) *Record {
	cp := *r
	if r.Tags != nil {
		cp.Tags = append([]string(nil), r.Tags...)
	}
	if r.MerklePath != nil {
		cp.MerklePath = append(json.RawMessage(nil), r.MerklePath...)
	}
	return &cp
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

func newClientID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "dcrpulse"
	}
	return hex.EncodeToString(b)
}

// atomicWriteJSON writes data to path via temp file + rename, mode 0600. Mirrors
// config.atomicWriteJSON (kept local so this package stays self-contained).
func atomicWriteJSON(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
