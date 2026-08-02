// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// ListForActiveWallet returns the active wallet's shared-wallet records,
// newest first, plus the wallet's name.
func ListForActiveWallet(ctx context.Context) (string, []*WalletRecord, error) {
	network, err := networkSeam(ctx)
	if err != nil {
		return "", nil, err
	}
	store, err := manager(network).StoreFor(activeWalletSeam())
	if err != nil {
		return "", nil, err
	}
	recs := store.Wallets()
	sort.Slice(recs, func(i, j int) bool { return recs[i].CreatedAt > recs[j].CreatedAt })
	return store.WalletName(), recs, nil
}

// Detail returns one record, its owning wallet's name and whether that
// wallet is the active one.
func Detail(ctx context.Context, id string) (*WalletRecord, string, bool, error) {
	network, err := networkSeam(ctx)
	if err != nil {
		return nil, "", false, err
	}
	store, rec := manager(network).Route(id, nil)
	if rec == nil {
		return nil, "", false, fmt.Errorf("unknown shared wallet %s", id)
	}
	return rec, store.WalletName(), store.WalletName() == activeWalletSeam(), nil
}

// BackupCard bundles everything a restore needs: the roster, the script,
// the address and this wallet's own key coordinates. Proposals and
// journals are device-local and deliberately excluded.
type BackupCard struct {
	CardVersion int           `json:"cardVersion"`
	WalletName  string        `json:"walletName"`
	ExportedAt  int64         `json:"exportedAt"`
	Record      *WalletRecord `json:"record"`
}

// ExportBackupCard builds the backup card for one shared wallet.
func ExportBackupCard(ctx context.Context, id string) (*BackupCard, error) {
	rec, walletName, _, err := Detail(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec.ScriptHex == "" || rec.Address == "" {
		return nil, fmt.Errorf("shared wallet %q has not activated yet; nothing to back up", rec.Label)
	}
	return &BackupCard{
		CardVersion: 1,
		WalletName:  walletName,
		ExportedAt:  time.Now().Unix(),
		Record:      rec,
	}, nil
}

// PendingItem is one action waiting on the user, across every local
// wallet: an invite to answer or a verified roster waiting for its
// wallet to become active.
type PendingItem struct {
	WalletName    string `json:"walletName"`
	TempID        string `json:"tempId"`
	Label         string `json:"label"`
	M             int    `json:"m"`
	N             int    `json:"n"`
	Status        string `json:"status"`
	InitiatorNick string `json:"initiatorNick,omitempty"`
	NeedsSwitch   bool   `json:"needsSwitch"`
	Kind          string `json:"kind"`
}

// Pending lists open invites and deferred imports across all wallets.
func Pending(ctx context.Context) ([]PendingItem, error) {
	network, err := networkSeam(ctx)
	if err != nil {
		return nil, err
	}
	m := manager(network)
	m.OpenExisting()
	if _, err := m.StoreFor(activeWalletSeam()); err != nil {
		return nil, err
	}
	active := activeWalletSeam()
	var items []PendingItem
	for _, s := range m.Stores() {
		for _, rec := range s.Wallets() {
			var kind string
			switch rec.Status {
			case StatusInvited:
				kind = "invite"
			case StatusPendingImport:
				kind = "resume"
			default:
				continue
			}
			item := PendingItem{
				WalletName:  s.WalletName(),
				TempID:      rec.TempID,
				Label:       rec.Label,
				M:           rec.M,
				N:           rec.N,
				Status:      rec.Status,
				NeedsSwitch: s.WalletName() != active,
				Kind:        kind,
			}
			if len(rec.Peers) > 0 {
				item.InitiatorNick = rec.Peers[0].Nick
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].TempID < items[j].TempID })
	return items, nil
}
