package gamingfunds

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
	"sort"
)

// Participant is learned from an authenticated BR sender, not a game RPC.
type Participant struct {
	UID         string `json:"uid"`
	Key         string `json:"key"`
	Destination string `json:"destination"`
	TermsHash   string `json:"termsHash"`
}

func (t TableAuthorization) TermsHash() string {
	// Wallet and account are local; the network, game and economics are shared.
	shared := t
	shared.Scope.Wallet = ""
	shared.Scope.Account = 0
	shared.Closed = false
	raw, _ := json.Marshal(shared)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func (s *Store) RecordParticipant(scope Scope, table, group, authenticatedUID string, p Participant) error {
	uid, err := hex.DecodeString(authenticatedUID)
	if err != nil || len(uid) != 32 || hex.EncodeToString(uid) != authenticatedUID {
		return fmt.Errorf("invalid authenticated BR sender")
	}
	if p.UID != authenticatedUID || p.Destination == "" {
		return fmt.Errorf("financial sender mismatch")
	}
	key, err := finance.PublicKey(p.Key)
	if err != nil {
		return err
	}
	p.Key = hex.EncodeToString(key)
	k, err := scopeKey(scope, table)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	accepted, ok := d.Tables[k]
	if !ok || accepted.Closed || accepted.Group != group || accepted.TermsHash() != p.TermsHash {
		return fmt.Errorf("financial announcement differs from accepted table")
	}
	peers := d.Peers[k]
	if peers == nil {
		peers = map[string]Participant{}
	}
	if old, ok := peers[authenticatedUID]; ok {
		if old != p {
			return fmt.Errorf("participant financial authority changed")
		}
		return nil
	}
	if len(peers) >= int(accepted.Seats) {
		return fmt.Errorf("financial roster already full")
	}
	for _, old := range peers {
		if old.Key == p.Key || old.Destination == p.Destination {
			return fmt.Errorf("duplicate financial participant")
		}
	}
	peers[authenticatedUID] = p
	d.Peers[k] = peers
	return s.save(d)
}

func (s *Store) Participants(scope Scope, table string) ([]Participant, error) {
	k, err := scopeKey(scope, table)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	if _, ok := d.Tables[k]; !ok {
		return nil, fmt.Errorf("unknown table")
	}
	out := make([]Participant, 0, len(d.Peers[k]))
	for _, p := range d.Peers[k] {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func checkRoster(d diskState, scope Scope, terms Terms) error {
	if terms.Kind == "seatbond" {
		return nil
	}
	k, err := scopeKey(scope, terms.Table)
	if err != nil {
		return err
	}
	peers := d.Peers[k]
	if len(peers) != len(terms.Members) {
		return fmt.Errorf("waiting for authenticated financial roster")
	}
	expected := rosterHash(peers)
	for uid := range peers {
		if d.RosterCommits[k][uid] != expected {
			return fmt.Errorf("waiting for all financial roster commitments")
		}
	}
	known := map[string]bool{}
	for _, p := range peers {
		known[p.Key] = true
	}
	for _, key := range terms.Members {
		if !known[key] {
			return fmt.Errorf("game supplied an unauthenticated financial member")
		}
	}
	return nil
}

func rosterHash(peers map[string]Participant) string {
	rows := make([]Participant, 0, len(peers))
	for _, p := range peers {
		rows = append(rows, p)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].UID < rows[j].UID })
	raw, _ := json.Marshal(rows)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}
func (s *Store) RosterHash(scope Scope, table string) (string, error) {
	k, err := scopeKey(scope, table)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return "", err
	}
	t, ok := d.Tables[k]
	if !ok || t.Closed {
		return "", fmt.Errorf("table not active")
	}
	if len(d.Peers[k]) != int(t.Seats) {
		return "", nil
	}
	return rosterHash(d.Peers[k]), nil
}
func (s *Store) CommitRoster(scope Scope, table, authenticatedUID, hash string) (bool, error) {
	k, err := scopeKey(scope, table)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return false, err
	}
	t, ok := d.Tables[k]
	if !ok || t.Closed || len(d.Peers[k]) != int(t.Seats) || hash != rosterHash(d.Peers[k]) {
		return false, fmt.Errorf("financial roster commitment mismatch")
	}
	if _, ok := d.Peers[k][authenticatedUID]; !ok {
		return false, fmt.Errorf("unknown roster signer")
	}
	commits := d.RosterCommits[k]
	if commits == nil {
		commits = map[string]string{}
	}
	if old, ok := commits[authenticatedUID]; ok {
		if old != hash {
			return false, fmt.Errorf("conflicting roster commitment")
		}
		return false, nil
	}
	commits[authenticatedUID] = hash
	d.RosterCommits[k] = commits
	return true, s.save(d)
}
