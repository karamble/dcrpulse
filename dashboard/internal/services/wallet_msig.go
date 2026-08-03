// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/rpc"
)

// MsigOwnKey is this wallet's contribution to a shared multisig wallet: a
// fresh external address and its derivation coordinates. The registry
// records the coordinates so a seed-restored wallet can fast-forward its
// address cursor and recover the key.
type MsigOwnKey struct {
	PubKey  string `json:"pubKey"`
	Address string `json:"address"`
	Account uint32 `json:"account"`
	Branch  uint32 `json:"branch"`
	Index   uint32 `json:"index"`
}

// DeriveMsigKey reserves the next external address of the account and
// returns its compressed public key with derivation coordinates.
// validateaddress is the authority for the pubkey hex and coordinates; the
// gRPC NextAddress public_key field carries the pay-to-pubkey address
// form, not hex.
func DeriveMsigKey(ctx context.Context, account uint32) (*MsigOwnKey, error) {
	if rpc.WalletClient == nil {
		return nil, fmt.Errorf("wallet RPC client not initialized")
	}
	address, err := GetNextAddress(ctx, account)
	if err != nil {
		return nil, err
	}
	addrParam, err := json.Marshal(address)
	if err != nil {
		return nil, err
	}
	raw, err := rpc.WalletClient.RawRequest(ctx, "validateaddress", []json.RawMessage{addrParam})
	if err != nil {
		return nil, fmt.Errorf("validateaddress: %w", err)
	}
	var res struct {
		IsValid  bool    `json:"isvalid"`
		IsMine   bool    `json:"ismine"`
		PubKey   string  `json:"pubkey"`
		AccountN *uint32 `json:"accountn"`
		Branch   *uint32 `json:"branch"`
		Index    *uint32 `json:"index"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	if !res.IsValid || !res.IsMine {
		return nil, fmt.Errorf("derived address %s is not recognized by the wallet", address)
	}
	if res.PubKey == "" || res.Branch == nil || res.Index == nil {
		return nil, fmt.Errorf("wallet did not reveal derivation data for %s", address)
	}
	key := &MsigOwnKey{
		PubKey:  res.PubKey,
		Address: address,
		Account: account,
		Branch:  *res.Branch,
		Index:   *res.Index,
	}
	if res.AccountN != nil {
		key.Account = *res.AccountN
	}
	return key, nil
}

// ImportMsigScript registers a redeem script with the wallet so the P2SH
// address is watched. Freshly created scripts skip the rescan (nothing to
// find yet); a restore rescans from just before the recorded creation
// height. Re-importing an existing script is a no-op so activation and
// restore stay idempotent.
func ImportMsigScript(ctx context.Context, scriptHex string, rescan bool, scanFrom int64) error {
	if rpc.WalletClient == nil {
		return fmt.Errorf("wallet RPC client not initialized")
	}
	params := make([]json.RawMessage, 0, 3)
	for _, v := range []interface{}{scriptHex, rescan, scanFrom} {
		p, err := json.Marshal(v)
		if err != nil {
			return err
		}
		params = append(params, p)
	}
	_, err := rpc.WalletClient.RawRequest(ctx, "importscript", params)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "already exist") {
		return nil
	}
	return err
}

// SharedUTXO is one unspent output of a shared P2SH address.
type SharedUTXO struct {
	TxID          string `json:"txid"`
	Vout          uint32 `json:"vout"`
	Tree          int8   `json:"tree"`
	Atoms         int64  `json:"atoms"`
	Confirmations int64  `json:"confirmations"`
}

// ListSharedUTXOs returns every unspent output paying the shared address,
// including unconfirmed ones. Imported script outputs are watched by the
// wallet from the moment importscript runs.
func ListSharedUTXOs(ctx context.Context, address string) ([]SharedUTXO, error) {
	if rpc.WalletClient == nil {
		return nil, fmt.Errorf("wallet RPC client not initialized")
	}
	params := make([]json.RawMessage, 0, 3)
	for _, v := range []interface{}{0, 9999999, []string{address}} {
		p, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		params = append(params, p)
	}
	raw, err := rpc.WalletClient.RawRequest(ctx, "listunspent", params)
	if err != nil {
		return nil, fmt.Errorf("listunspent: %w", err)
	}
	var rows []struct {
		TxID          string  `json:"txid"`
		Vout          uint32  `json:"vout"`
		Tree          int8    `json:"tree"`
		Amount        float64 `json:"amount"`
		Confirmations int64   `json:"confirmations"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	utxos := make([]SharedUTXO, 0, len(rows))
	for _, row := range rows {
		amt, err := dcrutil.NewAmount(row.Amount)
		if err != nil {
			return nil, fmt.Errorf("invalid amount for %s:%d: %v", row.TxID, row.Vout, err)
		}
		utxos = append(utxos, SharedUTXO{
			TxID:          row.TxID,
			Vout:          row.Vout,
			Tree:          row.Tree,
			Atoms:         int64(amt),
			Confirmations: row.Confirmations,
		})
	}
	return utxos, nil
}

// MsigTxConfirmations reports whether the wallet has seen txid and with
// how many confirmations (0 = mempool). known is false when the wallet
// does not know the transaction at all; that is a normal answer, not an
// error.
func MsigTxConfirmations(ctx context.Context, txid string) (int64, bool, error) {
	if rpc.WalletClient == nil {
		return 0, false, fmt.Errorf("wallet RPC client not initialized")
	}
	param, err := json.Marshal(txid)
	if err != nil {
		return 0, false, err
	}
	raw, err := rpc.WalletClient.RawRequest(ctx, "gettransaction", []json.RawMessage{param})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no information") {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("gettransaction: %w", err)
	}
	var info struct {
		Confirmations int64 `json:"confirmations"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return 0, false, err
	}
	return info.Confirmations, true, nil
}

// MsigPrevInput hands signrawtransaction a prevout it may not be able to
// resolve on its own (unconfirmed funding, pruned backend). The redeem
// script is never passed: without raw private keys dcrwallet ignores it
// and always resolves redeem scripts from its own store, which is why the
// script must be imported before signing.
type MsigPrevInput struct {
	TxID         string `json:"txid"`
	Vout         uint32 `json:"vout"`
	Tree         int8   `json:"tree"`
	ScriptPubKey string `json:"scriptPubKey"`
}

// SignMsigTransaction adds this wallet's signatures to the shared multisig
// inputs of rawTxHex and returns the updated transaction without
// broadcasting it. Unlock discipline mirrors SignAndPublishTransaction:
// signing authority is the per-account unlock of the account whose key
// participates. The returned hex is authoritative; the complete flag is
// ignored because dcrwallet reports partially signed multisig inputs as
// complete by design.
func SignMsigTransaction(ctx context.Context, rawTxHex string, prevInputs []MsigPrevInput, account uint32, passphrase []byte) (string, error) {
	if rpc.WalletClient == nil || rpc.WalletGrpcClient == nil {
		return "", fmt.Errorf("wallet RPC clients not initialized")
	}
	if IsMixerRunning() || IsAutobuyerRunning() {
		return "", ErrSpendWhileMixing
	}
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	beginUnlockedOp()
	defer endUnlockedOp()

	didUnlock, err := unlockAccountForSpend(ctx, account, passphrase)
	if err != nil {
		return "", err
	}
	if didUnlock {
		defer func() {
			relockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := rpc.WalletGrpcClient.LockAccount(relockCtx, &walletrpc.LockAccountRequest{AccountNumber: account}); err != nil {
				log.Printf("SignMsigTransaction: lock account %d: %v", account, err)
			}
		}()
	}

	txParam, err := json.Marshal(rawTxHex)
	if err != nil {
		return "", err
	}
	inputsParam, err := json.Marshal(prevInputs)
	if err != nil {
		return "", err
	}
	raw, err := rpc.WalletClient.RawRequest(ctx, "signrawtransaction", []json.RawMessage{txParam, inputsParam})
	if err != nil {
		return "", fmt.Errorf("signrawtransaction: %w", err)
	}
	var res struct {
		Hex    string `json:"hex"`
		Errors []struct {
			TxID  string `json:"txid"`
			Vout  uint32 `json:"vout"`
			Error string `json:"error"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	for _, e := range res.Errors {
		log.Printf("SignMsigTransaction: input %s:%d: %s", e.TxID, e.Vout, e.Error)
	}
	if res.Hex == "" {
		return "", fmt.Errorf("signrawtransaction returned no transaction")
	}
	return res.Hex, nil
}

// WalletTipHeight returns the wallet's best block height, used to record
// a shared wallet's creation height for bounded restore rescans.
func WalletTipHeight(ctx context.Context) (int64, error) {
	if rpc.WalletClient == nil {
		return 0, fmt.Errorf("wallet RPC client not initialized")
	}
	_, height, err := rpc.WalletClient.GetBestBlock(ctx)
	return height, err
}

// SyncAccountAddressIndex fast-forwards the wallet's derived-address
// cursor for one account branch. Shared-wallet restore runs this before
// validateaddress so a seed-restored wallet recognizes its own key even
// though the address never appeared on-chain as P2PKH.
func SyncAccountAddressIndex(ctx context.Context, accountName string, branch, index uint32) error {
	if rpc.WalletClient == nil {
		return fmt.Errorf("wallet RPC client not initialized")
	}
	params := make([]json.RawMessage, 0, 3)
	for _, v := range []interface{}{accountName, branch, index} {
		p, err := json.Marshal(v)
		if err != nil {
			return err
		}
		params = append(params, p)
	}
	_, err := rpc.WalletClient.RawRequest(ctx, "accountsyncaddressindex", params)
	return err
}
