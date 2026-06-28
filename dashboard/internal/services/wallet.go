// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"golang.org/x/sync/errgroup"
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
// index. Normal accounts (< 2^31) are their own index. Imported xpub accounts
// (>= 2^31) resolve through the recorded mapping; a missing entry is an error so
// offline signing never derives against the wrong account on the device.
func Bip44AccountIndex(ctx context.Context, acctNum uint32) (uint32, error) {
	if acctNum < importedXpubAccountBase {
		return acctNum, nil
	}
	if idx, ok := loadXpubAccountIndexes(ctx)[fmt.Sprintf("%d", acctNum)]; ok {
		return idx, nil
	}
	return 0, fmt.Errorf("unknown BIP44 account index for imported account %d; re-import its xpub specifying the account index", acctNum)
}

func FetchWalletDashboardData() (*types.WalletDashboardData, error) {
	ctx := context.Background()
	return FetchWalletDashboardDataWithContext(ctx)
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

	go func() {
		info, err := FetchAccountInfoWithContext(ctx)
		accountChan <- accountResult{info, err}
	}()

	go func() {
		accts, err := FetchAllAccounts(ctx)
		accountsChan <- accountsResult{accts, err}
	}()

	go func() {
		staking, err := FetchWalletStakingInfo(ctx)
		stakingChan <- stakingResult{staking, err}
	}()

	select {
	case res := <-accountChan:
		if res.err != nil {
			log.Printf("Warning: Failed to fetch account info: %v", res.err)
		} else {
			accountInfo = res.data
		}
	case <-ctx.Done():
		log.Printf("Warning: Account info fetch cancelled: %v", ctx.Err())
	}

	select {
	case res := <-accountsChan:
		if res.err != nil {
			log.Printf("Warning: Failed to fetch accounts: %v", res.err)
		} else {
			accounts = res.data
		}
	case <-ctx.Done():
		log.Printf("Warning: Accounts fetch cancelled: %v", ctx.Err())
	}

	select {
	case res := <-stakingChan:
		if res.err != nil {
			log.Printf("Warning: Failed to fetch staking info: %v", res.err)
			// Staking info is optional - continue without it
		} else {
			stakingInfo = res.data
		}
	case <-ctx.Done():
		log.Printf("Warning: Staking info fetch cancelled: %v", ctx.Err())
	}

	return &types.WalletDashboardData{
		WalletStatus: *walletStatus,
		AccountInfo:  *accountInfo,
		Accounts:     accounts,
		StakingInfo:  stakingInfo,
		LastUpdate:   time.Now(),
	}, nil
}

func FetchAccountInfo() (*types.AccountInfo, error) {
	return FetchAccountInfoWithContext(context.Background())
}

func FetchAccountInfoWithContext(ctx context.Context) (*types.AccountInfo, error) {
	// Get balance using getbalance (no arguments for all accounts)
	result, err := rpc.WalletClient.RawRequest(ctx, "getbalance", []json.RawMessage{})
	if err != nil {
		log.Printf("Warning: Failed to get balance: %v", err)
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

	// Parse the full balance response structure
	// getbalance returns: {
	//   "balances":[{account info},...],
	//   "blockhash":"...",
	//   "totallockedbytickets": X,
	//   "totalspendable": Y,
	//   "cumulativetotal": Z
	// }
	type AccountBalance struct {
		AccountName             string  `json:"accountname"`
		ImmatureCoinbaseRewards float64 `json:"immaturecoinbaserewards"`
		ImmatureStakeGeneration float64 `json:"immaturestakegeneration"`
		LockedByTickets         float64 `json:"lockedbytickets"`
		Spendable               float64 `json:"spendable"`
		Total                   float64 `json:"total"`
		Unconfirmed             float64 `json:"unconfirmed"`
		VotingAuthority         float64 `json:"votingauthority"`
	}
	type BalanceResponse struct {
		Balances             []AccountBalance `json:"balances"`
		BlockHash            string           `json:"blockhash"`
		TotalLockedByTickets float64          `json:"totallockedbytickets"`
		TotalSpendable       float64          `json:"totalspendable"`
		CumulativeTotal      float64          `json:"cumulativetotal"`
	}

	var balanceResp BalanceResponse
	if err := json.Unmarshal(result, &balanceResp); err != nil {
		log.Printf("Warning: Failed to unmarshal balance response: %v", err)
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
	// Get all accounts and their balances using getbalance RPC
	result, err := rpc.WalletClient.RawRequest(ctx, "getbalance", []json.RawMessage{})
	if err != nil {
		log.Printf("Warning: Failed to get accounts: %v", err)
		return []types.AccountInfo{}, nil
	}

	// Parse the balance response structure
	type AccountBalance struct {
		AccountName             string  `json:"accountname"`
		ImmatureCoinbaseRewards float64 `json:"immaturecoinbaserewards"`
		ImmatureStakeGeneration float64 `json:"immaturestakegeneration"`
		LockedByTickets         float64 `json:"lockedbytickets"`
		Spendable               float64 `json:"spendable"`
		Total                   float64 `json:"total"`
		Unconfirmed             float64 `json:"unconfirmed"`
		VotingAuthority         float64 `json:"votingauthority"`
	}
	type BalanceResponse struct {
		Balances  []AccountBalance `json:"balances"`
		BlockHash string           `json:"blockhash"`
	}

	var balanceResp BalanceResponse
	if err := json.Unmarshal(result, &balanceResp); err != nil {
		log.Printf("Warning: Failed to unmarshal accounts: %v", err)
		return []types.AccountInfo{}, nil
	}

	// getbalance does not return account numbers or per-account encryption
	// state; resolve them via gRPC Accounts.
	numbers := map[string]uint32{}
	encrypted := map[string]bool{}
	unlocked := map[string]bool{}
	if rpc.WalletGrpcClient != nil {
		if acctsResp, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{}); err != nil {
			log.Printf("Warning: gRPC Accounts call failed, account numbers will be 0: %v", err)
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

// unlockAccountForSpend makes a per-account-encrypted account usable for signing.
// It first checks whether the account is already unlocked: dcrwallet's
// UnlockAccount, when called on an already-unlocked account, does a strict
// passphrase-hash compare against the passphrase that first unlocked it and
// returns "invalid passphrase" on any mismatch (e.g. the account was unlocked
// earlier by a mix session). Skipping the redundant unlock avoids that. When the
// account is locked, it unlocks, lazily migrating to per-account encryption if
// needed (the default account on a fresh wallet isn't encrypted yet).
func unlockAccountForSpend(ctx context.Context, accountNumber uint32, passphrase []byte) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC client not initialized")
	}
	acctsResp, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{})
	if err == nil {
		for _, a := range acctsResp.Accounts {
			if a.AccountNumber == accountNumber {
				if a.AccountUnlocked {
					return nil // already usable; don't re-unlock (avoids the hash compare)
				}
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
				return fmt.Errorf("migrate account to per-account encryption: %w", mErr)
			}
			if _, err := rpc.WalletGrpcClient.UnlockAccount(ctx, &pb.UnlockAccountRequest{
				Passphrase:    passphrase,
				AccountNumber: accountNumber,
			}); err != nil {
				return fmt.Errorf("unlock source account: %w", err)
			}
			return nil
		}
		return fmt.Errorf("unlock source account: %w", err)
	}
	return nil
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
	// Snapshot current unlock state so we only report (and later re-lock)
	// accounts we ourselves unlock, leaving any already-unlocked account (e.g.
	// one a running mixer needs) untouched.
	alreadyUnlocked := map[uint32]bool{}
	if resp, aerr := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{}); aerr == nil {
		for _, a := range resp.Accounts {
			if a.AccountUnlocked {
				alreadyUnlocked[a.AccountNumber] = true
			}
		}
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
		if alreadyUnlocked[a.AccountNumber] {
			succeeded++
			continue
		}
		if err := unlockAccountForSpend(ctx, a.AccountNumber, passphrase); err != nil {
			// An account may carry a divergent per-account passphrase; skip it
			// rather than abort, so the accounts that do unlock (including each
			// ticket's commitment account) can still sign for the VSP. A
			// genuinely wrong passphrase fails every account, handled below.
			log.Printf("unlockAllAccountsForSpend: skipping account %q (%d): %v", a.AccountName, a.AccountNumber, err)
			continue
		}
		succeeded++
		newlyUnlocked = append(newlyUnlocked, a.AccountNumber)
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
		log.Printf("vspTicketCommitAccounts: GetTrackedVSPTickets: %v", err)
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
		_, _ = rpc.WalletGrpcClient.LockAccount(ctx, &pb.LockAccountRequest{AccountNumber: acct})
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

// Old FetchTransactions functions removed - replaced by ListTransactions

func FetchAddresses() ([]types.Address, error) {
	return FetchAddressesWithContext(context.Background())
}

func FetchAddressesWithContext(ctx context.Context) ([]types.Address, error) {
	// List addresses via raw RPC - only return addresses with funds (not empty)
	// This prevents returning 40k+ empty addresses
	result, err := rpc.WalletClient.RawRequest(ctx, "listreceivedbyaddress", []json.RawMessage{
		json.RawMessage(`0`),     // minconf
		json.RawMessage(`false`), // include empty = false (only show addresses with funds)
	})
	if err != nil {
		log.Printf("Warning: Failed to list addresses: %v", err)
		return []types.Address{}, nil
	}

	// Parse the result
	var rawAddrList []map[string]interface{}
	if err := json.Unmarshal(result, &rawAddrList); err != nil {
		log.Printf("Warning: Failed to unmarshal addresses: %v", err)
		return []types.Address{}, nil
	}

	// Limit to 100 addresses max to prevent huge payloads
	maxAddresses := 100
	if len(rawAddrList) > maxAddresses {
		log.Printf("Warning: Wallet has %d addresses with funds, limiting to %d", len(rawAddrList), maxAddresses)
		rawAddrList = rawAddrList[:maxAddresses]
	}

	addresses := make([]types.Address, 0, len(rawAddrList))
	for _, addr := range rawAddrList {
		address, _ := addr["address"].(string)
		account, _ := addr["account"].(string)
		amount, _ := addr["amount"].(float64)

		addresses = append(addresses, types.Address{
			Address: address,
			Account: account,
			Used:    amount > 0, // Has received funds
			Path:    "",         // Would need to query separately
		})
	}

	log.Printf("Returning %d addresses with funds", len(addresses))
	return addresses, nil
}

func FetchWalletStakingInfo(ctx context.Context) (*types.WalletStakingInfo, error) {
	stakingInfo := &types.WalletStakingInfo{}

	// Fetch getstakeinfo
	stakeInfoResult, err := rpc.WalletClient.RawRequest(ctx, "getstakeinfo", []json.RawMessage{})
	if err != nil {
		log.Printf("Warning: Failed to get stake info: %v", err)
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
		log.Printf("Warning: Failed to unmarshal stake info: %v", err)
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

	// Fetch estimatestakediff
	estimateResult, err := rpc.WalletClient.RawRequest(ctx, "estimatestakediff", []json.RawMessage{})
	if err != nil {
		log.Printf("Warning: Failed to estimate stake diff: %v", err)
	} else {
		type EstimateResponse struct {
			Min      float64 `json:"min"`
			Max      float64 `json:"max"`
			Expected float64 `json:"expected"`
		}
		var estimate EstimateResponse
		if err := json.Unmarshal(estimateResult, &estimate); err == nil {
			stakingInfo.EstimatedMin = estimate.Min
			stakingInfo.EstimatedMax = estimate.Max
			stakingInfo.EstimatedExpected = estimate.Expected
		}
	}

	// Fetch getstakedifficulty
	difficultyResult, err := rpc.WalletClient.RawRequest(ctx, "getstakedifficulty", []json.RawMessage{})
	if err != nil {
		log.Printf("Warning: Failed to get stake difficulty: %v", err)
	} else {
		type DifficultyResponse struct {
			Current float64 `json:"current"`
			Next    float64 `json:"next"`
		}
		var difficulty DifficultyResponse
		if err := json.Unmarshal(difficultyResult, &difficulty); err == nil {
			stakingInfo.CurrentDifficulty = difficulty.Current
			stakingInfo.NextDifficulty = difficulty.Next
		}
	}

	// Fetch dcrd getblocksubsidy for the next block (current PoS reward).
	// chaincfg.MainNetParams SubsidyReductionInterval is 6144.
	const subsidyReductionInterval int64 = 6144
	stakingInfo.SubsidyReductionInterval = subsidyReductionInterval
	if rpc.DcrdClient != nil {
		chainHeight, err := rpc.DcrdClient.GetBlockCount(ctx)
		if err != nil {
			log.Printf("Warning: Failed to get chain height for block subsidy: %v", err)
		} else {
			nextHeight := chainHeight + 1
			subsidyResult, err := rpc.DcrdClient.RawRequest(ctx, "getblocksubsidy", []json.RawMessage{
				json.RawMessage(fmt.Sprintf("%d", nextHeight)),
				json.RawMessage("5"),
			})
			if err != nil {
				log.Printf("Warning: Failed to get block subsidy: %v", err)
			} else {
				type SubsidyResponse struct {
					Developer int64 `json:"developer"`
					PoS       int64 `json:"pos"`
					PoW       int64 `json:"pow"`
					Total     int64 `json:"total"`
				}
				var subsidy SubsidyResponse
				if err := json.Unmarshal(subsidyResult, &subsidy); err != nil {
					log.Printf("Warning: Failed to unmarshal block subsidy: %v", err)
				} else {
					stakingInfo.BlockSubsidyHeight = nextHeight
					stakingInfo.BlockSubsidyTotal = float64(subsidy.Total) / 1e8
					stakingInfo.BlockSubsidyPoS = float64(subsidy.PoS) / 1e8
					stakingInfo.BlockSubsidyPoW = float64(subsidy.PoW) / 1e8
					stakingInfo.BlockSubsidyTreasury = float64(subsidy.Developer) / 1e8
					stakingInfo.BlocksUntilSubsidyReduction = subsidyReductionInterval - (chainHeight % subsidyReductionInterval)
				}
			}
		}
	}

	return stakingInfo, nil
}

// ListTransactions fetches recent wallet transactions
func ListTransactions(ctx context.Context, count, from int) (*types.TransactionListResponse, error) {
	// Default parameters
	if count <= 0 {
		count = 50 // Default to 50 transactions
	}
	if count > 10000 {
		count = 10000 // Cap at 10000 for performance
	}

	// Get current chain height for maturity calculations
	var currentHeight int64 = 0
	if rpc.DcrdClient != nil {
		chainHeight, err := rpc.DcrdClient.GetBlockCount(ctx)
		if err == nil {
			currentHeight = chainHeight
		}
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
	var rpcTransactions []struct {
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

	if err := json.Unmarshal(result, &rpcTransactions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transactions: %w", err)
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

	// Process transactions - use gettransaction for accurate net amounts on multi-entry txs
	txGroups := make(map[string]*types.Transaction)
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
				isMixed = isCoinJoinTransaction(ctx, rpcTx.TxID)
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

			// CoinJoin: use gettransaction (listtransactions includes other participants)
			if isMixed {
				var err error
				netAmount, err = getTransactionNetAmount(ctx, rpcTx.TxID)
				if err != nil {
					log.Printf("Warning: Could not get net amount for CoinJoin %s: %v, skipping", rpcTx.TxID[:12], err)
					processed[rpcTx.TxID] = true
					continue
				}
				log.Printf("CoinJoin %s: wallet net amount = %.8f DCR", rpcTx.TxID[:12], netAmount)
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
				log.Printf("TX %s involves multiple accounts: %v", rpcTx.TxID[:12], accounts)
			}

			var blockHeight int64 = 0
			if currentHeight > 0 && rpcTx.Confirmations > 0 {
				blockHeight = currentHeight - rpcTx.Confirmations + 1
			}

			// Determine category from net amount for regular txs
			category := rpcTx.Category
			if rpcTx.TxType == "regular" {
				if isMixed {
					category = "coinjoin"
				} else if netAmount > 0 {
					category = "receive"
				} else if netAmount < 0 {
					category = "send"
				} else {
					category = "self"
				}
			}

			// CoinJoin fee is the cost to participate
			var fee float64 = 0
			if isMixed && netAmount < 0 {
				fee = -netAmount
			}

			// Vote maturity: a vote's returned stake + reward matures over 256
			// blocks before it is spendable. A vote spans multiple listtransactions
			// entries, so it lands here rather than the single-entry branches below;
			// compute maturity here too or the row is stuck at "Voted (Maturing)".
			var isTicketMature bool = false
			var blocksUntilSpendable int64 = 0
			if rpcTx.TxType == "vote" && blockHeight > 0 && currentHeight > 0 {
				blocksPassed := currentHeight - blockHeight
				if blocksPassed >= 256 {
					isTicketMature = true
				} else {
					blocksUntilSpendable = 256 - blocksPassed
				}
			}

			tx := types.Transaction{
				TxID:                 rpcTx.TxID,
				Amount:               netAmount,
				Fee:                  fee,
				Confirmations:        rpcTx.Confirmations,
				BlockHash:            rpcTx.BlockHash,
				BlockTime:            rpcTx.BlockTime,
				Time:                 time.Unix(rpcTx.Time, 0),
				Category:             category,
				TxType:               rpcTx.TxType,
				Address:              "",
				Account:              accountName,
				Vout:                 0,
				Generated:            false,
				IsMixed:              isMixed,
				BlockHeight:          blockHeight,
				IsTicketMature:       isTicketMature,
				BlocksUntilSpendable: blocksUntilSpendable,
			}
			transactions = append(transactions, tx)
			processed[rpcTx.TxID] = true
			continue
		}

		isMixed := false
		if rpcTx.TxType == "regular" {
			isMixed = isCoinJoinTransaction(ctx, rpcTx.TxID)
		}

		txGroupKey := func(txid, category string) string {
			return fmt.Sprintf("%s-%s", txid, category)
		}

		// Group receive transactions with same txid
		if rpcTx.Category == "receive" {
			groupKey := txGroupKey(rpcTx.TxID, rpcTx.Category)

			if existing, exists := txGroups[groupKey]; exists {
				existing.Amount += rpcTx.Amount
				if existing.Address == "" && rpcTx.Address != "" {
					existing.Address = rpcTx.Address
				}
			} else {
				var blockHeight int64 = 0
				if currentHeight > 0 && rpcTx.Confirmations > 0 {
					blockHeight = currentHeight - rpcTx.Confirmations + 1
				}

				// Vote maturity: 256 blocks required before funds are spendable
				var isTicketMature bool = false
				var blocksUntilSpendable int64 = 0
				if rpcTx.TxType == "vote" && blockHeight > 0 && currentHeight > 0 {
					blocksPassed := currentHeight - blockHeight
					if blocksPassed >= 256 {
						isTicketMature = true
						blocksUntilSpendable = 0
					} else {
						isTicketMature = false
						blocksUntilSpendable = 256 - blocksPassed
					}
				}

				tx := &types.Transaction{
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
				txGroups[groupKey] = tx
			}
		} else {
			var blockHeight int64 = 0
			if currentHeight > 0 && rpcTx.Confirmations > 0 {
				blockHeight = currentHeight - rpcTx.Confirmations + 1
			}

			var isTicketMature bool = false
			var blocksUntilSpendable int64 = 0
			if rpcTx.TxType == "vote" && blockHeight > 0 && currentHeight > 0 {
				blocksPassed := currentHeight - blockHeight
				if blocksPassed >= 256 {
					isTicketMature = true
					blocksUntilSpendable = 0
				} else {
					isTicketMature = false
					blocksUntilSpendable = 256 - blocksPassed
				}
			}

			tx := types.Transaction{
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
			transactions = append(transactions, tx)
		}

		processed[rpcTx.TxID] = true
	}

	// Add grouped receives to main list
	for _, tx := range txGroups {
		transactions = append(transactions, *tx)
	}

	// A vote's listtransactions net cancels to ~0; show the stakebase reward
	// read directly from the vote transaction instead.
	for i := range transactions {
		if transactions[i].TxType == "vote" {
			if reward, ok := voteStakebaseReward(ctx, transactions[i].TxID); ok {
				transactions[i].Amount = reward
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

// getTransactionNetAmount returns wallet's net position (credits - debits) using gettransaction
func getTransactionNetAmount(ctx context.Context, txHash string) (float64, error) {
	if rpc.WalletClient == nil {
		return 0, fmt.Errorf("wallet client not available")
	}

	result, err := rpc.WalletClient.RawRequest(ctx, "gettransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, txHash)),
	})
	if err != nil {
		return 0, fmt.Errorf("gettransaction failed: %w", err)
	}

	var txInfo struct {
		Amount float64 `json:"amount"`
		Fee    float64 `json:"fee"`
	}

	if err := json.Unmarshal(result, &txInfo); err != nil {
		return 0, fmt.Errorf("failed to parse gettransaction: %w", err)
	}

	return txInfo.Amount, nil
}

// voteStakebaseReward returns a vote (SSGen) transaction's stakebase input value,
// which is the staking reward returned to the ticket. The first input of a vote
// is the stakebase; its amountin is the reward, read directly from dcrd. Returns
// false when dcrd is unavailable or the tx is not a vote.
func voteStakebaseReward(ctx context.Context, txHash string) (float64, bool) {
	if rpc.DcrdClient == nil {
		return 0, false
	}

	rawTxResult, err := rpc.DcrdClient.RawRequest(ctx, "getrawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, txHash)),
		json.RawMessage("1"),
	})
	if err != nil {
		log.Printf("Vote reward lookup failed for %s: getrawtransaction error: %v", txHash, err)
		return 0, false
	}

	var tx struct {
		Vin []struct {
			Stakebase string  `json:"stakebase,omitempty"`
			AmountIn  float64 `json:"amountin"`
		} `json:"vin"`
	}

	if err := json.Unmarshal(rawTxResult, &tx); err != nil {
		log.Printf("Vote reward lookup failed for %s: unmarshal error: %v", txHash, err)
		return 0, false
	}

	if len(tx.Vin) == 0 || tx.Vin[0].Stakebase == "" {
		return 0, false
	}

	return tx.Vin[0].AmountIn, true
}

// isCoinJoinTransaction detects CoinJoin by analyzing tx structure (3+ inputs/outputs, matching amounts)
func isCoinJoinTransaction(ctx context.Context, txHash string) bool {
	if rpc.DcrdClient == nil {
		log.Printf("CoinJoin check skipped for %s: no dcrd connection", txHash)
		return false
	}

	rawTxResult, err := rpc.DcrdClient.RawRequest(ctx, "getrawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, txHash)),
		json.RawMessage("1"),
	})
	if err != nil {
		log.Printf("CoinJoin check failed for %s: getrawtransaction error: %v", txHash, err)
		return false
	}

	var tx struct {
		Vin []struct {
			Txid string `json:"txid,omitempty"`
		} `json:"vin"`
		Vout []struct {
			Value float64 `json:"value"`
		} `json:"vout"`
	}

	if err := json.Unmarshal(rawTxResult, &tx); err != nil {
		log.Printf("CoinJoin check failed for %s: unmarshal error: %v", txHash, err)
		return false
	}

	log.Printf("Analyzing tx %s: %d inputs, %d outputs", txHash, len(tx.Vin), len(tx.Vout))

	if len(tx.Vin) < 3 {
		log.Printf("TX %s: not enough inputs (%d < 3)", txHash, len(tx.Vin))
		return false
	}

	if len(tx.Vout) < 3 {
		log.Printf("TX %s: not enough outputs (%d < 3)", txHash, len(tx.Vout))
		return false
	}

	outputValues := make(map[float64]int)
	for _, vout := range tx.Vout {
		rounded := float64(int64(vout.Value*100000000)) / 100000000
		outputValues[rounded]++
	}

	log.Printf("TX %s: output value distribution: %v", txHash, outputValues)

	for value, count := range outputValues {
		if count >= 3 {
			log.Printf("TX %s: IDENTIFIED AS COINJOIN - %d outputs with value %.8f", txHash, count, value)
			return true
		}
	}

	log.Printf("TX %s: not a CoinJoin (no 3+ matching output values)", txHash)
	return false
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
			log.Printf("VSP FEE DETECTED: %s (block %d) is fee for ticket %s (block %d)",
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

func SignAndPublishTransaction(ctx context.Context, sourceAccount uint32, unsignedTxBytes []byte, passphrase []byte) (string, error) {
	if rpc.WalletGrpcClient == nil {
		return "", fmt.Errorf("wallet gRPC client not initialized")
	}
	if IsMixerRunning() || IsAutobuyerRunning() {
		return "", ErrSpendWhileMixing
	}
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	// Make the source account usable for signing (skips if already unlocked,
	// migrates to per-account encryption if needed), then auto-relock on return.
	if err := unlockAccountForSpend(ctx, sourceAccount, passphrase); err != nil {
		return "", err
	}
	defer func() {
		relockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = rpc.WalletGrpcClient.LockAccount(relockCtx, &pb.LockAccountRequest{AccountNumber: sourceAccount})
	}()

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

	g, gctx := errgroup.WithContext(ctx)
	for _, a := range acctsResp.GetAccounts() {
		a := a
		// Skip imported (2^31 - 1) and xpub-imported (>= 2^31) accounts.
		if a.GetAccountNumber() >= 2147483647 {
			continue
		}
		g.Go(func() error {
			_, perr := rpc.WalletGrpcClient.SetAccountPassphrase(gctx, &pb.SetAccountPassphraseRequest{
				AccountNumber:        a.GetAccountNumber(),
				AccountPassphrase:    oldPass,
				NewAccountPassphrase: newPass,
			})
			return perr
		})
	}
	return g.Wait()
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
		_, _ = rpc.WalletGrpcClient.LockWallet(lockCtx, &pb.LockWalletRequest{})
	}()

	if _, err := rpc.WalletGrpcClient.DiscoverUsage(ctx, &pb.DiscoverUsageRequest{
		DiscoverAccounts: false,
		GapLimit:         gapLimit,
	}); err != nil {
		return fmt.Errorf("DiscoverUsage RPC: %w", err)
	}
	return nil
}
