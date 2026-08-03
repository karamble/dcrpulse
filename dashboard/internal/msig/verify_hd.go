// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"encoding/hex"
	"fmt"

	"github.com/decred/dcrd/wire"
)

// An HD spend's inputs may sit at different ladder indices, each with its
// own redeem script and its own child-key set. Signatures therefore
// attribute per input, and progress counts PARTICIPANTS, not keys: the
// same member signs input 0 with a different pubkey than input 1, and a
// signature only counts once its owner has signed every input.

// InputResolver returns one input's redeem script and the map from
// lowercase child-pubkey hex to the owning participant's roster xpub.
type InputResolver func(idx int) (redeem []byte, keyOwner map[string]string, err error)

// resolverForInputs builds the resolver for a proposal's recorded
// inputs. Every input address must be a known ladder rung.
func resolverForInputs(rec *WalletRecord, inputs []ProposalInput) (InputResolver, error) {
	view, err := ladderFor(rec)
	if err != nil {
		return nil, err
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return nil, err
	}
	type resolved struct {
		redeem   []byte
		keyOwner map[string]string
	}
	byIdx := make([]resolved, len(inputs))
	for i, in := range inputs {
		entry, ok := view.byAddr[in.Address]
		if !ok {
			return nil, fmt.Errorf("input %s:%d spends an address outside this wallet's ladder", in.TxID, in.Vout)
		}
		keys, err := KeysAt(rec.Xpubs, entry.Branch, entry.Index, params)
		if err != nil {
			return nil, err
		}
		owner := make(map[string]string, len(keys))
		for ki, k := range keys {
			owner[hex.EncodeToString(k)] = rec.Xpubs[ki]
		}
		byIdx[i] = resolved{redeem: entry.Script, keyOwner: owner}
	}
	return func(idx int) ([]byte, map[string]string, error) {
		if idx < 0 || idx >= len(byIdx) {
			return nil, nil, fmt.Errorf("input index %d out of range", idx)
		}
		return byIdx[idx].redeem, byIdx[idx].keyOwner, nil
	}, nil
}

// VerifyProposalUpdateHD checks a transaction against the proposal's
// txid and returns the PARTICIPANTS (by roster xpub) whose signatures
// are present on every input. The txid covers the whole prefix, so the
// single comparison proves inputs, outputs, lock time and expiry are
// untouched; only signature scripts can legally differ.
func VerifyProposalUpdateHD(updated *wire.MsgTx, expectedTxID string, resolve InputResolver) (map[string]bool, error) {
	if updated.TxHash().String() != expectedTxID {
		return nil, fmt.Errorf("transaction prefix was altered: txid changed")
	}
	if len(updated.TxIn) == 0 {
		return nil, fmt.Errorf("transaction has no inputs")
	}
	var common map[string]bool
	for idx := range updated.TxIn {
		redeem, keyOwner, err := resolve(idx)
		if err != nil {
			return nil, err
		}
		keys := make([][]byte, 0, len(keyOwner))
		for kh := range keyOwner {
			k, err := hex.DecodeString(kh)
			if err != nil {
				return nil, err
			}
			keys = append(keys, k)
		}
		signedKeys, err := AttributeSigs(updated, idx, redeem, keys)
		if err != nil {
			return nil, err
		}
		owners := make(map[string]bool, len(signedKeys))
		for kh := range signedKeys {
			owners[keyOwner[kh]] = true
		}
		if common == nil {
			common = owners
			continue
		}
		for o := range common {
			if !owners[o] {
				delete(common, o)
			}
		}
	}
	return common, nil
}
