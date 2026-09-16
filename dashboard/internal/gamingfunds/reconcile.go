package gamingfunds

import (
	"encoding/hex"
	"fmt"
	"strings"
)

type ChainObservation struct {
	Known         bool
	Confirmations int64
	BlockHash     string
	Height        int64
}

// DepositObservation is the independently observed state of one registered
// output. SpendingTx may name a peer payout or unilateral recovery that this
// bridge did not create itself.
type DepositObservation struct {
	OutputFound        bool
	Confirmations      int64
	FundingBlock       string
	FundingHeight      int64
	SpendingTx         string
	SpendConfirmations int64
}

// ObserveDeposit records output/spend facts obtained from dcrd and dcrwallet.
// It is deliberately not exposed over the game protocol.
func (s *Store) ObserveDeposit(scope Scope, id string, facts DepositObservation) error {
	if facts.Confirmations < 0 || facts.FundingHeight < 0 || facts.SpendConfirmations < 0 || (facts.OutputFound && facts.SpendingTx != "") {
		return fmt.Errorf("invalid deposit observation")
	}
	if facts.Confirmations > 0 {
		block, err := hex.DecodeString(facts.FundingBlock)
		if err != nil || len(block) != 32 || facts.FundingHeight == 0 {
			return fmt.Errorf("confirmed deposit observation lacks a block")
		}
	} else if facts.FundingBlock != "" || facts.FundingHeight != 0 {
		return fmt.Errorf("unconfirmed deposit observation has a block")
	}
	if facts.SpendingTx != "" {
		txid, err := hex.DecodeString(strings.TrimSpace(facts.SpendingTx))
		if err != nil || len(txid) != 32 {
			return fmt.Errorf("invalid spending transaction")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	dep, ok := d.Deposits[id]
	if !ok || dep.Scope != scope {
		return fmt.Errorf("unknown deposit")
	}
	switch {
	case facts.OutputFound:
		dep.SpendingTx = ""
		dep.Confirmations = facts.Confirmations
		dep.FundingBlock = facts.FundingBlock
		dep.FundingHeight = facts.FundingHeight
		if facts.Confirmations > 0 {
			dep.State = "confirmed"
		} else {
			dep.State = "mempool"
		}
	case facts.SpendingTx != "":
		dep.SpendingTx = strings.TrimSpace(facts.SpendingTx)
		if facts.SpendConfirmations > 0 {
			dep.State = "spent"
		} else {
			dep.State = "spend_pending"
		}
	default:
		dep.SpendingTx = ""
		dep.State = "needs_attention"
	}
	d.Deposits[id] = dep
	return s.save(d)
}

func (s *Store) Operations() ([]Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]Operation, 0, len(d.Operations))
	for _, op := range d.Operations {
		out = append(out, op)
	}
	return out, nil
}
func (s *Store) Tables() ([]TableAuthorization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]TableAuthorization, 0, len(d.Tables))
	for _, t := range d.Tables {
		out = append(out, t)
	}
	return out, nil
}

// ObserveOperation records actual node observations. An absent transaction is
// unknown, never proof of a spent output. Confirmations may fall after a reorg.
func (s *Store) ObserveOperation(id string, facts ChainObservation) error {
	if facts.Confirmations < 0 || facts.Height < 0 || (!facts.Known && (facts.Confirmations != 0 || facts.BlockHash != "")) {
		return fmt.Errorf("invalid chain observation")
	}
	if facts.Confirmations > 0 {
		hash, err := hex.DecodeString(facts.BlockHash)
		if err != nil || len(hash) != 32 || facts.Height == 0 {
			return fmt.Errorf("confirmed observation lacks a block")
		}
	} else if facts.BlockHash != "" || facts.Height != 0 {
		return fmt.Errorf("unconfirmed observation has a block")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	op, ok := d.Operations[id]
	if !ok {
		return fmt.Errorf("unknown operation")
	}
	if op.State == "awaiting_signatures" {
		return nil
	}
	op.State = "publishing"
	if facts.Known {
		op.State = "mempool"
	}
	if facts.Known && facts.Confirmations > 0 {
		op.State = "confirmed"
	}
	op.Confirmations = facts.Confirmations
	op.BlockHash = facts.BlockHash
	op.Height = facts.Height
	d.Operations[id] = op
	if p, ok := d.Settlements[id]; ok {
		p.State = op.State
		if p.State == "mempool" {
			p.State = "publishing"
		}
		d.Settlements[id] = p
	}
	for depID, dep := range d.Deposits {
		if dep.Scope != op.Scope {
			continue
		}
		if dep.FundingTx == id {
			dep.FundingHeight = facts.Height
			dep.FundingBlock = facts.BlockHash
			dep.Confirmations = facts.Confirmations
			if dep.SpendingTx == "" {
				dep.State = op.State
			}
		}
		if op.Kind != "funding" {
			for _, input := range op.Inputs {
				prev, err := parseOutput(dep.Outpoint)
				if err != nil {
					continue
				}
				if prev.String() != input {
					continue
				}
				if facts.Known && facts.Confirmations > 0 {
					dep.SpendingTx = id
					dep.State = "spent"
				} else if dep.SpendingTx == id {
					dep.SpendingTx = ""
					dep.State = "needs_attention"
				}
			}
		}
		d.Deposits[depID] = dep
	}
	return s.save(d)
}
