// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/hdkeychain/v3"
)

// The device exports account dpubs to the microSD card as a CBOR AccountExport
// file (accounts.dcr), so a companion without a camera can import watch-only
// accounts without typing keys:
//
//	AccountExport   = array[2]{format_version=1, accounts []ExportedAccount}
//	ExportedAccount = array[3]{account u32 (BIP44 index), dpub tstr, name tstr}
//
// The account index is authoritative from the device - offline signing derives
// against it. The name is a suggestion. Decoders accept arrays longer than
// declared and ignore the extra fields (append-only evolution).

const (
	accountExportFormatVersion = 1
	accountExportMaxBytes      = 1 << 16
	accountExportMaxEntries    = 64
)

// ParseAccountExport decodes and validates a device account-export file. Every
// dpub must parse as an extended public key for the wallet's network; the
// returned fingerprint is computed here (first 4 bytes of dcrutil.Hash160 of
// the dpub's own compressed pubkey) for display cross-checks against the
// device. Parse-only: nothing is imported.
func ParseAccountExport(ctx context.Context, data []byte) ([]types.AccountExportEntry, error) {
	params, err := hdChainParams(ctx)
	if err != nil {
		return nil, err
	}
	return parseAccountExport(data, params)
}

func parseAccountExport(data []byte, params *chaincfg.Params) ([]types.AccountExportEntry, error) {
	if len(data) > accountExportMaxBytes {
		return nil, fmt.Errorf("file too large for an account export")
	}

	r := &cborReader{buf: data}
	top, err := r.arrayHead()
	if err != nil {
		return nil, fmt.Errorf("not an account export file: %v", err)
	}
	if top < 2 {
		return nil, fmt.Errorf("not an account export file: too few fields")
	}
	ver, err := r.uint()
	if err != nil {
		return nil, fmt.Errorf("not an account export file: %v", err)
	}
	if ver != accountExportFormatVersion {
		return nil, fmt.Errorf("unsupported account export version %d", ver)
	}
	count, err := r.arrayHead()
	if err != nil {
		return nil, fmt.Errorf("malformed account list: %v", err)
	}
	if count > accountExportMaxEntries {
		return nil, fmt.Errorf("too many accounts in the export (%d)", count)
	}

	entries := make([]types.AccountExportEntry, 0, count)
	seen := make(map[uint32]bool, count)
	for i := 0; i < count; i++ {
		fields, err := r.arrayHead()
		if err != nil || fields < 3 {
			return nil, fmt.Errorf("malformed account entry %d", i)
		}
		acct, err := r.uint()
		if err != nil {
			return nil, fmt.Errorf("entry %d: bad account index: %v", i, err)
		}
		if acct >= 1<<31 {
			return nil, fmt.Errorf("entry %d: account index %d out of BIP44 range", i, acct)
		}
		dpub, err := r.text(200)
		if err != nil {
			return nil, fmt.Errorf("entry %d: bad dpub: %v", i, err)
		}
		name, err := r.text(200)
		if err != nil {
			return nil, fmt.Errorf("entry %d: bad name: %v", i, err)
		}
		for f := 3; f < fields; f++ {
			if err := r.skip(); err != nil {
				return nil, fmt.Errorf("entry %d: %v", i, err)
			}
		}
		if seen[uint32(acct)] {
			return nil, fmt.Errorf("account index %d appears twice in the export", acct)
		}
		seen[uint32(acct)] = true

		key, err := hdkeychain.NewKeyFromString(dpub, params)
		if err != nil {
			return nil, fmt.Errorf("entry %d (account %d): invalid extended public key for this network: %v", i, acct, err)
		}
		if key.IsPrivate() {
			return nil, fmt.Errorf("entry %d (account %d): file contains a PRIVATE key, refusing", i, acct)
		}
		if len(name) > 50 {
			name = name[:50]
		}
		entries = append(entries, types.AccountExportEntry{
			Account: uint32(acct),
			Dpub:    dpub,
			Name:    name,
			Fp:      hex.EncodeToString(dcrutil.Hash160(key.SerializedPubKey())[:4]),
		})
	}
	return entries, nil
}

// XpubAlreadyImported reports whether an existing account is backed by the
// same extended public key. Keys are compared by their serialized pubkey, not
// the base58 string, so differing metadata cannot mask a duplicate.
func XpubAlreadyImported(ctx context.Context, dpub string) (string, bool, error) {
	params, err := hdChainParams(ctx)
	if err != nil {
		return "", false, err
	}
	candidate, err := hdkeychain.NewKeyFromString(dpub, params)
	if err != nil {
		return "", false, fmt.Errorf("invalid extended public key: %v", err)
	}
	cand := candidate.SerializedPubKey()
	if rpc.WalletGrpcClient == nil {
		return "", false, fmt.Errorf("wallet gRPC client not initialized")
	}
	accts, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{})
	if err != nil {
		return "", false, err
	}
	for _, a := range accts.Accounts {
		// The literal "imported" bucket holds single keys, not an xpub.
		if a.AccountName == "imported" {
			continue
		}
		existing, xerr := GetAccountExtendedPubKey(ctx, a.AccountNumber)
		if xerr != nil {
			continue
		}
		key, kerr := hdkeychain.NewKeyFromString(existing, params)
		if kerr != nil {
			continue
		}
		if bytes.Equal(key.SerializedPubKey(), cand) {
			return a.AccountName, true, nil
		}
	}
	return "", false, nil
}

// AnnotateAccountExportConflicts classifies each entry against the existing
// accounts. Same xpub AND same BIP44 index = the benign duplicate
// (AlreadyImported; the UI skips it silently). A PARTIAL match - the same key
// under a different index, or the index held by a different key - sets
// Conflict, because that means the device export and the wallet disagree and
// the user must look. Name collisions are neither: the name is an editable
// suggestion. Best-effort: if the wallet is unreachable the entries stay
// unannotated and the import-time guards decide.
func AnnotateAccountExportConflicts(ctx context.Context, entries []types.AccountExportEntry) {
	params, err := hdChainParams(ctx)
	if err != nil || rpc.WalletGrpcClient == nil {
		return
	}
	accts, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{})
	if err != nil {
		return
	}
	type existingAccount struct {
		name   string
		pubkey []byte
		index  uint32
		hasIdx bool
	}
	var existing []existingAccount
	for _, a := range accts.Accounts {
		// The literal "imported" bucket holds single keys, not an xpub.
		if a.AccountName == "imported" {
			continue
		}
		xp, xerr := GetAccountExtendedPubKey(ctx, a.AccountNumber)
		if xerr != nil {
			continue
		}
		key, kerr := hdkeychain.NewKeyFromString(xp, params)
		if kerr != nil {
			continue
		}
		ea := existingAccount{name: a.AccountName, pubkey: key.SerializedPubKey()}
		if idx, ierr := Bip44AccountIndex(ctx, a.AccountNumber); ierr == nil {
			ea.index, ea.hasIdx = idx, true
		}
		existing = append(existing, ea)
	}

	for i := range entries {
		key, kerr := hdkeychain.NewKeyFromString(entries[i].Dpub, params)
		if kerr != nil {
			continue
		}
		pub := key.SerializedPubKey()

		matchedKey := false
		for _, ea := range existing {
			if !bytes.Equal(ea.pubkey, pub) {
				continue
			}
			matchedKey = true
			if ea.hasIdx && ea.index == entries[i].Account {
				entries[i].AlreadyImported = true
			} else {
				entries[i].Conflict = fmt.Sprintf(
					"this key is already imported as account %q under a different index - check the device export", ea.name)
			}
			break
		}
		if matchedKey {
			continue
		}
		for _, ea := range existing {
			if ea.hasIdx && ea.index == entries[i].Account {
				entries[i].Conflict = fmt.Sprintf(
					"account index %d is already used by account %q with a DIFFERENT key - something is wrong", entries[i].Account, ea.name)
				break
			}
		}
	}
}
