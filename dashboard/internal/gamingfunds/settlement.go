package gamingfunds

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
)

type Settlement struct {
	ID           string              `json:"id"`
	Scope        Scope               `json:"scope"`
	Table        string              `json:"table"`
	Raw          string              `json:"raw"`
	Inputs       []finance.Input     `json:"inputs"`
	Payments     []finance.Payment   `json:"payments"`
	Destinations map[string]string   `json:"destinations"`
	FeeAtoms     int64               `json:"feeAtoms"`
	ExpiresAt    int64               `json:"expiresAt"`
	State        string              `json:"state"`
	Signatures   map[string][][]byte `json:"signatures"`
}

func expireSettlements(d *diskState, now int64) bool {
	changed := false
	for id, p := range d.Settlements {
		if p.State == "awaiting_approval" && p.ExpiresAt <= now {
			p.State = "expired"
			d.Settlements[id] = p
			changed = true
		}
	}
	return changed
}

// ProposeSettlement is called after fresh chain validation. It persists exact
// public intent before the dashboard offers approval, without signing anything.
func (s *Store) ProposeSettlement(scope Scope, p finance.Payout, params stdaddr.AddressParams) (Settlement, error) {
	var empty Settlement
	k, err := scopeKey(scope, p.Table)
	if err != nil {
		return empty, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return empty, err
	}
	now := time.Now()
	if expireSettlements(&d, now.Unix()) {
		if err = s.save(d); err != nil {
			return empty, err
		}
	}
	table, ok := d.Tables[k]
	if !ok || table.Closed {
		return empty, fmt.Errorf("table not active")
	}
	destinations := map[string]string{}
	for _, peer := range d.Peers[k] {
		destinations[peer.Key] = peer.Destination
	}
	built, err := finance.BuildPayout(p, destinations, params, feeRules())
	if err != nil {
		return empty, err
	}
	tx, inputs := built.Transaction, built.Inputs
	ours, ok := d.Keys[k]
	if !ok {
		return empty, fmt.Errorf("local financial key missing")
	}
	ownDeposit := ""
	for _, input := range inputs {
		if err = table.CheckDeposit(input.Terms); err != nil {
			return empty, err
		}
		if input.Terms.Game != scope.Game || input.Terms.Network != scope.Network {
			return empty, fmt.Errorf("foreign payout input")
		}
		if err = checkRoster(d, scope, input.Terms); err != nil {
			return empty, err
		}
		if input.Terms.Recovery == ours.Public {
			for _, dep := range d.Deposits {
				if dep.Scope == scope && dep.Terms.Table == p.Table && dep.Terms.Kind == "stake" && dep.Outpoint == formatOutput(input.Outpoint.Hash.String(), input.Outpoint.Index) && dep.Terms.Atoms == input.Terms.Atoms {
					ownDeposit = dep.ID
				}
			}
			if ownDeposit == "" {
				return empty, fmt.Errorf("payout does not spend the bridge's recorded stake")
			}
		}
	}
	if ownDeposit == "" {
		return empty, fmt.Errorf("payout excludes local stake")
	}
	raw, err := tx.Bytes()
	if err != nil {
		return empty, err
	}
	id := tx.TxHash().String()
	if old, ok := d.Settlements[id]; ok {
		if old.Scope != scope || old.Raw != hex.EncodeToString(raw) {
			return empty, fmt.Errorf("conflicting payout intent")
		}
		if old.State == "expired" {
			old.State = "awaiting_approval"
			old.ExpiresAt = now.Add(10 * time.Minute).Unix()
			old.Signatures = map[string][][]byte{}
			d.Settlements[id] = old
			if err = s.save(d); err != nil {
				return empty, err
			}
		}
		return old, nil
	}
	for _, old := range d.Settlements {
		if old.Scope == scope && old.Table == p.Table && old.State != "expired" && old.State != "rejected" {
			return empty, fmt.Errorf("table already has a payout proposal")
		}
	}
	out := Settlement{ID: id, Scope: scope, Table: p.Table, Raw: hex.EncodeToString(raw), Inputs: inputs, Payments: append([]finance.Payment(nil), built.Payments...), Destinations: destinations, FeeAtoms: built.FeeAtoms, ExpiresAt: now.Add(10 * time.Minute).Unix(), State: "awaiting_approval", Signatures: map[string][][]byte{}}
	d.Settlements[id] = out
	if err = s.save(d); err != nil {
		return empty, err
	}
	return out, nil
}
func formatOutput(hash string, index uint32) string { return fmt.Sprintf("%s:%d", hash, index) }

func (s *Store) Settlement(scope Scope, id string) (Settlement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return Settlement{}, err
	}
	p, ok := d.Settlements[id]
	if !ok || p.Scope != scope {
		return Settlement{}, fmt.Errorf("unknown payout")
	}
	if expireSettlements(&d, time.Now().Unix()) {
		if err = s.save(d); err != nil {
			return Settlement{}, err
		}
		p = d.Settlements[id]
	}
	return p, nil
}
func (s *Store) AllSettlements() ([]Settlement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	if expireSettlements(&d, time.Now().Unix()) {
		if err = s.save(d); err != nil {
			return nil, err
		}
	}
	out := make([]Settlement, 0, len(d.Settlements))
	for _, p := range d.Settlements {
		out = append(out, p)
	}
	return out, nil
}

// ApproveSettlement is an internal dashboard action, never a game RPC.
// Signatures are durable before they can be handed to the BR outbox.
func (s *Store) ApproveSettlement(scope Scope, id string, sign WalletSigner) ([][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	p, ok := d.Settlements[id]
	if !ok || p.Scope != scope {
		return nil, fmt.Errorf("unknown payout")
	}
	k, err := scopeKey(scope, p.Table)
	if err != nil {
		return nil, err
	}
	key, ok := d.Keys[k]
	if !ok {
		return nil, fmt.Errorf("wallet key missing")
	}
	if sigs := p.Signatures[key.Public]; len(sigs) > 0 {
		return sigs, nil
	}
	if p.State == "awaiting_approval" && p.ExpiresAt <= time.Now().Unix() {
		p.State = "expired"
		d.Settlements[id] = p
		if err = s.save(d); err != nil {
			return nil, err
		}
	}
	if p.State != "awaiting_approval" || d.Tables[k].Closed {
		return nil, fmt.Errorf("payout approval expired or table closed")
	}
	raw, err := hex.DecodeString(p.Raw)
	if err != nil {
		return nil, err
	}
	tx, err := finance.DecodeTransaction(raw)
	if err != nil {
		return nil, err
	}
	for _, op := range d.Operations {
		if op.Kind == "funding" {
			continue
		}
		for _, in := range p.Inputs {
			for _, reserved := range op.Inputs {
				if reserved == in.Outpoint.String() {
					return nil, fmt.Errorf("payout input already reserved")
				}
			}
		}
	}
	if sign == nil {
		return nil, fmt.Errorf("wallet signer missing")
	}
	sigs := make([][]byte, len(p.Inputs))
	refs := make([]string, len(p.Inputs))
	for i, input := range p.Inputs {
		hash, err := finance.SignatureHash(tx, i, input)
		if err != nil {
			return nil, err
		}
		sig, err := sign(key, hash)
		if err != nil {
			return nil, err
		}
		if err = finance.VerifySignature(tx, i, input, key.Public, sig); err != nil {
			return nil, err
		}
		sigs[i] = sig
		refs[i] = input.Outpoint.String()
	}
	p.Signatures[key.Public] = sigs
	p.State = "awaiting_signatures"
	d.Settlements[id] = p
	d.Operations[id] = Operation{ID: id, Scope: scope, Kind: "settlement", Inputs: refs, Raw: p.Raw, Approved: true, State: "awaiting_signatures"}
	if err = s.save(d); err != nil {
		return nil, err
	}
	return sigs, nil
}

// RejectSettlement records an operator's explicit refusal. An identical game
// retry returns this terminal decision; only a different payout intent can ask
// the operator again.
func (s *Store) RejectSettlement(scope Scope, id string) (Settlement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return Settlement{}, err
	}
	p, ok := d.Settlements[id]
	if !ok || p.Scope != scope {
		return Settlement{}, fmt.Errorf("unknown payout")
	}
	if p.State != "awaiting_approval" || len(p.Signatures) != 0 {
		return p, fmt.Errorf("payout is no longer awaiting approval")
	}
	p.State = "rejected"
	d.Settlements[id] = p
	if err = s.save(d); err != nil {
		return Settlement{}, err
	}
	return p, nil
}

// AddSettlementSignatures accepts only the financial key bound to this BR uid.
func (s *Store) AddSettlementSignatures(scope Scope, id, authenticatedUID string, sigs [][]byte, params stdaddr.AddressParams) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return false, err
	}
	p, ok := d.Settlements[id]
	if !ok || p.Scope != scope {
		return false, fmt.Errorf("unknown payout")
	}
	if p.State == "awaiting_approval" && p.ExpiresAt <= time.Now().Unix() {
		p.State = "expired"
		d.Settlements[id] = p
		if err = s.save(d); err != nil {
			return false, err
		}
	}
	if p.State != "awaiting_approval" && p.State != "awaiting_signatures" && p.State != "publishing" && p.State != "mempool" && p.State != "confirmed" {
		return false, fmt.Errorf("payout is not accepting signatures")
	}
	k, err := scopeKey(scope, p.Table)
	if err != nil {
		return false, err
	}
	peer, ok := d.Peers[k][authenticatedUID]
	if !ok {
		return false, fmt.Errorf("unrecognized payout signer")
	}
	raw, err := hex.DecodeString(p.Raw)
	if err != nil {
		return false, err
	}
	tx, err := finance.DecodeTransaction(raw)
	if err != nil {
		return false, err
	}
	if len(sigs) != len(p.Inputs) {
		return false, fmt.Errorf("incomplete payout signatures")
	}
	for i, input := range p.Inputs {
		if err = finance.VerifySignature(tx, i, input, peer.Key, sigs[i]); err != nil {
			return false, err
		}
	}
	// Once assembled, repeated wire deliveries must not reset chain state or
	// replace the exact signed bytes already approved for publication.
	if op, exists := d.Operations[id]; exists && op.State != "awaiting_signatures" {
		if !op.Approved || op.Scope != scope || op.Kind != "settlement" {
			return false, fmt.Errorf("invalid recorded payout operation")
		}
		return true, nil
	}
	if d.Tables[k].Closed {
		return false, fmt.Errorf("table closed; no further local payout assembly")
	}
	p.Signatures[peer.Key] = sigs
	ours, ok := d.Keys[k]
	if !ok {
		return false, fmt.Errorf("wallet key missing")
	}
	ready := len(p.Signatures) == len(d.Peers[k]) && len(p.Signatures[ours.Public]) > 0
	if ready {
		for i, input := range p.Inputs {
			byKey := map[string][]byte{}
			for key, rows := range p.Signatures {
				if len(rows) != len(p.Inputs) {
					return false, fmt.Errorf("inconsistent signature record")
				}
				byKey[key] = rows[i]
			}
			tx.TxIn[i].SignatureScript, err = finance.SettlementWitness(tx, i, input, byKey)
			if err != nil {
				return false, err
			}
		}
		if err = finance.VerifySpend(tx, p.Inputs, params); err != nil {
			return false, err
		}
		assembled, err := tx.Bytes()
		if err != nil {
			return false, err
		}
		op, ok := d.Operations[id]
		if !ok || !op.Approved || op.Scope != scope {
			return false, fmt.Errorf("local payout approval missing")
		}
		op.Raw = hex.EncodeToString(assembled)
		op.State = "publishing"
		d.Operations[id] = op
		p.State = "publishing"
	}
	d.Settlements[id] = p
	if err = s.save(d); err != nil {
		return false, err
	}
	return ready, nil
}
