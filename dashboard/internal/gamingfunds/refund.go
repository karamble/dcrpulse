package gamingfunds

import (
	"encoding/hex"
	"fmt"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
)

// refundTransaction is private: game RPCs cannot request a signature from this
// primitive. Dashboard recovery validates fresh chain state and approves the
// exact output, fee, and destination before calling it.
type WalletSigner func(WalletKey, []byte) ([]byte, error)

func refundTransaction(terms Terms, key WalletKey, sign WalletSigner, prev wire.OutPoint, pay []byte, fee int64, params stdaddr.AddressParams) (*wire.MsgTx, error) {
	if key.Public != terms.Recovery || sign == nil {
		return nil, fmt.Errorf("wallet recovery signer unavailable")
	}
	input := finance.Input{Terms: terms, Outpoint: prev}
	tx, err := finance.Refund(input, pay, fee)
	if err != nil {
		return nil, err
	}
	hash, err := finance.SignatureHash(tx, 0, input)
	if err != nil {
		return nil, err
	}
	sig, err := sign(key, hash)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript, err = finance.RefundWitness(tx, 0, input, sig)
	if err != nil {
		return nil, err
	}
	if err = finance.VerifySpend(tx, []finance.Input{input}, params); err != nil {
		return nil, err
	}
	return tx, nil
}

// VerifyRecovery checks immutable script and wallet locator consistency. The
// wallet adapter independently checks ownership; signing never happens here.
func (s *Store) VerifyRecovery(scope Scope, id string, params stdaddr.AddressParams) error {
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
	k, err := scopeKey(scope, dep.Terms.Table)
	if err != nil {
		return err
	}
	rec, ok := d.Keys[k]
	if !ok || rec.Public != dep.Terms.Recovery || rec.Scope != scope || rec.Address == "" {
		return fmt.Errorf("wallet recovery authority missing")
	}
	script, err := dep.Terms.Script()
	if err != nil {
		return err
	}
	addr, pk, err := dep.Terms.Output(params)
	if err != nil {
		return err
	}
	if hex.EncodeToString(script) != dep.Script || addr != dep.Address || hex.EncodeToString(pk) != dep.PkScript {
		return fmt.Errorf("recovery descriptor mismatch")
	}
	return nil
}

type RecoveryQuote struct {
	ID          string `json:"id"`
	DepositID   string `json:"depositId"`
	Destination string `json:"destination"`
	FeeAtoms    int64  `json:"feeAtoms"`
	ReturnAtoms int64  `json:"returnAtoms"`
	ExpiresAt   int64  `json:"expiresAt"`
	TxID        string `json:"txid,omitempty"`
}

func (s *Store) CloseTable(scope Scope, table string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	found := false
	if key, err := scopeKey(scope, table); err == nil {
		if t, ok := d.Tables[key]; ok {
			t.Closed = true
			d.Tables[key] = t
			found = true
		}
	}
	for id, dep := range d.Deposits {
		if dep.Scope == scope && dep.Terms.Table == table {
			dep.Closed = true
			d.Deposits[id] = dep
			found = true
		}
	}
	if !found {
		return fmt.Errorf("unknown table")
	}
	return s.save(d)
}
func (s *Store) QuoteRecovery(scope Scope, id, dest string, fee int64, params stdaddr.AddressParams) (RecoveryQuote, error) {
	var zero RecoveryQuote
	a, err := stdaddr.DecodeAddress(dest, params)
	if err != nil {
		return zero, err
	}
	_ = a
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return zero, err
	}
	dep, ok := d.Deposits[id]
	if !ok || dep.Scope != scope {
		return zero, fmt.Errorf("unknown deposit")
	}
	if !dep.Closed {
		return zero, fmt.Errorf("close the table before recovery")
	}
	if dep.Outpoint == "" {
		return zero, fmt.Errorf("deposit has no funded output")
	}
	if fee <= 0 || fee > 100000 || dep.Terms.Atoms-fee < 10000 {
		return zero, fmt.Errorf("insufficient amount after recovery fee")
	}
	prev, err := parseOutput(dep.Outpoint)
	if err != nil {
		return zero, err
	}
	for _, op := range d.Operations {
		for _, input := range op.Inputs {
			if op.Kind != "settlement" && input == prev.String() {
				return zero, fmt.Errorf("deposit already has a pending transaction")
			}
		}
	}
	token, err := randomID()
	if err != nil {
		return zero, err
	}
	q := RecoveryQuote{ID: token, DepositID: id, Destination: dest, FeeAtoms: fee, ReturnAtoms: dep.Terms.Atoms - fee, ExpiresAt: time.Now().Add(2 * time.Minute).Unix()}
	d.Quotes[token] = q
	if err = s.save(d); err != nil {
		return zero, err
	}
	return q, nil
}

// ApproveRecovery is called only by the dashboard after fresh chain and wallet
// checks. It signs internally and durably reserves the output before returning.
func (s *Store) ApproveRecovery(scope Scope, quoteID string, params stdaddr.AddressParams, sign WalletSigner) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	q, ok := d.Quotes[quoteID]
	if !ok {
		return nil, fmt.Errorf("recovery quote unavailable")
	}
	dep, ok := d.Deposits[q.DepositID]
	if !ok || dep.Scope != scope {
		return nil, fmt.Errorf("unknown recovery deposit")
	}
	if q.TxID != "" {
		op, ok := d.Operations[q.TxID]
		if !ok || !op.Approved || op.Scope != scope {
			return nil, fmt.Errorf("recovery journal inconsistent")
		}
		return hex.DecodeString(op.Raw)
	}
	if time.Now().Unix() >= q.ExpiresAt || !dep.Closed {
		return nil, fmt.Errorf("quote expired or table not closed")
	}
	prev, err := parseOutput(dep.Outpoint)
	if err != nil {
		return nil, err
	}
	for _, op := range d.Operations {
		for _, input := range op.Inputs {
			if op.Kind != "settlement" && input == prev.String() {
				return nil, fmt.Errorf("deposit already reserved")
			}
		}
	}
	k, err := scopeKey(scope, dep.Terms.Table)
	if err != nil {
		return nil, err
	}
	rec, ok := d.Keys[k]
	if !ok {
		return nil, fmt.Errorf("recovery key unavailable")
	}
	dest, err := stdaddr.DecodeAddress(q.Destination, params)
	if err != nil {
		return nil, err
	}
	_, pay := dest.PaymentScript()
	tx, err := refundTransaction(dep.Terms, rec, sign, prev, pay, q.FeeAtoms, params)
	if err != nil {
		return nil, err
	}
	raw, err := tx.Bytes()
	if err != nil {
		return nil, err
	}
	id := tx.TxHash().String()
	d.Operations[id] = Operation{ID: id, Scope: scope, Kind: "recovery", DepositIDs: []string{dep.ID}, Inputs: []string{prev.String()}, Raw: hex.EncodeToString(raw), Approved: true, State: "publishing"}
	q.TxID = id
	d.Quotes[quoteID] = q
	dep.State = "recovery_pending"
	d.Deposits[dep.ID] = dep
	if err = s.save(d); err != nil {
		return nil, err
	}
	return raw, nil
}
func (s *Store) RecoveryQuote(scope Scope, id string) (RecoveryQuote, Deposit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return RecoveryQuote{}, Deposit{}, err
	}
	q, ok := d.Quotes[id]
	dep, has := d.Deposits[q.DepositID]
	if !ok || !has || dep.Scope != scope {
		return RecoveryQuote{}, Deposit{}, fmt.Errorf("unknown recovery quote")
	}
	return q, dep, nil
}
func parseOutput(raw string) (wire.OutPoint, error) {
	var out wire.OutPoint
	tx, index, ok := strings.Cut(raw, ":")
	if !ok {
		return out, fmt.Errorf("invalid output reference")
	}
	hash, err := chainhash.NewHashFromStr(tx)
	if err != nil {
		return out, err
	}
	n, err := strconv.ParseUint(index, 10, 32)
	if err != nil {
		return out, err
	}
	return wire.OutPoint{Hash: *hash, Index: uint32(n), Tree: wire.TxTreeRegular}, nil
}
