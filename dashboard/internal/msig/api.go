// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

// BackupCard bundles everything a restore needs: the xpub roster, the
// cursors and this wallet's own account coordinates. The seed alone
// recovers nothing (the roster lives only here), and the xpubs recover
// nothing without the exact derivation procedure, so the card names its
// scheme and a restore refuses schemes it does not implement — deriving
// any other way would rebuild the wrong wallet. Proposals and journals
// are device-local and deliberately excluded.
type BackupCard struct {
	CardVersion      int           `json:"cardVersion"`
	DerivationScheme string        `json:"derivationScheme"`
	WalletName       string        `json:"walletName"`
	ExportedAt       int64         `json:"exportedAt"`
	Record           *WalletRecord `json:"record"`
}

const backupCardVersion = 2

// ErrRestoreNeedsPassphrase asks the caller to re-run the restore with
// the wallet passphrase: the card's dedicated account does not exist yet
// and recreating it needs one.
var ErrRestoreNeedsPassphrase = fmt.Errorf("the wallet passphrase is needed to recreate this shared wallet's account")

// assumedAccountGapLimit mirrors dcrwallet's DefaultAccountGapLimit: the
// wallet refuses a new account while this many trailing accounts have no
// transaction history. The value is config-only upstream and cannot be
// queried over RPC, so the stock default is assumed.
const assumedAccountGapLimit = 10

// ErrRestoreAccountGap reports a restore the wallet is guaranteed to
// refuse: reaching the card's account would require creating more
// consecutive unused accounts than the wallet allows.
var ErrRestoreAccountGap = fmt.Errorf("the wallet refuses to create that many unused accounts in a row")

// ExportBackupCard builds the backup card for one shared wallet.
func ExportBackupCard(ctx context.Context, id string) (*BackupCard, error) {
	rec, walletName, _, err := Detail(ctx, id)
	if err != nil {
		return nil, err
	}
	if !rec.HD || rec.Address == "" || len(rec.Xpubs) == 0 || rec.OwnHD == nil {
		return nil, fmt.Errorf("shared wallet %q has not activated yet; nothing to back up", rec.Label)
	}
	return &BackupCard{
		CardVersion:      backupCardVersion,
		DerivationScheme: DerivationScheme,
		WalletName:       walletName,
		ExportedAt:       time.Now().Unix(),
		Record:           rec,
	}, nil
}

// ImportBackupCard restores a shared wallet from its backup card into
// the active wallet. Ownership is proven by extended-key equality: the
// same seed derives the same account xpub, so the card's own xpub must
// match an account of this wallet — located by scanning every account,
// which also survives renames and renumbering. Only when no account
// matches are accounts created sequentially up to the card's number,
// which needs the wallet passphrase and is bounded by the wallet's
// unused-account gap limit. The ladder windows are re-imported
// and one deferred rescan bounded by the creation height recovers the
// history.
func ImportBackupCard(ctx context.Context, card *BackupCard, passphrase []byte) (*WalletRecord, error) {
	defer zero(passphrase)
	if card == nil || card.Record == nil {
		return nil, fmt.Errorf("the backup file is empty")
	}
	if card.CardVersion > backupCardVersion {
		return nil, fmt.Errorf("this backup was made by a newer dcrpulse; upgrade to restore it")
	}
	if card.CardVersion < backupCardVersion {
		return nil, fmt.Errorf("this backup predates HD shared wallets and cannot be restored by this build")
	}
	if card.DerivationScheme != DerivationScheme {
		return nil, fmt.Errorf("this backup uses derivation scheme %q; this build implements %q and restoring would rebuild the wrong addresses",
			card.DerivationScheme, DerivationScheme)
	}
	rec := card.Record
	if !rec.HD || rec.Address == "" || len(rec.Xpubs) == 0 || rec.OwnHD == nil {
		return nil, fmt.Errorf("the backup file is missing the roster or the account key")
	}
	network, err := networkSeam(ctx)
	if err != nil {
		return nil, err
	}
	if rec.Network != network {
		return nil, fmt.Errorf("this backup is for %s; this wallet runs on %s", rec.Network, network)
	}
	params, err := paramsForNetwork(network)
	if err != nil {
		return nil, err
	}
	if err := ValidateXpubRoster(rec.Xpubs, rec.N); err != nil {
		return nil, fmt.Errorf("the backup roster is malformed: %v", err)
	}
	ownIn := false
	for _, x := range rec.Xpubs {
		if x == rec.OwnHD.Xpub {
			ownIn = true
		}
	}
	if !ownIn {
		return nil, fmt.Errorf("the backup key is not part of the shared wallet's roster")
	}
	// Recompute the wallet id rather than trusting the card.
	derived, err := WalletIDForRoster(rec.M, rec.Xpubs, params)
	if err != nil {
		return nil, err
	}
	if derived != rec.Address {
		return nil, fmt.Errorf("the backup roster does not derive its recorded address")
	}
	store, err := manager(network).StoreFor(activeWalletSeam())
	if err != nil {
		return nil, err
	}
	if _, exists := store.Wallet(rec.Address); exists {
		return nil, fmt.Errorf("this shared wallet is already restored here")
	}
	account, err := locateAccountByXpub(ctx, rec.OwnHD.Xpub)
	if err != nil {
		// Preflight before demanding a passphrase: every account created on
		// the way up is unused, so a target more than the gap limit past the
		// wallet's last account is guaranteed to be refused.
		highest, herr := highestNormalAccount(ctx)
		if herr != nil {
			return nil, herr
		}
		if rec.OwnHD.Account > highest+assumedAccountGapLimit {
			return nil, fmt.Errorf("%w: the backup needs account %d but this wallet's accounts end at %d; recover the seed's used accounts first (a full seed restore with account discovery), then retry",
				ErrRestoreAccountGap, rec.OwnHD.Account, highest)
		}
		if len(passphrase) == 0 {
			return nil, ErrRestoreNeedsPassphrase
		}
		account, err = recreateAccountTo(ctx, rec.OwnHD, passphrase)
		if err != nil {
			return nil, err
		}
	}

	restored := cloneRecord(rec)
	restored.Proposals = nil
	restored.Status = StatusActive
	restored.FailReason = ""
	restored.OwnHD.Account = account
	// Import everything from scratch; the card's cursors only widen the
	// window, they are not trusted as import state.
	if restored.Ext != nil {
		restored.Ext.ImportedThrough = 0
	}
	if restored.Int != nil {
		restored.Int.ImportedThrough = 0
	}
	if err := store.PutWallet(restored); err != nil {
		return nil, err
	}
	if err := ensureWindowImported(ctx, store, restored.TempID); err != nil {
		return nil, fmt.Errorf("the wallet could not import the ladder: %v", err)
	}
	scanFrom := restored.CreatedHeight - 1
	if scanFrom < 0 {
		scanFrom = 0
	}
	rescanSeam(scanFrom)
	out, _ := store.Wallet(restored.TempID)
	return out, nil
}

// locateAccountByXpub scans every account for the one whose extended
// public key matches; renames and different numbering do not matter.
func locateAccountByXpub(ctx context.Context, xpub string) (uint32, error) {
	accounts, err := accountsSeam(ctx)
	if err != nil {
		return 0, err
	}
	for _, a := range accounts {
		got, err := accountXpubSeam(ctx, a.AccountNumber)
		if err != nil {
			continue
		}
		if got == xpub {
			return a.AccountNumber, nil
		}
	}
	return 0, fmt.Errorf("no account of this wallet holds the backup's key")
}

// highestNormalAccount reports the wallet's highest BIP44 account
// number, ignoring the imported and xpub-imported ranges.
func highestNormalAccount(ctx context.Context) (uint32, error) {
	accounts, err := accountsSeam(ctx)
	if err != nil {
		return 0, err
	}
	highest := uint32(0)
	for _, a := range accounts {
		if a.AccountNumber < 1<<31 && a.AccountNumber > highest {
			highest = a.AccountNumber
		}
	}
	return highest, nil
}

// recreateAccountTo creates accounts sequentially until the card's
// account number exists, then proves the seed by xpub equality.
func recreateAccountTo(ctx context.Context, own *OwnHDKey, passphrase []byte) (uint32, error) {
	for i := 0; i < int(own.Account)+1; i++ {
		highest, err := highestNormalAccount(ctx)
		if err != nil {
			return 0, err
		}
		if highest >= own.Account {
			break
		}
		name := fmt.Sprintf("shared-restored-%d", highest+1)
		if _, err := createAccountSeam(ctx, name, passphrase); err != nil {
			if strings.Contains(err.Error(), "no transaction history") {
				return 0, fmt.Errorf("%w: recreate account %d: %v; recover the seed's used accounts first, then retry",
					ErrRestoreAccountGap, highest+1, err)
			}
			return 0, fmt.Errorf("recreate account %d: %v", highest+1, err)
		}
	}
	got, err := accountXpubSeam(ctx, own.Account)
	if err != nil {
		return 0, err
	}
	if got != own.Xpub {
		return 0, fmt.Errorf("this wallet's seed does not produce the backup's key; restore it into the wallet whose seed created it")
	}
	return own.Account, nil
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

	// ProposedPeers is who the initiator says the other cosigners are, so
	// an invitee sees the membership before it accepts. Initiator-asserted
	// and unverified: the UI must present it as a claim, not a fact.
	ProposedPeers []RosterPeer `json:"proposedPeers,omitempty"`
}

// SharedAccounts reports the dedicated account numbers of every shared
// wallet in the active wallet's registry, so account listings can keep
// those key reservoirs out of spend selectors. Every record counts,
// terminal ones included: the account never stops being a reservoir.
// Best-effort by design; any failure reports no accounts rather than
// breaking the listing.
func SharedAccounts(ctx context.Context) map[uint32]bool {
	out := make(map[uint32]bool)
	network, err := networkSeam(ctx)
	if err != nil {
		return out
	}
	s, err := manager(network).StoreFor(activeWalletSeam())
	if err != nil {
		return out
	}
	for _, rec := range s.Wallets() {
		if rec.HD && rec.OwnHD != nil {
			out[rec.OwnHD.Account] = true
		}
	}
	return out
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
			case StatusReviewing:
				// Every key is in; the initiator has to look at the
				// cosigners and sign off before anything is announced.
				kind = "activate"
			case StatusConfirming:
				// The roster verified; the cosigner has to confirm who
				// holds the keys before the wallet can receive funds.
				kind = "confirm"
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
			if kind == "invite" {
				item.ProposedPeers = rec.ProposedPeers
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].TempID < items[j].TempID })
	return items, nil
}
