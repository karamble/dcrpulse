package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"dcrpulse/internal/gamingfunds"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
)

// validateGamingFunding inspects the concrete wallet-built transaction before
// the approval is visible and again before signing. Only the exact registered
// deposit and change to the bound account are allowed.
func validateGamingFunding(ctx context.Context, dep gamingfunds.Deposit, raw []byte) (int64, error) {
	if len(raw) == 0 || len(raw) > maxGamingTxBytes {
		return 0, fmt.Errorf("invalid funding transaction size")
	}
	tx, err := finance.DecodeTransaction(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid funding transaction")
	}
	if len(tx.TxIn) == 0 || len(tx.TxOut) == 0 || tx.LockTime != 0 || tx.Expiry != 0 {
		return 0, fmt.Errorf("unsupported funding transaction shape")
	}
	params, err := chainParams(ctx)
	if err != nil {
		return 0, err
	}
	expected, err := hex.DecodeString(dep.PkScript)
	if err != nil {
		return 0, err
	}
	var input, output int64
	seen := map[wire.OutPoint]bool{}
	for index, in := range tx.TxIn {
		if seen[in.PreviousOutPoint] || (in.PreviousOutPoint.Tree != wire.TxTreeRegular && in.PreviousOutPoint.Tree != wire.TxTreeStake) {
			return 0, fmt.Errorf("duplicate or unsupported funding input")
		}
		seen[in.PreviousOutPoint] = true
		facts, err := spendPrevout(ctx, in.PreviousOutPoint)
		if err != nil {
			return 0, err
		}
		if !facts.Found || facts.ValueAtoms <= 0 || facts.ValueAtoms > maxSpendAtoms-input {
			return 0, fmt.Errorf("funding input unavailable or amount invalid")
		}
		if len(facts.Addresses) != 1 {
			return 0, fmt.Errorf("funding input ownership is ambiguous")
		}
		owner, err := ValidateAddress(ctx, facts.Addresses[0])
		if err != nil {
			return 0, err
		}
		if !owner.IsValid || !owner.IsMine || owner.AccountNumber != dep.Scope.Account {
			return 0, fmt.Errorf("funding input is not in the bound wallet account")
		}
		if in.ValueIn != facts.ValueAtoms {
			return 0, fmt.Errorf("funding input value differs from chain")
		}
		if len(in.SignatureScript) > 0 {
			if len(facts.PkScript) == 0 {
				return 0, fmt.Errorf("funding input script unavailable")
			}
			vm, e := txscript.NewEngine(facts.PkScript, tx, index, txscript.ScriptVerifyCheckSequenceVerify|txscript.ScriptVerifyCleanStack|txscript.ScriptVerifySigPushOnly, facts.ScriptVersion, nil)
			if e != nil {
				return 0, e
			}
			if e = vm.Execute(); e != nil {
				return 0, fmt.Errorf("wallet funding signature is invalid: %w", e)
			}
		}
		input += facts.ValueAtoms
	}
	matches := 0
	for _, out := range tx.TxOut {
		if out.Version != 0 || out.Value <= 0 || out.Value > maxSpendAtoms-output {
			return 0, fmt.Errorf("invalid funding output")
		}
		output += out.Value
		if bytes.Equal(out.PkScript, expected) {
			if out.Value != dep.Terms.Atoms {
				return 0, fmt.Errorf("funding amount differs from registered terms")
			}
			matches++
			continue
		}
		_, addresses := stdscript.ExtractAddrs(out.Version, out.PkScript, params)
		if len(addresses) != 1 {
			return 0, fmt.Errorf("unrecognized funding change")
		}
		owner, err := ValidateAddress(ctx, addresses[0].String())
		if err != nil {
			return 0, err
		}
		if !owner.IsValid || !owner.IsMine || owner.AccountNumber != dep.Scope.Account {
			return 0, fmt.Errorf("funding change leaves the bound account")
		}
	}
	if matches != 1 {
		return 0, fmt.Errorf("funding must contain exactly one registered deposit")
	}
	fee := input - output
	if fee <= 0 || fee > 100000 {
		return 0, fmt.Errorf("funding fee is outside the bridge limit")
	}
	return fee, nil
}

// sameFundingPrefix prevents a signer from changing recipients, fees, input
// references, sequence locks or expiry after the operator approved the preview.
func sameFundingPrefix(unsigned, signed []byte) error {
	a, err := finance.DecodeTransaction(unsigned)
	if err != nil {
		return err
	}
	b, err := finance.DecodeTransaction(signed)
	if err != nil {
		return err
	}
	if !finance.SameIntent(a, b) {
		return fmt.Errorf("signed funding differs from the approved transaction")
	}
	return nil
}
