// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package archive

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"dcrpulse/internal/fsutil"
)

// tokenRe matches a censorship token, which also names its file.
var tokenRe = regexp.MustCompile(`^[0-9a-f]{16,64}$`)

// Entry is an archived proposal and what its verification established.
type Entry struct {
	Proposal *Proposal
	Checked  Checked
}

// Store is the archive on disk, one file per proposal. It reads the
// directory once, on first use.
type Store struct {
	dir       string
	serverKey string

	mu      sync.Mutex
	loaded  bool
	entries map[string]Entry
}

// NewStore returns the archive kept in dir, verified with serverKey.
func NewStore(dir, serverKey string) *Store {
	return &Store{dir: dir, serverKey: serverKey, entries: map[string]Entry{}}
}

// ServerKey is the Politeia server key the archive is verified with.
func (s *Store) ServerKey() string { return s.serverKey }

// loadLocked reads every archived file once. A file that does not parse or
// verify is left out and logged; the next verified fetch replaces it.
func (s *Store) loadLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	names, err := os.ReadDir(s.dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			govnLog.Warnf("Politeia archive %s unreadable: %v", s.dir, err)
		}
		return
	}
	for _, n := range names {
		token, ok := strings.CutSuffix(n.Name(), ".json")
		if !ok || !tokenRe.MatchString(token) {
			continue
		}
		e, err := s.read(token)
		if err != nil {
			govnLog.Warnf("Ignoring archived proposal %s: %v", token, err)
			continue
		}
		s.entries[token] = e
	}
}

func (s *Store) read(token string) (Entry, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, token+".json"))
	if err != nil {
		return Entry{}, err
	}
	var p Proposal
	if err := json.Unmarshal(raw, &p); err != nil {
		return Entry{}, err
	}
	if p.Token() != token {
		return Entry{}, fmt.Errorf("file holds proposal %s", p.Token())
	}
	ck, err := Verify(&p, s.serverKey)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Proposal: &p, Checked: ck}, nil
}

// Get returns the archived proposal with the token.
func (s *Store) Get(token string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadLocked()
	e, ok := s.entries[token]
	return e, ok
}

// All returns every archived proposal.
func (s *Store) All() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadLocked()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	return out
}

// Put verifies a proposal and writes it, replacing any earlier copy.
func (s *Store) Put(p *Proposal) (Entry, error) {
	e, _, err := s.put(p, false)
	return e, err
}

// SetVoteCheck records a vote check on the archived copy it was made against
// and keeps the ballots it read, as politeiaverify's votes bundle. A copy
// replaced during the check gets no result.
func (s *Store) SetVoteCheck(token string, checked *Proposal, vc VoteCheck, bundle []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadLocked()
	e, ok := s.entries[token]
	if !ok {
		return fmt.Errorf("proposal %s is not archived", token)
	}
	if e.Proposal != checked {
		return errors.New("the archived proposal changed during the check")
	}
	p := *e.Proposal
	p.VoteCheck = &vc
	raw, err := json.Marshal(&p)
	if err != nil {
		return err
	}
	votes := filepath.Join(s.dir, "votes")
	if err := os.MkdirAll(votes, 0o700); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(votes, token+".json"), bundle, 0o600); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(s.dir, token+".json"), raw, 0o600); err != nil {
		return err
	}
	s.entries[token] = Entry{Proposal: &p, Checked: e.Checked}
	return nil
}

// VotesFile is where the ballots of a checked proposal are kept.
func (s *Store) VotesFile(token string) (string, error) {
	if !tokenRe.MatchString(token) {
		return "", fmt.Errorf("invalid token %q", token)
	}
	return filepath.Join(s.dir, "votes", token+".json"), nil
}

// put verifies and writes p. With onlyNewer, as for an import, it keeps a
// held copy this dashboard did not import, or an import as new or newer, and
// reports false.
func (s *Store) put(p *Proposal, onlyNewer bool) (Entry, bool, error) {
	token := p.Token()
	if !tokenRe.MatchString(token) {
		return Entry{}, false, fmt.Errorf("invalid token %q", token)
	}
	ck, err := Verify(p, s.serverKey)
	if err != nil {
		return Entry{}, false, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return Entry{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadLocked()
	if old, ok := s.entries[token]; ok && onlyNewer &&
		(old.Proposal.Source != SourceImport || old.Proposal.ArchivedAt >= p.ArchivedAt) {
		return old, false, nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return Entry{}, false, err
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(s.dir, token+".json"), raw, 0o600); err != nil {
		return Entry{}, false, err
	}
	e := Entry{Proposal: p, Checked: ck}
	s.entries[token] = e
	return e, true, nil
}
