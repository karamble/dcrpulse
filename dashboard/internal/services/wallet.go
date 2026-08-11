// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
)

func FetchWalletStatus() (*types.WalletStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// getinfo also serves as a "wallet is loaded" probe.
	walletInfo, err := rpc.WalletClient.GetInfo(ctx)
	if err != nil {
		return &types.WalletStatus{
			Status:      "no_wallet",
			SyncMessage: fmt.Sprintf("Wallet not available: %v", err),
		}, nil
	}

	snap := GetSyncSnapshot()

	unlocked := true
	daemonConnected := snap.DaemonConnected
	if rpc.WalletClient != nil {
		if raw, werr := rpc.WalletClient.RawRequest(ctx, "walletinfo", nil); werr == nil {
			var wi struct {
				Unlocked        bool `json:"unlocked"`
				DaemonConnected bool `json:"daemonconnected"`
			}
			if jerr := json.Unmarshal(raw, &wi); jerr == nil {
				unlocked = wi.Unlocked
				if !snap.DaemonConnected {
					daemonConnected = wi.DaemonConnected
				}
			}
		}
	}

	bestBlockHash := ""
	var syncHeight int64
	if bestHash, bestHeight, berr := rpc.WalletClient.GetBestBlock(ctx); berr == nil {
		syncHeight = bestHeight
		bestBlockHash = bestHash.String()
	}

	status := "synced"
	syncProgress := 100.0
	syncMessage := "Fully synced"
	rescanInProgress := false

	switch {
	case !daemonConnected:
		status = "disconnected"
		syncMessage = "Disconnected from dcrd"
		syncProgress = 0
	case snap.Phase == SyncPhaseRescanning:
		status = "syncing"
		rescanInProgress = true
		syncProgress = snap.RescanProgressPc
		syncMessage = fmt.Sprintf("Rescanning... %d/%d blocks (%.1f%%)", snap.RescanThrough, snap.RescanFrom, snap.RescanProgressPc)
	case snap.Phase == SyncPhaseFetchingCfilters:
		status = "syncing"
		if snap.CfiltersEnd > snap.CfiltersStart {
			syncMessage = fmt.Sprintf("Fetching committed filters (block %d → %d)", snap.CfiltersStart, snap.CfiltersEnd)
		} else {
			syncMessage = "Fetching committed filters"
		}
		syncProgress = 0
	case snap.Phase == SyncPhaseFetchingHeaders:
		status = "syncing"
		syncMessage = fmt.Sprintf("Fetching headers (%d so far)", snap.HeadersCount)
		if rpc.DcrdClient != nil {
			if chainHeight, cherr := rpc.DcrdClient.GetBlockCount(ctx); cherr == nil && chainHeight > 0 {
				syncProgress = float64(snap.HeadersCount) / float64(chainHeight) * 100
				if syncProgress > 100 {
					syncProgress = 100
				}
			}
		}
	case snap.Phase == SyncPhaseDiscoverAddresses:
		status = "syncing"
		syncMessage = "Discovering addresses"
		syncProgress = 0
	case snap.Phase == SyncPhaseUnsynced || snap.Phase == SyncPhaseUnknown:
		status = "syncing"
		syncMessage = "Sync starting"
		syncProgress = 0
	}

	major := walletInfo.Version / 1000000
	minor := (walletInfo.Version / 10000) % 100
	patch := (walletInfo.Version / 100) % 100

	return &types.WalletStatus{
		Status:           status,
		SyncProgress:     syncProgress,
		SyncHeight:       syncHeight,
		BestBlockHash:    bestBlockHash,
		Version:          fmt.Sprintf("v%d.%d.%d", major, minor, patch),
		Unlocked:         unlocked,
		DaemonConnected:  daemonConnected,
		RescanInProgress: rescanInProgress,
		SyncMessage:      syncMessage,
		IsWatchOnly:      ActiveWalletIsWatchOnly(ctx),
	}, nil
}

// ActiveWalletIsWatchOnly reports whether the currently-loaded wallet is
// watching-only (no private keys, cannot spend). The flag is cached per wallet
// from dcrwallet's authoritative OpenWalletResponse (see cacheWatchOnly).
// Returns false on any error - callers guarding spend operations get a
// conservative "assume spendable" default, and dcrwallet itself still rejects
// signing on a watch-only wallet.
func ActiveWalletIsWatchOnly(ctx context.Context) bool {
	name := ActiveWalletName()
	if name == "" {
		return false
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return false
	}
	cfg, err := config.LoadWalletCfg(network, name)
	if err != nil {
		return false
	}
	var isWatchOnly bool
	_, _ = cfg.Get(config.KeyIsWatchOnly, &isWatchOnly)
	return isWatchOnly
}

// importedXpubAccountBase is the lowest dcrwallet account number assigned to an
// imported xpub account; normal BIP44 accounts are below it.
const importedXpubAccountBase = uint32(1) << 31

// loadXpubAccountIndexes returns the active wallet's map of imported xpub account
// number (stringified) -> real BIP44 account index. Missing/unreadable config
// yields an empty map.
func loadXpubAccountIndexes(ctx context.Context) map[string]uint32 {
	m := map[string]uint32{}
	name := ActiveWalletName()
	network, err := CurrentNetwork(ctx)
	if name == "" || err != nil {
		return m
	}
	cfg, err := config.LoadWalletCfg(network, name)
	if err != nil {
		return m
	}
	_, _ = cfg.Get(config.KeyXpubAccountIndexes, &m)
	if m == nil {
		m = map[string]uint32{}
	}
	return m
}

// SetXpubAccountIndex records the real BIP44 account index for an imported xpub
// account, so offline signing can derive against the correct account on the device.
func SetXpubAccountIndex(ctx context.Context, acctNum, bip44Index uint32) error {
	name := ActiveWalletName()
	network, err := CurrentNetwork(ctx)
	if name == "" || err != nil {
		return fmt.Errorf("wallet config not available")
	}
	cfg, err := config.LoadWalletCfg(network, name)
	if err != nil {
		return err
	}
	m := map[string]uint32{}
	_, _ = cfg.Get(config.KeyXpubAccountIndexes, &m)
	if m == nil {
		m = map[string]uint32{}
	}
	m[fmt.Sprintf("%d", acctNum)] = bip44Index
	if err := cfg.Set(config.KeyXpubAccountIndexes, m); err != nil {
		return err
	}
	return cfg.Save()
}

// Bip44AccountIndex maps a dcrwallet account number to its real BIP44 account
// index. The recorded mapping wins for ANY account number: a watch-only wallet
// created from a device account-export may bind a non-zero device account to
// the wallet's default account 0. Unmapped normal accounts (< 2^31) are their
// own index. An unmapped imported xpub account (>= 2^31) is an error so
// offline signing never derives against the wrong account on the device.
func Bip44AccountIndex(ctx context.Context, acctNum uint32) (uint32, error) {
	if idx, ok := loadXpubAccountIndexes(ctx)[fmt.Sprintf("%d", acctNum)]; ok {
		return idx, nil
	}
	if acctNum < importedXpubAccountBase {
		return acctNum, nil
	}
	return 0, fmt.Errorf("unknown BIP44 account index for imported account %d; re-import its xpub specifying the account index", acctNum)
}

// Bip44IndexInUse reports whether some account already maps the given BIP44
// index, returning the wallet account number that holds it. Two accounts
// mapping the same device index would make offline signing ambiguous.
func Bip44IndexInUse(ctx context.Context, index uint32) (string, bool) {
	for acct, idx := range loadXpubAccountIndexes(ctx) {
		if idx == index {
			return acct, true
		}
	}
	return "", false
}

func FetchWalletDashboardDataWithContext(ctx context.Context) (*types.WalletDashboardData, error) {
	walletStatus, err := FetchWalletStatus()
	if err != nil {
		return nil, err
	}

	accountInfo := &types.AccountInfo{}
	accounts := []types.AccountInfo{}
	var stakingInfo *types.WalletStakingInfo

	// Fetch data with timeout protection - use channels to respect context
	type accountResult struct {
		data *types.AccountInfo
		err  error
	}
	type accountsResult struct {
		data []types.AccountInfo
		err  error
	}
	type stakingResult struct {
		data *types.WalletStakingInfo
		err  error
	}

	accountChan := make(chan accountResult, 1)
	accountsChan := make(chan accountsResult, 1)
	stakingChan := make(chan stakingResult, 1)

	bal := &walletBalances{}

	go func() {
		info, err := fetchAccountInfo(ctx, bal)
		accountChan <- accountResult{info, err}
	}()

	go func() {
		accts, err := fetchAllAccounts(ctx, bal)
		accountsChan <- accountsResult{accts, err}
	}()

	go func() {
		staking, err := FetchWalletStakingInfo(ctx)
		stakingChan <- stakingResult{staking, err}
	}()

	select {
	case res := <-accountChan:
		if res.err != nil {
			wlltLog.Warnf("Failed to fetch account info: %v", res.err)
		} else {
			accountInfo = res.data
		}
	case <-ctx.Done():
		wlltLog.Warnf("Account info fetch cancelled: %v", ctx.Err())
	}

	select {
	case res := <-accountsChan:
		if res.err != nil {
			wlltLog.Warnf("Failed to fetch accounts: %v", res.err)
		} else {
			accounts = res.data
		}
	case <-ctx.Done():
		wlltLog.Warnf("Accounts fetch cancelled: %v", ctx.Err())
	}

	select {
	case res := <-stakingChan:
		if res.err != nil {
			wlltLog.Warnf("Failed to fetch staking info: %v", res.err)
			// Staking info is optional - continue without it
		} else {
			stakingInfo = res.data
		}
	case <-ctx.Done():
		wlltLog.Warnf("Staking info fetch cancelled: %v", ctx.Err())
	}

	return &types.WalletDashboardData{
		WalletStatus: *walletStatus,
		AccountInfo:  *accountInfo,
		Accounts:     accounts,
		StakingInfo:  stakingInfo,
		LastUpdate:   time.Now(),
	}, nil
}

// getbalance returns one point-in-time view of the whole wallet:
//
//	{"balances":[{account info},...], "blockhash":"...",
//	 "totallockedbytickets": X, "totalspendable": Y, "cumulativetotal": Z}
type accountBalance struct {
	AccountName             string  `json:"accountname"`
	ImmatureCoinbaseRewards float64 `json:"immaturecoinbaserewards"`
	ImmatureStakeGeneration float64 `json:"immaturestakegeneration"`
	LockedByTickets         float64 `json:"lockedbytickets"`
	Spendable               float64 `json:"spendable"`
	Total                   float64 `json:"total"`
	Unconfirmed             float64 `json:"unconfirmed"`
	VotingAuthority         float64 `json:"votingauthority"`
}

type balanceResponse struct {
	Balances             []accountBalance `json:"balances"`
	BlockHash            string           `json:"blockhash"`
	TotalLockedByTickets float64          `json:"totallockedbytickets"`
	TotalSpendable       float64          `json:"totalspendable"`
	CumulativeTotal      float64          `json:"cumulativetotal"`
}

// walletBalances fetches getbalance once, so the wallet-wide totals and the
// per-account list are read from the same block.
type walletBalances struct {
	once sync.Once
	resp balanceResponse
	err  error
}

func (w *walletBalances) get(ctx context.Context) (balanceResponse, error) {
	w.once.Do(func() {
		result, rerr := rpc.WalletClient.RawRequest(ctx, "getbalance", []json.RawMessage{})
		if rerr != nil {
			w.err = rerr
			return
		}
		if uerr := json.Unmarshal(result, &w.resp); uerr != nil {
			w.err = fmt.Errorf("unmarshal balance response: %w", uerr)
		}
	})
	return w.resp, w.err
}

func fetchAccountInfo(ctx context.Context, bal *walletBalances) (*types.AccountInfo, error) {
	balanceResp, err := bal.get(ctx)
	if err != nil {
		wlltLog.Warnf("Failed to get balance: %v", err)
		return &types.AccountInfo{
			AccountName:        "Total",
			TotalBalance:       0,
			SpendableBalance:   0,
			ImmatureBalance:    0,
			UnconfirmedBalance: 0,
			LockedByTickets:    0,
			AccountNumber:      0,
		}, nil
	}

	// Sum immature and unconfirmed balances across all accounts
	immature := 0.0
	unconfirmed := 0.0
	lockedByTickets := 0.0
	votingAuthority := 0.0

	for _, acct := range balanceResp.Balances {
		immature += acct.ImmatureCoinbaseRewards + acct.ImmatureStakeGeneration
		unconfirmed += acct.Unconfirmed
		lockedByTickets += acct.LockedByTickets
		votingAuthority += acct.VotingAuthority
	}

	// Return wallet-wide totals with granular breakdown
	return &types.AccountInfo{
		AccountName:        "Total",
		TotalBalance:       balanceResp.CumulativeTotal,
		SpendableBalance:   balanceResp.TotalSpendable,
		ImmatureBalance:    immature,
		UnconfirmedBalance: unconfirmed,
		LockedByTickets:    balanceResp.TotalLockedByTickets,
		VotingAuthority:    votingAuthority,
		AccountNumber:      0,
		// Wallet-wide totals
		CumulativeTotal:      balanceResp.CumulativeTotal,
		TotalSpendable:       balanceResp.TotalSpendable,
		TotalLockedByTickets: balanceResp.TotalLockedByTickets,
	}, nil
}

func FetchAllAccounts(ctx context.Context) ([]types.AccountInfo, error) {
	return fetchAllAccounts(ctx, &walletBalances{})
}

func fetchAllAccounts(ctx context.Context, bal *walletBalances) ([]types.AccountInfo, error) {
	balanceResp, err := bal.get(ctx)
	if err != nil {
		wlltLog.Warnf("Failed to get accounts: %v", err)
		return []types.AccountInfo{}, nil
	}

	// getbalance does not return account numbers or per-account encryption
	// state; resolve them via gRPC Accounts.
	numbers := map[string]uint32{}
	encrypted := map[string]bool{}
	unlocked := map[string]bool{}
	if rpc.WalletGrpcClient != nil {
		if acctsResp, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{}); err != nil {
			wlltLog.Warnf("GRPC Accounts call failed, account numbers will be 0: %v", err)
		} else {
			for _, a := range acctsResp.Accounts {
				numbers[a.AccountName] = a.AccountNumber
				encrypted[a.AccountName] = a.AccountEncrypted
				unlocked[a.AccountName] = a.AccountUnlocked
			}
		}
	}

	xpubIndexes := loadXpubAccountIndexes(ctx)

	accounts := make([]types.AccountInfo, 0, len(balanceResp.Balances))
	for _, acct := range balanceResp.Balances {
		num := numbers[acct.AccountName]
		var bip44Index *uint32
		if num >= importedXpubAccountBase {
			if idx, ok := xpubIndexes[fmt.Sprintf("%d", num)]; ok {
				idx := idx
				bip44Index = &idx
			}
		}
		accounts = append(accounts, types.AccountInfo{
			AccountName:             acct.AccountName,
			TotalBalance:            acct.Total,
			SpendableBalance:        acct.Spendable,
			ImmatureBalance:         acct.ImmatureCoinbaseRewards + acct.ImmatureStakeGeneration,
			UnconfirmedBalance:      acct.Unconfirmed,
			LockedByTickets:         acct.LockedByTickets,
			VotingAuthority:         acct.VotingAuthority,
			ImmatureCoinbaseRewards: acct.ImmatureCoinbaseRewards,
			ImmatureStakeGeneration: acct.ImmatureStakeGeneration,
			AccountNumber:           num,
			AccountEncrypted:        encrypted[acct.AccountName],
			AccountUnlocked:         unlocked[acct.AccountName],
			Reserved:                IsReservedAccountName(acct.AccountName),
			Bip44Index:              bip44Index,
		})
	}

	return accounts, nil
}

// CreateAccount creates a new BIP44 account via gRPC NextAccount and then
// per-account-encrypts it with the same passphrase so signing can go through
// UnlockAccount. Returns the new account number.
func CreateAccount(ctx context.Context, accountName string, passphrase []byte) (uint32, error) {
	if rpc.WalletGrpcClient == nil {
		return 0, fmt.Errorf("wallet gRPC unavailable")
	}
	resp, err := rpc.WalletGrpcClient.NextAccount(ctx, &pb.NextAccountRequest{
		Passphrase:  passphrase,
		AccountName: accountName,
	})
	if err != nil {
		return 0, err
	}
	if _, err := rpc.WalletGrpcClient.SetAccountPassphrase(ctx, &pb.SetAccountPassphraseRequest{
		AccountNumber:        resp.AccountNumber,
		NewAccountPassphrase: passphrase,
		WalletPassphrase:     passphrase,
	}); err != nil {
		return 0, fmt.Errorf("account created but failed to set per-account passphrase: %w", err)
	}
	return resp.AccountNumber, nil
}

// ensureAccountEncrypted lazily migrates an account to per-account encryption
// if it isn't already (e.g. the default account on a freshly-created wallet).
// Mirrors Decrediton's one-time setAccountsPass migration. Safe to call on
// accounts that are already per-account-encrypted — it's a no-op then.
func ensureAccountEncrypted(ctx context.Context, accountNumber uint32, passphrase []byte) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC unavailable")
	}
	acctsResp, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{})
	if err != nil {
		return err
	}
	for _, a := range acctsResp.Accounts {
		if a.AccountNumber == accountNumber {
			if a.AccountEncrypted {
				return nil
			}
			_, err := rpc.WalletGrpcClient.SetAccountPassphrase(ctx, &pb.SetAccountPassphraseRequest{
				AccountNumber:        accountNumber,
				NewAccountPassphrase: passphrase,
				WalletPassphrase:     passphrase,
			})
			return err
		}
	}
	return fmt.Errorf("account %d not found", accountNumber)
}

// ensureAllAccountsEncrypted gives every normal account the same per-account
// passphrase (equal to the wallet passphrase) in one pass, so all accounts unlock
// uniformly via UnlockAccount. Mirrors Decrediton's setAccountsPass migration.
// Run after wallet creation and after a restore's account discovery so the default
// account never diverges from the accounts recovered or created later. Skips the
// same accounts unlockAllAccountsForSpend does: imported, dex (bisonw-managed), and
// xpub-imported (>= 2^31). Each account is delegated to ensureAccountEncrypted,
// which is a no-op for accounts already per-account-encrypted.
func ensureAllAccountsEncrypted(ctx context.Context, passphrase []byte) error {
	accounts, err := FetchAllAccounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if a.AccountName == "imported" || a.AccountName == "dex" || a.AccountNumber >= 1<<31 {
			continue
		}
		if err := ensureAccountEncrypted(ctx, a.AccountNumber, passphrase); err != nil {
			return fmt.Errorf("encrypt account %q (%d): %w", a.AccountName, a.AccountNumber, err)
		}
	}
	return nil
}

// unlockAccountForSpend makes a per-account-encrypted account usable for signing
// and verifies the passphrase in the process. The unlock is the only check
// standing behind a spend: SignTransaction is called without a passphrase, so it
// signs with whatever keys are unlocked. dcrwallet compares the passphrase
// against the one that encrypted the account even when it is already unlocked,
// and every account this app encrypts carries the wallet passphrase, so
// re-unlocking an open account succeeds. Reports whether this call transitioned
// the account from locked to unlocked, so a caller only re-locks what it opened.
// Lazily migrates to per-account encryption if needed (the default account on a
// fresh wallet isn't encrypted yet).
func unlockAccountForSpend(ctx context.Context, accountNumber uint32, passphrase []byte) (bool, error) {
	if rpc.WalletGrpcClient == nil {
		return false, fmt.Errorf("wallet gRPC client not initialized")
	}
	wasUnlocked := false
	if acctsResp, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{}); err == nil {
		for _, a := range acctsResp.Accounts {
			if a.AccountNumber == accountNumber {
				wasUnlocked = a.AccountUnlocked
				break
			}
		}
	}

	if _, err := rpc.WalletGrpcClient.UnlockAccount(ctx, &pb.UnlockAccountRequest{
		Passphrase:    passphrase,
		AccountNumber: accountNumber,
	}); err != nil {
		if strings.Contains(err.Error(), "account is not encrypted with a unique passphrase") {
			if mErr := ensureAccountEncrypted(ctx, accountNumber, passphrase); mErr != nil {
				return false, fmt.Errorf("migrate account to per-account encryption: %w", mErr)
			}
			if _, err := rpc.WalletGrpcClient.UnlockAccount(ctx, &pb.UnlockAccountRequest{
				Passphrase:    passphrase,
				AccountNumber: accountNumber,
			}); err != nil {
				return false, fmt.Errorf("unlock source account: %w", err)
			}
			return !wasUnlocked, nil
		}
		return false, fmt.Errorf("unlock source account: %w", err)
	}
	return !wasUnlocked, nil
}

// unlockAllAccountsForSpend unlocks every normal (non-imported, non-watch-only)
// account and returns the account numbers it actually transitioned from locked
// to unlocked. VSP fee reconciliation signs with each ticket's commitment-
// address key, which can belong to any account (e.g. tickets bought from the
// mixed account), so unlocking only the fee account leaves that signing key
// locked. Pass the returned slice to relockAccountsAfterVSP to re-lock them
// once processing is done. Mirrors Decrediton's unlockAllAcctAndExecFn.
func unlockAllAccountsForSpend(ctx context.Context, passphrase []byte) ([]uint32, error) {
	accounts, err := FetchAllAccounts(ctx)
	if err != nil {
		return nil, err
	}
	var candidates, succeeded int
	var newlyUnlocked []uint32
	for _, a := range accounts {
		// The imported (2^31-1) and xpub-imported (>=2^31) accounts hold no
		// per-account passphrase key and cannot be unlocked this way. The dex
		// account is managed by the DCRDEX backend (bisonw), which may encrypt
		// it with its own passphrase; it never holds VSP tickets, so skip it.
		if a.AccountName == "imported" || a.AccountName == "dex" || a.AccountNumber >= 1<<31 {
			continue
		}
		candidates++
		// Every candidate is unlocked, including one that is already open, so
		// the passphrase is checked against all of them: crediting an
		// already-open account would let a wrong passphrase through here.
		didUnlock, err := unlockAccountForSpend(ctx, a.AccountNumber, passphrase)
		if err != nil {
			// An account may carry a divergent per-account passphrase; skip it
			// rather than abort, so the accounts that do unlock (including each
			// ticket's commitment account) can still sign for the VSP. A
			// genuinely wrong passphrase fails every account, handled below.
			wlltLog.Warnf("unlockAllAccountsForSpend: skipping account %q (%d): %v", a.AccountName, a.AccountNumber, err)
			continue
		}
		succeeded++
		// Only accounts this call opened are re-locked afterwards, leaving one
		// something else needs (a running mixer, say) untouched.
		if didUnlock {
			newlyUnlocked = append(newlyUnlocked, a.AccountNumber)
		}
	}
	if candidates > 0 && succeeded == 0 {
		return nil, fmt.Errorf("invalid passphrase")
	}
	return newlyUnlocked, nil
}

// vspTicketCommitAccounts returns the set of account numbers that own the
// commitment addresses of the wallet's currently tracked VSP tickets. The
// dcrwallet VSP client reconciles those tickets' fees in a background timer
// and must keep these accounts' signing keys unlocked. Mirrors Decrediton's
// getVSPTrackedTicketsCommitAccounts.
func vspTicketCommitAccounts(ctx context.Context) map[uint32]bool {
	out := map[uint32]bool{}
	if rpc.WalletGrpcClient == nil {
		return out
	}
	resp, err := rpc.WalletGrpcClient.GetTrackedVSPTickets(ctx, &pb.GetTrackedVSPTicketsRequest{})
	if err != nil {
		wlltLog.Warnf("vspTicketCommitAccounts: GetTrackedVSPTickets: %v", err)
		return out
	}
	for _, v := range resp.GetVsps() {
		for _, t := range v.GetTickets() {
			addr := t.GetCommitmentAddress()
			if addr == "" {
				continue
			}
			va, verr := rpc.WalletGrpcClient.ValidateAddress(ctx, &pb.ValidateAddressRequest{Address: addr})
			if verr != nil || !va.GetIsMine() {
				continue
			}
			out[va.GetAccountNumber()] = true
		}
	}
	return out
}

// relockAccountsAfterVSP re-locks the accounts unlockAllAccountsForSpend
// unlocked, skipping those that own a tracked VSP ticket's commitment address
// (the VSP client keeps reconciling their fees in the background, which fires
// after the originating RPC returns). Mirrors Decrediton's relockAccounts /
// filterUnlockableAccounts.
func relockAccountsAfterVSP(unlocked []uint32) {
	if len(unlocked) == 0 || rpc.WalletGrpcClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	keep := vspTicketCommitAccounts(ctx)
	for _, acct := range unlocked {
		if keep[acct] {
			continue
		}
		if _, err := rpc.WalletGrpcClient.LockAccount(ctx, &pb.LockAccountRequest{AccountNumber: acct}); err != nil {
			wlltLog.Errorf("relockAccountsAfterVSP: lock account %d: %v", acct, err)
		}
	}
}

// relockAccount locks an account that was unlocked for a spend. It runs on a
// fresh background context so a cancelled operation context cannot prevent the
// relock. A failed relock leaves the account's signing key usable, so it is
// reported through onErr (when non-nil) rather than dropped.
func relockAccount(accountNumber uint32, onErr func(string)) {
	if rpc.WalletGrpcClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := rpc.WalletGrpcClient.LockAccount(ctx, &pb.LockAccountRequest{AccountNumber: accountNumber}); err != nil && onErr != nil {
		onErr(fmt.Sprintf("Failed to relock account %d: %v", accountNumber, err))
	}
}

// verifyAccountPassphrase checks a passphrase against an account and leaves the
// account's lock state exactly as it found it. Flows that detach into a
// goroutine use this to reject a wrong passphrase while the HTTP caller is still
// listening, instead of reporting it later through an event.
func verifyAccountPassphrase(ctx context.Context, accountNumber uint32, passphrase []byte) error {
	didUnlock, err := unlockAccountForSpend(ctx, accountNumber, passphrase)
	if err != nil {
		return err
	}
	if didUnlock {
		relockAccount(accountNumber, nil)
	}
	return nil
}

// unlockedOps counts the flows that currently depend on an account staying
// unlocked between their unlock and their matching re-lock. The lock monitor
// stands down while any is in flight so it cannot lock an account out from
// under a spend. Mirrors Decrediton's control.unlockAndExecFnRunning.
var unlockedOps atomic.Int32

func beginUnlockedOp() { unlockedOps.Add(1) }

func endUnlockedOp() { unlockedOps.Add(-1) }

// accountLockSweepInterval matches Decrediton's monitorLockableAccounts timer.
const accountLockSweepInterval = 30 * time.Second

// StartAccountLockMonitor re-locks accounts that no longer need to be open.
// Accounts are deliberately left unlocked by the VSP fee reconciler, the mixer
// and the autobuyer, and the latter two only re-lock from deferred calls inside
// their goroutine, which a dashboard restart skips while dcrwallet keeps
// running. Without this sweep such an account stays unlocked until dcrwallet
// itself restarts. Mirrors Decrediton's monitorLockableAccounts.
func StartAccountLockMonitor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(accountLockSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepLockableAccounts(ctx)
			}
		}
	}()
}

// sweepLockableAccounts locks every account dcrwallet reports unlocked that
// nothing still needs.
func sweepLockableAccounts(ctx context.Context) {
	if rpc.WalletGrpcClient == nil {
		return
	}
	// The mixer and autobuyer hold their accounts open for their whole run, and
	// a spend holds its own between the unlock and the re-lock.
	if unlockedOps.Load() > 0 || IsMixerRunning() || IsAutobuyerRunning() {
		return
	}
	sweepCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := rpc.WalletGrpcClient.Accounts(sweepCtx, &pb.AccountsRequest{})
	if err != nil {
		return
	}
	// Resolved on first use so an idle wallet costs no VSP lookup.
	var keep map[uint32]bool
	for _, a := range resp.Accounts {
		// Only individually encrypted accounts can be locked; the dex account
		// is bisonw's to manage. Mirrors Decrediton's unlocked && encrypted
		// filter plus its dex exclusion.
		if !a.AccountUnlocked || !a.AccountEncrypted || a.AccountName == "dex" || a.AccountNumber >= 1<<31 {
			continue
		}
		if keep == nil {
			keep = vspTicketCommitAccounts(sweepCtx)
		}
		if keep[a.AccountNumber] {
			continue
		}
		if _, err := rpc.WalletGrpcClient.LockAccount(sweepCtx, &pb.LockAccountRequest{AccountNumber: a.AccountNumber}); err != nil {
			wlltLog.Errorf("account lock monitor: lock account %d: %v", a.AccountNumber, err)
			continue
		}
		wlltLog.Infof("account lock monitor: locked unused account %d", a.AccountNumber)
	}
}

// RenameAccount renames an existing account. dcrwallet's RenameAccount gRPC
// does not require the passphrase — the account name is metadata, not key
// material.
func RenameAccount(ctx context.Context, accountNumber uint32, newName string) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC unavailable")
	}
	_, err := rpc.WalletGrpcClient.RenameAccount(ctx, &pb.RenameAccountRequest{
		AccountNumber: accountNumber,
		NewName:       newName,
	})
	return err
}

// GetAccountExtendedPubKey returns the BIP32 extended public key for the given
// account. Used for watch-only export. No passphrase needed — it's a public key.
func GetAccountExtendedPubKey(ctx context.Context, accountNumber uint32) (string, error) {
	if rpc.WalletGrpcClient == nil {
		return "", fmt.Errorf("wallet gRPC unavailable")
	}
	resp, err := rpc.WalletGrpcClient.GetAccountExtendedPubKey(ctx, &pb.GetAccountExtendedPubKeyRequest{
		AccountNumber: accountNumber,
	})
	if err != nil {
		return "", err
	}
	return resp.AccExtendedPubKey, nil
}

// Names of the two accounts the mixer uses; Decrediton convention.
const (
	PrivacyMixedAccountName  = "mixed"
	PrivacyChangeAccountName = "unmixed"
	// privacyMixedAccountBranch is the BIP44 branch the mixer and mixed ticket
	// purchases use for the mixed account (Decrediton convention; also passed to
	// StartMixer).
	privacyMixedAccountBranch = 0
)

// DexAccountName is the dedicated dcrwallet account DCRDEX trades from. Defined
// here (and referenced by handlers/dcrdex.go) so the reserved-account check has
// a single source of truth.
const DexAccountName = "dex"

// IsReservedAccountName reports whether name is one of the dcrwallet accounts
// other daemons bind to by name - the privacy mixer's mixed/unmixed accounts,
// dcrlnd's lightning account, DCRDEX's dex account - or dcrwallet's imported
// bucket. These must never be renamed: renaming silently breaks the binding
// (dcrlnd account-ID mismatch, DCRDEX "account not found"). Case-insensitive.
func IsReservedAccountName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case PrivacyMixedAccountName, PrivacyChangeAccountName, LightningAccountName, DexAccountName, "imported":
		return true
	default:
		return false
	}
}

// FindPrivacyAccounts looks up the mixer's mixed and unmixed accounts by name.
// `configured` is true only when both exist.
func FindPrivacyAccounts(ctx context.Context) (mixed uint32, change uint32, configured bool, err error) {
	accounts, err := FetchAllAccounts(ctx)
	if err != nil {
		return 0, 0, false, err
	}
	var foundMixed, foundChange bool
	for _, a := range accounts {
		switch a.AccountName {
		case PrivacyMixedAccountName:
			mixed = a.AccountNumber
			foundMixed = true
		case PrivacyChangeAccountName:
			change = a.AccountNumber
			foundChange = true
		}
	}
	return mixed, change, foundMixed && foundChange, nil
}

// TicketMixing holds the accounts a ticket purchase routes through when privacy
// is enabled. Mirrors Decrediton: the mixed account is the funding + split +
// mixed account, the unmixed account receives change.
type TicketMixing struct {
	Mixed  uint32
	Change uint32
}

// TicketMixingParams reports the mixing accounts to use for a ticket purchase or
// auto-buy. ok is true only when both privacy accounts ("mixed"/"unmixed") exist,
// matching Decrediton's "privacy on when a mixed+change account is configured".
// On any lookup error it returns ok=false so purchasing falls back to plain mode.
func TicketMixingParams(ctx context.Context) (TicketMixing, bool) {
	mixed, change, configured, err := FindPrivacyAccounts(ctx)
	if err != nil || !configured {
		return TicketMixing{}, false
	}
	return TicketMixing{Mixed: mixed, Change: change}, true
}

// SetupPrivacyAccounts creates whichever of "mixed" / "unmixed" is missing.
// Idempotent — if both exist, returns their numbers without touching anything.
func SetupPrivacyAccounts(ctx context.Context, passphrase []byte) (mixed uint32, change uint32, err error) {
	accounts, err := FetchAllAccounts(ctx)
	if err != nil {
		return 0, 0, err
	}
	var haveMixed, haveChange bool
	for _, a := range accounts {
		switch a.AccountName {
		case PrivacyMixedAccountName:
			mixed = a.AccountNumber
			haveMixed = true
		case PrivacyChangeAccountName:
			change = a.AccountNumber
			haveChange = true
		}
	}

	if !haveMixed {
		n, cerr := CreateAccount(ctx, PrivacyMixedAccountName, passphrase)
		if cerr != nil {
			return 0, 0, fmt.Errorf("create %q: %w", PrivacyMixedAccountName, cerr)
		}
		mixed = n
	}
	if !haveChange {
		n, cerr := CreateAccount(ctx, PrivacyChangeAccountName, passphrase)
		if cerr != nil {
			return 0, 0, fmt.Errorf("create %q: %w", PrivacyChangeAccountName, cerr)
		}
		change = n
	}

	return mixed, change, nil
}

func FetchWalletStakingInfo(ctx context.Context) (*types.WalletStakingInfo, error) {
	stakingInfo := &types.WalletStakingInfo{}

	// Fetch getstakeinfo
	stakeInfoResult, err := rpc.WalletClient.RawRequest(ctx, "getstakeinfo", []json.RawMessage{})
	if err != nil {
		wlltLog.Warnf("Failed to get stake info: %v", err)
		return nil, err
	}

	type StakeInfoResponse struct {
		BlockHeight    int64   `json:"blockheight"`
		Difficulty     float64 `json:"difficulty"`
		TotalSubsidy   float64 `json:"totalsubsidy"`
		OwnMempoolTix  int32   `json:"ownmempooltix"`
		Immature       int32   `json:"immature"`
		Unspent        int32   `json:"unspent"`
		Voted          int32   `json:"voted"`
		Revoked        int32   `json:"revoked"`
		UnspentExpired int32   `json:"unspentexpired"`
		PoolSize       int32   `json:"poolsize"`
		AllMempoolTix  int32   `json:"allmempooltix"`
	}

	var stakeInfo StakeInfoResponse
	if err := json.Unmarshal(stakeInfoResult, &stakeInfo); err != nil {
		wlltLog.Warnf("Failed to unmarshal stake info: %v", err)
		return nil, err
	}

	stakingInfo.BlockHeight = stakeInfo.BlockHeight
	stakingInfo.Difficulty = stakeInfo.Difficulty
	stakingInfo.TotalSubsidy = stakeInfo.TotalSubsidy
	stakingInfo.OwnMempoolTix = stakeInfo.OwnMempoolTix
	stakingInfo.Immature = stakeInfo.Immature
	stakingInfo.Unspent = stakeInfo.Unspent
	stakingInfo.Voted = stakeInfo.Voted
	stakingInfo.Revoked = stakeInfo.Revoked
	stakingInfo.UnspentExpired = stakeInfo.UnspentExpired
	stakingInfo.PoolSize = stakeInfo.PoolSize
	stakingInfo.AllMempoolTix = stakeInfo.AllMempoolTix

	if estimate, err := rpc.WalletClient.EstimateStakeDiff(ctx, nil); err != nil {
		wlltLog.Warnf("Failed to estimate stake diff: %v", err)
	} else {
		stakingInfo.EstimatedMin = estimate.Min
		stakingInfo.EstimatedMax = estimate.Max
		stakingInfo.EstimatedExpected = estimate.Expected
	}

	if difficulty, err := rpc.WalletClient.GetStakeDifficulty(ctx); err != nil {
		wlltLog.Warnf("Failed to get stake difficulty: %v", err)
	} else {
		stakingInfo.CurrentDifficulty = difficulty.CurrentStakeDifficulty
		stakingInfo.NextDifficulty = difficulty.NextStakeDifficulty
	}

	// Fetch dcrd getblocksubsidy for the next block (current PoS reward).
	var subsidyReductionInterval int64
	if params, perr := chainParams(ctx); perr == nil {
		subsidyReductionInterval = params.SubsidyReductionInterval
	} else {
		wlltLog.Warnf("Failed to get chain params for subsidy interval: %v", perr)
	}
	stakingInfo.SubsidyReductionInterval = subsidyReductionInterval
	if rpc.DcrdClient != nil {
		chainHeight, err := rpc.DcrdClient.GetBlockCount(ctx)
		if err != nil {
			wlltLog.Warnf("Failed to get chain height for block subsidy: %v", err)
		} else {
			nextHeight := chainHeight + 1
			subsidy, err := rpc.DcrdClient.GetBlockSubsidy(ctx, nextHeight, 5)
			if err != nil {
				wlltLog.Warnf("Failed to get block subsidy: %v", err)
			} else {
				stakingInfo.BlockSubsidyHeight = nextHeight
				stakingInfo.BlockSubsidyTotal = dcrutil.Amount(subsidy.Total).ToCoin()
				stakingInfo.BlockSubsidyPoS = dcrutil.Amount(subsidy.PoS).ToCoin()
				stakingInfo.BlockSubsidyPoW = dcrutil.Amount(subsidy.PoW).ToCoin()
				stakingInfo.BlockSubsidyTreasury = dcrutil.Amount(subsidy.Developer).ToCoin()
				if subsidyReductionInterval > 0 {
					stakingInfo.BlocksUntilSubsidyReduction = subsidyReductionInterval - (chainHeight % subsidyReductionInterval)
				}
			}
		}
	}

	return stakingInfo, nil
}

// ListTransactions fetches recent wallet transactions
// listTxEntry is one listtransactions result row.
type listTxEntry struct {
	Account         string   `json:"account"`
	Address         string   `json:"address"`
	Amount          float64  `json:"amount"`
	BlockHash       string   `json:"blockhash"`
	BlockTime       int64    `json:"blocktime"`
	Category        string   `json:"category"`
	Confirmations   int64    `json:"confirmations"`
	Fee             float64  `json:"fee"`
	Generated       bool     `json:"generated"`
	Time            int64    `json:"time"`
	TimeReceived    int64    `json:"timereceived"`
	TxID            string   `json:"txid"`
	TxType          string   `json:"txtype"`
	Vout            uint32   `json:"vout"`
	WalletConflicts []string `json:"walletconflicts"`
}

// walletTxRow builds the list row every branch shares, computing the block
// height and the vote maturity so a split vote entry cannot stick at
// "Voted (Maturing)".
func walletTxRow(rpcTx listTxEntry, isMixed bool, currentHeight, voteMaturity int64) types.Transaction {
	var blockHeight int64
	if currentHeight > 0 && rpcTx.Confirmations > 0 {
		blockHeight = currentHeight - rpcTx.Confirmations + 1
	}
	var isTicketMature bool
	var blocksUntilSpendable int64
	if rpcTx.TxType == "vote" && blockHeight > 0 && currentHeight > 0 {
		blocksPassed := currentHeight - blockHeight
		if blocksPassed >= voteMaturity {
			isTicketMature = true
		} else {
			blocksUntilSpendable = voteMaturity - blocksPassed
		}
	}
	return types.Transaction{
		TxID:                 rpcTx.TxID,
		Amount:               rpcTx.Amount,
		Fee:                  rpcTx.Fee,
		Confirmations:        rpcTx.Confirmations,
		BlockHash:            rpcTx.BlockHash,
		BlockTime:            rpcTx.BlockTime,
		Time:                 time.Unix(rpcTx.Time, 0),
		Category:             rpcTx.Category,
		TxType:               rpcTx.TxType,
		Address:              rpcTx.Address,
		Account:              rpcTx.Account,
		Vout:                 rpcTx.Vout,
		Generated:            rpcTx.Generated,
		IsMixed:              isMixed,
		BlockHeight:          blockHeight,
		IsTicketMature:       isTicketMature,
		BlocksUntilSpendable: blocksUntilSpendable,
	}
}

func ListTransactions(ctx context.Context, count, from int) (*types.TransactionListResponse, error) {
	// Default parameters
	if count <= 0 {
		count = 50 // Default to 50 transactions
	}
	if count > 10000 {
		count = 10000 // Cap at 10000 for performance
	}

	// A vote's returned stake and reward mature over CoinbaseMaturity blocks.
	var voteMaturity int64 = 256
	if params, err := chainParams(ctx); err == nil {
		voteMaturity = int64(params.CoinbaseMaturity)
	}

	// The wallet's own tip. listtransactions confirmations are measured against
	// it, so heights derived from them stay in the same frame even when the
	// wallet lags dcrd.
	currentHeight := int64(0)
	if h, err := WalletTipHeight(ctx); err == nil {
		currentHeight = h
	}

	// Call listtransactions RPC with parameters
	result, err := rpc.WalletClient.RawRequest(ctx, "listtransactions", []json.RawMessage{
		json.RawMessage(`"*"`),                    // account (all accounts)
		json.RawMessage(fmt.Sprintf("%d", count)), // count
		json.RawMessage(fmt.Sprintf("%d", from)),  // from (skip)
		json.RawMessage("false"),                  // includewatchonly
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list transactions: %w", err)
	}

	// Parse the response
	var rpcTransactions []listTxEntry

	if err := json.Unmarshal(result, &rpcTransactions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transactions: %w", err)
	}

	// One wallet stream over the listed range replaces the old lookup per
	// transaction: the raw bytes carry the CoinJoin shape and the stakebase
	// reward, the credits and debits carry the wallet's net position.
	facts, ferr := listTxFactsFor(ctx, listTxMinHeight(rpcTransactions, currentHeight))
	if ferr != nil {
		wlltLog.Warnf("Transaction facts stream unavailable, rows degrade: %v", ferr)
		facts = map[string]listTxFacts{}
	}

	// Group by txid - multiple entries indicate CoinJoin or ticket with change
	type TxGroup struct {
		Entries []int
		TxType  string
	}
	txMap := make(map[string]*TxGroup)

	for i, rpcTx := range rpcTransactions {
		if _, exists := txMap[rpcTx.TxID]; !exists {
			txMap[rpcTx.TxID] = &TxGroup{
				Entries: []int{},
				TxType:  rpcTx.TxType,
			}
		}
		txMap[rpcTx.TxID].Entries = append(txMap[rpcTx.TxID].Entries, i)
	}

	transactions := make([]types.Transaction, 0)
	processed := make(map[string]bool)

	for _, rpcTx := range rpcTransactions {
		if processed[rpcTx.TxID] {
			continue
		}

		group := txMap[rpcTx.TxID]

		// Multi-entry transactions need special handling
		if len(group.Entries) > 1 {
			isMixed := false
			if rpcTx.TxType == "regular" {
				isMixed = facts[rpcTx.TxID].mixed
			}

			var netAmount float64
			var accountName string
			accountsInvolved := make(map[string]bool)

			// Collect account info from all entries
			for _, idx := range group.Entries {
				entry := rpcTransactions[idx]
				if entry.Account != "" {
					accountsInvolved[entry.Account] = true
				}
				if accountName == "" && entry.Account != "" {
					accountName = entry.Account
				}
			}

			// CoinJoin: the wallet's own net position (the listing rows
			// include other participants' outputs).
			if isMixed {
				netAmount = facts[rpcTx.TxID].netDCR
			} else {
				// Non-CoinJoin: sum entries (already wallet-filtered)
				for _, idx := range group.Entries {
					entry := rpcTransactions[idx]
					netAmount += entry.Amount
				}
			}

			if len(accountsInvolved) > 1 {
				accounts := make([]string, 0, len(accountsInvolved))
				for acc := range accountsInvolved {
					accounts = append(accounts, acc)
				}
				wlltLog.Debugf("TX %s involves multiple accounts: %v", rpcTx.TxID[:12], accounts)
			}

			// Determine category from net amount for regular txs. The net
			// is compared against half an atom rather than exact zero:
			// float summation across multi-output transfers can leave
			// sub-atom residue, while any real movement is at least one atom.
			category := rpcTx.Category
			if rpcTx.TxType == "regular" {
				if isMixed {
					category = "coinjoin"
				} else if math.Abs(netAmount) < 5e-9 {
					category = "self"
				} else if netAmount > 0 {
					category = "receive"
				} else {
					category = "send"
				}
			}

			// CoinJoin fee is the cost to participate
			var fee float64 = 0
			if isMixed && netAmount < 0 {
				fee = -netAmount
			}

			// Intra-wallet transfer: every output returns to the wallet, so
			// the wallet's true delta is just the fee, which dcrwallet
			// attaches (negative) to the debit-side entries.
			if category == "self" {
				for _, idx := range group.Entries {
					if f := rpcTransactions[idx].Fee; f != 0 {
						fee = math.Abs(f)
						break
					}
				}
				netAmount = -fee
			}

			// The group collapses to one row: net amount, derived category and
			// fee replace the entry's own, and per-output fields are cleared.
			tx := walletTxRow(rpcTx, isMixed, currentHeight, voteMaturity)
			tx.Amount = netAmount
			tx.Fee = fee
			tx.Category = category
			tx.Address = ""
			tx.Account = accountName
			tx.Vout = 0
			tx.Generated = false
			transactions = append(transactions, tx)
			processed[rpcTx.TxID] = true
			continue
		}

		isMixed := false
		if rpcTx.TxType == "regular" {
			isMixed = facts[rpcTx.TxID].mixed
		}

		tx := walletTxRow(rpcTx, isMixed, currentHeight, voteMaturity)
		transactions = append(transactions, tx)
		processed[rpcTx.TxID] = true
	}

	// A vote's listtransactions net cancels to ~0; show the stakebase reward
	// read directly from the vote transaction instead.
	for i := range transactions {
		if transactions[i].TxType == "vote" {
			if f, ok := facts[transactions[i].TxID]; ok && f.hasReward {
				transactions[i].Amount = f.voteReward
			}
		}
	}

	// Detect VSP fees (paid exactly 6 blocks after ticket)
	for i := range transactions {
		isVSPFee, relatedTicket := isVSPFeeTransaction(transactions[i], transactions)
		if isVSPFee {
			transactions[i].Category = "vspfee"
			transactions[i].IsVSPFee = true
			transactions[i].RelatedTicket = relatedTicket
		}
	}

	// Tag Lightning channel funding/close transactions: on-chain they are
	// indistinguishable from external sends/receives (a funding output is a
	// 2-of-2 script the wallet does not own), so cross-reference dcrlnd's
	// channel list.
	fundingTxs, closingTxs := lightningChannelTxIDs(ctx)
	if len(fundingTxs) > 0 || len(closingTxs) > 0 {
		for i := range transactions {
			switch transactions[i].Category {
			case "send":
				transactions[i].IsChannelFunding = fundingTxs[transactions[i].TxID]
			case "receive":
				transactions[i].IsChannelClose = closingTxs[transactions[i].TxID]
			}
		}
	}

	sort.Slice(transactions, func(i, j int) bool {
		timeI := transactions[i].BlockTime
		if timeI == 0 {
			timeI = transactions[i].Time.Unix()
		}
		timeJ := transactions[j].BlockTime
		if timeJ == 0 {
			timeJ = transactions[j].Time.Unix()
		}
		return timeI > timeJ
	})

	return &types.TransactionListResponse{
		Transactions: transactions,
		Total:        len(transactions),
	}, nil
}

// listTxFacts carries what listtransactions cannot supply for a row: the
// CoinJoin verdict, the wallet's net position, and a vote's stakebase reward.
type listTxFacts struct {
	mixed      bool
	netDCR     float64
	voteReward float64
	hasReward  bool
}

// listTxMinHeight returns the lowest mined block height among the listed
// entries, or 0 (whole history) when none is derivable.
func listTxMinHeight(entries []listTxEntry, currentHeight int64) int32 {
	if currentHeight <= 0 {
		return 0
	}
	min := int32(0)
	for _, e := range entries {
		if e.Confirmations <= 0 {
			continue
		}
		h := int32(currentHeight - e.Confirmations + 1)
		if h > 0 && (min == 0 || h < min) {
			min = h
		}
	}
	return min
}

// listTxFactsFor streams the wallet's own view of the listed range once,
// mined and unmined, replacing the old lookup per transaction.
func listTxFactsFor(ctx context.Context, minHeight int32) (map[string]listTxFacts, error) {
	client := rpc.WalletGrpcClient
	if client == nil {
		return nil, fmt.Errorf("dcrwallet gRPC not available")
	}
	stream, err := client.GetTransactions(ctx, &pb.GetTransactionsRequest{
		StartingBlockHeight: minHeight,
		EndingBlockHeight:   -1,
	})
	if err != nil {
		return nil, err
	}
	facts := make(map[string]listTxFacts)
	add := func(details []*pb.TransactionDetails) {
		for _, t := range details {
			if txid, f, ok := deriveListTxFacts(t); ok {
				facts[txid] = f
			}
		}
	}
	for {
		resp, rerr := stream.Recv()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, rerr
		}
		if mb := resp.GetMinedTransactions(); mb != nil {
			add(mb.GetTransactions())
		}
		add(resp.GetUnminedTransactions())
	}
	return facts, nil
}

// deriveListTxFacts computes one transaction's facts from the streamed record:
// the raw bytes give the input count, output values and stakebase reward, and
// credits minus debits reproduce gettransaction's net amount exactly.
func deriveListTxFacts(t *pb.TransactionDetails) (string, listTxFacts, bool) {
	var mtx wire.MsgTx
	if err := mtx.Deserialize(bytes.NewReader(t.GetTransaction())); err != nil {
		return "", listTxFacts{}, false
	}
	txid := ""
	if h, err := chainhash.NewHash(t.GetHash()); err == nil {
		txid = h.String()
	} else {
		txid = hex.EncodeToString(t.GetHash())
	}
	values := make([]float64, len(mtx.TxOut))
	for i, out := range mtx.TxOut {
		values[i] = dcrutil.Amount(out.Value).ToCoin()
	}
	var net int64
	for _, c := range t.GetCredits() {
		net += c.GetAmount()
	}
	for _, d := range t.GetDebits() {
		net -= d.GetPreviousAmount()
	}
	f := listTxFacts{
		mixed:  looksLikeCoinJoin(len(mtx.TxIn), values),
		netDCR: dcrutil.Amount(net).ToCoin(),
	}
	if t.GetTransactionType() == pb.TransactionDetails_VOTE && len(mtx.TxIn) > 0 {
		f.voteReward = dcrutil.Amount(mtx.TxIn[0].ValueIn).ToCoin()
		f.hasReward = true
	}
	return txid, f, true
}

// isVSPFeeTransaction detects VSP fees by 6-block timing after ticket purchase (validated pattern)
func isVSPFeeTransaction(tx types.Transaction, allTransactions []types.Transaction) (bool, string) {
	if tx.Category != "send" || tx.TxType != "regular" {
		return false, ""
	}

	absAmount := tx.Amount
	if absAmount < 0 {
		absAmount = -absAmount
	}
	if absAmount < 0.001 || absAmount > 0.02 {
		return false, ""
	}

	if tx.BlockHeight == 0 {
		return false, ""
	}

	for _, otherTx := range allTransactions {
		if otherTx.TxType != "ticket" || otherTx.BlockHeight == 0 {
			continue
		}

		if tx.BlockHeight-otherTx.BlockHeight == 6 {
			wlltLog.Debugf("VSP FEE DETECTED: %s (block %d) is fee for ticket %s (block %d)",
				tx.TxID[:12], tx.BlockHeight, otherTx.TxID[:12], otherTx.BlockHeight)
			return true, otherTx.TxID
		}
	}

	return false, ""
}

func GetNextAddress(ctx context.Context, account uint32) (string, error) {
	if rpc.WalletGrpcClient == nil {
		return "", fmt.Errorf("wallet gRPC client not initialized")
	}
	resp, err := rpc.WalletGrpcClient.NextAddress(ctx, &pb.NextAddressRequest{
		Account:   account,
		Kind:      pb.NextAddressRequest_BIP0044_EXTERNAL,
		GapPolicy: pb.NextAddressRequest_GAP_POLICY_WRAP,
	})
	if err != nil {
		return "", err
	}
	return resp.Address, nil
}

func ValidateAddress(ctx context.Context, address string) (*pb.ValidateAddressResponse, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC client not initialized")
	}
	return rpc.WalletGrpcClient.ValidateAddress(ctx, &pb.ValidateAddressRequest{Address: address})
}

func ConstructTransaction(ctx context.Context, sourceAccount uint32, outputs []types.TxRecipient, sendAll bool) (*pb.ConstructTransactionResponse, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC client not initialized")
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("at least one output is required")
	}
	req := &pb.ConstructTransactionRequest{
		SourceAccount:         sourceAccount,
		RequiredConfirmations: 1,
	}
	if sendAll {
		// Send-all sweeps the whole balance to a single recipient via the change
		// destination, so only the first output's address is used.
		req.OutputSelectionAlgorithm = pb.ConstructTransactionRequest_ALL
		req.ChangeDestination = &pb.ConstructTransactionRequest_OutputDestination{Address: outputs[0].Address}
	} else {
		req.OutputSelectionAlgorithm = pb.ConstructTransactionRequest_UNSPECIFIED
		for _, o := range outputs {
			req.NonChangeOutputs = append(req.NonChangeOutputs, &pb.ConstructTransactionRequest_Output{
				Destination: &pb.ConstructTransactionRequest_OutputDestination{Address: o.Address},
				Amount:      o.AmountAtoms,
			})
		}
		// When spending the mixed account with privacy enabled, route change to
		// the unmixed account so mixed coins' change never pollutes the mixed
		// set. Mirrors Decrediton; otherwise dcrwallet defaults change to the
		// source account, which for a mixed-account send would land back in
		// mixed. For any other source (or privacy off) we leave it to dcrwallet.
		if mixing, ok := TicketMixingParams(ctx); ok && sourceAccount == mixing.Mixed {
			changeAddr, err := GetNextAddress(ctx, mixing.Change)
			if err != nil {
				return nil, fmt.Errorf("derive unmixed change address: %w", err)
			}
			req.ChangeDestination = &pb.ConstructTransactionRequest_OutputDestination{Address: changeAddr}
		}
	}
	return rpc.WalletGrpcClient.ConstructTransaction(ctx, req)
}

func DecodeRawTransaction(ctx context.Context, txBytes []byte) (*pb.DecodedTransaction, error) {
	if rpc.DecodeMessageClient == nil {
		return nil, fmt.Errorf("decode message gRPC client not initialized")
	}
	resp, err := rpc.DecodeMessageClient.DecodeRawTransaction(ctx, &pb.DecodeRawTransactionRequest{SerializedTransaction: txBytes})
	if err != nil {
		return nil, err
	}
	return resp.Transaction, nil
}

// ErrSpendWhileMixing is returned by SignAndPublishTransaction when a regular
// send is attempted while the privacy mixer or ticket autobuyer is running. They
// both spend the wallet's UTXOs, so they must not run at the same time. Mirrors
// Decrediton, which blocks the Send tab while either is active.
var ErrSpendWhileMixing = fmt.Errorf("stop the privacy mixer or ticket autobuyer before sending a transaction")

// ErrSpendWhilePurchasing is the ticket-purchase case, kept separate because the
// advice differs: a purchase cannot be stopped, only waited out.
var ErrSpendWhilePurchasing = fmt.Errorf("wait for the ticket purchase to finish before sending a transaction")

// spendGuard reports why a spend must not run right now, or nil. The mixer, the
// autobuyer and a ticket purchase all draw the same UTXOs. A purchase pauses the
// mixer for its own duration, so checking the mixer alone leaves that whole
// window open.
func spendGuard() error {
	if IsMixerRunning() || IsAutobuyerRunning() {
		return ErrSpendWhileMixing
	}
	if IsTicketPurchaseInProgress() {
		return ErrSpendWhilePurchasing
	}
	return nil
}

func SignAndPublishTransaction(ctx context.Context, sourceAccount uint32, unsignedTxBytes []byte, passphrase []byte) (string, error) {
	if err := spendGuard(); err != nil {
		return "", err
	}
	if rpc.WalletGrpcClient == nil {
		return "", fmt.Errorf("wallet gRPC client not initialized")
	}
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	beginUnlockedOp()
	defer endUnlockedOp()

	// Verify the passphrase and make the source account usable for signing,
	// migrating to per-account encryption if needed.
	didUnlock, err := unlockAccountForSpend(ctx, sourceAccount, passphrase)
	if err != nil {
		return "", err
	}
	// Only re-lock an account this send opened; one the VSP reconciler or a
	// mixer is holding open has to stay that way.
	if didUnlock {
		defer func() {
			relockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := rpc.WalletGrpcClient.LockAccount(relockCtx, &pb.LockAccountRequest{AccountNumber: sourceAccount}); err != nil {
				wlltLog.Errorf("SignAndPublishTransaction: lock account %d: %v", sourceAccount, err)
			}
		}()
	}

	signResp, err := rpc.WalletGrpcClient.SignTransaction(ctx, &pb.SignTransactionRequest{
		SerializedTransaction: unsignedTxBytes,
	})
	if err != nil {
		return "", err
	}
	pubResp, err := rpc.WalletGrpcClient.PublishTransaction(ctx, &pb.PublishTransactionRequest{
		SignedTransaction: signResp.Transaction,
	})
	if err != nil {
		return "", err
	}
	hash, err := chainhash.NewHash(pubResp.TransactionHash)
	if err != nil {
		return hex.EncodeToString(pubResp.TransactionHash), nil
	}
	return hash.String(), nil
}

// PartialPassphraseChangeError reports that the wallet passphrase was changed
// but the listed accounts kept the previous one. The wallet-wide change cannot
// be rolled back, so this is a state the user has to know about: those accounts
// cannot be unlocked for spending with either passphrase until they are updated.
type PartialPassphraseChangeError struct {
	Accounts []uint32
	Err      error
}

func (e *PartialPassphraseChangeError) Error() string {
	nums := make([]string, len(e.Accounts))
	for i, a := range e.Accounts {
		nums[i] = strconv.FormatUint(uint64(a), 10)
	}
	return fmt.Sprintf("the wallet passphrase was changed, but account(s) %s still use the previous one "+
		"and cannot be spent from until updated: %v", strings.Join(nums, ", "), e.Err)
}

func (e *PartialPassphraseChangeError) Unwrap() error { return e.Err }

// ChangePrivatePassphrase rotates the wallet's private (signing)
// passphrase and every account's per-account passphrase. Mirrors
// Decrediton's app/actions/ControlActions.js:187-232: wallet-wide
// rotation first, then a parallel fan-out of SetAccountPassphrase over
// every account with accountNumber < 2^31 - 1, always passing the old
// passphrase as AccountPassphrase. Every account is expected to be
// per-account-encrypted already (set at creation by CreateAccount).
// The caller is expected to zero both byte slices after this returns.
func ChangePrivatePassphrase(ctx context.Context, oldPass, newPass []byte) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC client not initialized")
	}

	// dcrwallet unlocks each account to re-encrypt it, so hold off the lock
	// sweep: it locks accounts it sees unlocked, and one landing mid-fan-out
	// makes the rest fail with "account must be unlocked".
	beginUnlockedOp()
	defer endUnlockedOp()

	if _, err := rpc.WalletGrpcClient.ChangePassphrase(ctx, &pb.ChangePassphraseRequest{
		Key:           pb.ChangePassphraseRequest_PRIVATE,
		OldPassphrase: oldPass,
		NewPassphrase: newPass,
	}); err != nil {
		return err
	}

	acctsResp, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{})
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}

	// Skip imported (2^31 - 1) and xpub-imported (>= 2^31) accounts.
	targets := make([]uint32, 0, len(acctsResp.GetAccounts()))
	for _, a := range acctsResp.GetAccounts() {
		if a.GetAccountNumber() < 2147483647 {
			targets = append(targets, a.GetAccountNumber())
		}
	}

	// Every account is attempted even if one fails. The wallet-wide change above
	// has already happened and cannot be undone, so cancelling the rest would
	// leave more accounts holding the old passphrase than necessary. Matches
	// Decrediton's Promise.all, which also lets every call finish.
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	for i, acct := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = rpc.WalletGrpcClient.SetAccountPassphrase(ctx, &pb.SetAccountPassphraseRequest{
				AccountNumber:        acct,
				AccountPassphrase:    oldPass,
				NewAccountPassphrase: newPass,
			})
		}()
	}
	wg.Wait()

	var failed []uint32
	var firstErr error
	for i, err := range errs {
		if err != nil {
			failed = append(failed, targets[i])
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if len(failed) > 0 {
		return &PartialPassphraseChangeError{Accounts: failed, Err: firstErr}
	}
	return nil
}

// DiscoverUsage unlocks the wallet and runs dcrwallet's DiscoverUsage gRPC to
// scan the chain for previously-used addresses of the existing accounts under
// gapLimit. Blocks until the scan completes. The wallet is re-locked on return.
//
// It requests address discovery only (DiscoverAccounts=false), matching
// Decrediton's post-setup Discover Address Usage. Account discovery runs only
// during a restore, before accounts are per-account-encrypted (runDiscoveryRpcSync).
func DiscoverUsage(ctx context.Context, passphrase []byte, gapLimit uint32) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC client not initialized")
	}

	unlockCtx, unlockCancel := context.WithTimeout(ctx, 10*time.Second)
	_, err := rpc.WalletGrpcClient.UnlockWallet(unlockCtx, &pb.UnlockWalletRequest{
		Passphrase: passphrase,
	})
	unlockCancel()
	if err != nil {
		return fmt.Errorf("unlock wallet: %w", err)
	}
	defer func() {
		lockCtx, lockCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer lockCancel()
		if _, err := rpc.WalletGrpcClient.LockWallet(lockCtx, &pb.LockWalletRequest{}); err != nil {
			wlltLog.Errorf("DiscoverUsage: lock wallet: %v", err)
		}
	}()

	if _, err := rpc.WalletGrpcClient.DiscoverUsage(ctx, &pb.DiscoverUsageRequest{
		DiscoverAccounts: false,
		GapLimit:         gapLimit,
	}); err != nil {
		return fmt.Errorf("DiscoverUsage RPC: %w", err)
	}
	return nil
}
