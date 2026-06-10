// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/dexassets"
	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/pkg/bisonw"
	"dcrpulse/pkg/exchangerate"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/gorilla/websocket"
)

// dexAccountName is the dedicated dcrwallet account DCRDEX trades from.
const dexAccountName = services.DexAccountName

// DcrdexStatus reports reachability and versions of the backend-only bisonw
// daemon. Richer onboarding state (initialized/logged-in/registered) is added
// alongside the init/login routes.
type DcrdexStatus struct {
	Reachable     bool   `json:"reachable"`
	Initialized   bool   `json:"initialized"`
	Unlocked      bool   `json:"unlocked"`
	SeedBackedUp  bool   `json:"seedBackedUp"`
	Stage         string `json:"stage"` // unavailable | needs-init | needs-unlock | ready
	BisonwVersion string `json:"bisonwVersion,omitempty"`
	RPCServerVer  string `json:"rpcServerVersion,omitempty"`
	Error         string `json:"error,omitempty"`
}

// GetDcrdexStatusHandler reports whether the bisonw RPC server is reachable and
// its versions, via the version route (no app initialization required).
func GetDcrdexStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	st := DcrdexStatus{Initialized: dcrdexInitialized(), SeedBackedUp: dcrdexSeedBackedUp()}
	_, st.Unlocked = rpc.DcrdexAppPass()

	client, err := rpc.DcrdexClient()
	if err != nil {
		st.Stage = "unavailable"
		st.Error = err.Error()
		json.NewEncoder(w).Encode(st)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	v, err := client.Version(ctx)
	if err != nil {
		st.Stage = "unavailable"
		st.Error = err.Error()
		json.NewEncoder(w).Encode(st)
		return
	}

	st.Reachable = true
	if v.Bisonw != nil {
		st.BisonwVersion = v.Bisonw.VersionString
	}
	if v.RPCServerVersion != nil {
		st.RPCServerVer = formatSemver(v.RPCServerVersion.Major, v.RPCServerVersion.Minor, v.RPCServerVersion.Patch)
	}
	switch {
	case !st.Initialized:
		st.Stage = "needs-init"
	case !st.Unlocked:
		st.Stage = "needs-unlock"
	default:
		if has, herr := client.HasWallet(ctx, bisonw.AssetDCR); herr == nil && !has {
			st.Stage = "needs-wallet"
		} else {
			st.Stage = "ready"
		}
	}
	json.NewEncoder(w).Encode(st)
}

// dexWalletCfg loads the active wallet's per-wallet config. DEX onboarding state
// is per wallet: each wallet has its own bisonw appdata, app seed, accounts, and
// bonds, so a freshly-switched wallet resolves to its own init/backup state.
func dexWalletCfg() (*config.WalletCfg, error) {
	net, err := services.CurrentNetwork(context.Background())
	if err != nil {
		return nil, err
	}
	return config.LoadWalletCfg(net, services.CurrentWalletName())
}

func dcrdexInitialized() bool {
	cfg, err := dexWalletCfg()
	if err != nil {
		return false
	}
	var b bool
	cfg.Get(config.KeyDcrdexInitialized, &b)
	return b
}

func setDcrdexInitialized() error {
	cfg, err := dexWalletCfg()
	if err != nil {
		return err
	}
	if err := cfg.Set(config.KeyDcrdexInitialized, true); err != nil {
		return err
	}
	return cfg.Save()
}

func dcrdexSeedBackedUp() bool {
	cfg, err := dexWalletCfg()
	if err != nil {
		return false
	}
	var b bool
	cfg.Get(config.KeyDcrdexSeedBackedUp, &b)
	return b
}

func setDcrdexSeedBackedUp(v bool) error {
	cfg, err := dexWalletCfg()
	if err != nil {
		return err
	}
	if err := cfg.Set(config.KeyDcrdexSeedBackedUp, v); err != nil {
		return err
	}
	return cfg.Save()
}

func formatSemver(major, minor, patch uint32) string {
	return itoa(major) + "." + itoa(minor) + "." + itoa(patch)
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

type dcrdexAuthRequest struct {
	AppPass string `json:"appPass"`
	Seed    string `json:"seed,omitempty"`
}

// InitDcrdexHandler initializes the bisonw client with a user-supplied app
// password (optionally restoring from a seed), logs in, and holds the password
// in memory for the session. The password is never persisted.
func InitDcrdexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if ready, reason := services.WalletReady(r.Context()); !ready {
		http.Error(w, reason, http.StatusServiceUnavailable)
		return
	}
	var req dcrdexAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AppPass == "" {
		http.Error(w, "appPass is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.Init(ctx, req.AppPass, req.Seed); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if _, err := client.Login(ctx, req.AppPass); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	rpc.SetDcrdexAppPass(req.AppPass)
	if err := setDcrdexInitialized(); err != nil {
		log.Printf("dcrdex: persist initialized flag: %v", err)
	}
	// A restored seed is already backed up by definition; a freshly generated one
	// is not, so the unlock nag prompts the user to back it up.
	if err := setDcrdexSeedBackedUp(req.Seed != ""); err != nil {
		log.Printf("dcrdex: persist seed-backed-up flag: %v", err)
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// UnlockDcrdexHandler logs the bisonw client in with the supplied app password
// and holds it in memory for the session (used after a restart re-locks it).
func UnlockDcrdexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req dcrdexAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AppPass == "" {
		http.Error(w, "appPass is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := client.Login(ctx, req.AppPass); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	rpc.SetDcrdexAppPass(req.AppPass)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// LockDcrdexHandler logs the bisonw client out and forgets the in-memory app
// password.
func LockDcrdexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Only forget the in-memory app password if bisonw actually locked. Logout
	// refuses (and locks nothing) while any order is still active, so clearing
	// the password regardless would leave the daemon - wallets, dex account and
	// any running bot - unlocked behind a "locked" dashboard.
	if client, err := rpc.DcrdexClient(); err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := client.Logout(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	rpc.ClearDcrdexAppPass()
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// CreateDcrdexWalletHandler configures DCRDEX's Decred wallet against the
// dashboard's dcrwallet. It ensures a dedicated `dex` account exists (creating
// it with the supplied wallet passphrase if missing), then registers the
// dcrwalletRPC wallet in bisonw. Requires the DEX session to be unlocked.
func CreateDcrdexWalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if ready, reason := services.WalletReady(r.Context()); !ready {
		http.Error(w, reason, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		WalletPass string `json:"walletPass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WalletPass == "" {
		http.Error(w, "walletPass is required", http.StatusBadRequest)
		return
	}
	appPass, ok := rpc.DcrdexAppPass()
	if !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	if err := ensureDexAccount(ctx, []byte(req.WalletPass)); err != nil {
		http.Error(w, "dex account: "+err.Error(), http.StatusBadGateway)
		return
	}

	cfg := bisonw.DCRWalletRPCConfig{
		Account:   dexAccountName,
		Username:  dexEnv("DCRWALLET_RPC_USER", "dcrwallet"),
		Password:  dexEnv("DCRWALLET_RPC_PASS", "dcrwalletpass"),
		RPCListen: dexEnv("DCRWALLET_RPC_HOST", "dcrwallet") + ":" + dexEnv("DCRWALLET_RPC_PORT", "9110"),
		RPCCert:   dexEnv("DCRDEX_DCRWALLET_CERT", "/app-data/dcrd/rpc.cert"),
	}
	if err := client.NewDCRWallet(ctx, appPass, req.WalletPass, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ensureDexAccount makes sure the dedicated `dex` account exists in dcrwallet,
// creating it (with the wallet passphrase) if not present.
func ensureDexAccount(ctx context.Context, passphrase []byte) error {
	accts, err := services.FetchAllAccounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accts {
		if a.AccountName == dexAccountName {
			return nil
		}
	}
	_, err = services.CreateAccount(ctx, dexAccountName, passphrase)
	return err
}

func dexEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// DcrdexWalletInfo is the read-only view of DCRDEX's Decred wallet used by the
// registration screen to show funding status. The balance is converted to DCR
// in the backend with dcrutil.
type DcrdexWalletInfo struct {
	Configured   bool    `json:"configured"`
	AvailableDcr float64 `json:"availableDcr"`
	Address      string  `json:"address"`
	Synced       bool    `json:"synced"`
	SyncProgress float32 `json:"syncProgress"`
}

// GetDcrdexWalletHandler returns the DCRDEX Decred wallet's available balance
// (in DCR) and deposit address, so the user can fund it before posting a bond.
func GetDcrdexWalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	raw, err := client.Wallets(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var states []struct {
		AssetID      uint32  `json:"assetID"`
		Address      string  `json:"address"`
		Synced       bool    `json:"synced"`
		SyncProgress float32 `json:"syncProgress"`
		Balance      struct {
			Available uint64 `json:"available"`
		} `json:"balance"`
	}
	if err := json.Unmarshal(raw, &states); err != nil {
		http.Error(w, "decode wallets: "+err.Error(), http.StatusBadGateway)
		return
	}
	for _, s := range states {
		if s.AssetID == bisonw.AssetDCR {
			json.NewEncoder(w).Encode(DcrdexWalletInfo{
				Configured:   true,
				AvailableDcr: dcrutil.Amount(s.Balance.Available).ToCoin(),
				Address:      s.Address,
				Synced:       s.Synced,
				SyncProgress: s.SyncProgress,
			})
			return
		}
	}
	json.NewEncoder(w).Encode(DcrdexWalletInfo{Configured: false})
}

// GetDcrdexExchangesHandler returns the known/registered DEX servers (raw).
func GetDcrdexExchangesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	raw, err := client.Exchanges(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write(raw)
}

// atomsToConv converts an atomic amount to conventional units using the asset's
// conversion factor (from the embedded catalog). Decred's atoms-per-coin is used
// as a fallback for unknown assets.
func atomsToConv(atoms, convFactor uint64) float64 {
	if convFactor == 0 {
		convFactor = uint64(dcrutil.AtomsPerCoin)
	}
	return float64(atoms) / float64(convFactor)
}

// convToAtoms converts a conventional amount to atomic units, rounding to the
// nearest atom.
func convToAtoms(amount float64, convFactor uint64) uint64 {
	if convFactor == 0 {
		convFactor = uint64(dcrutil.AtomsPerCoin)
	}
	return uint64(math.Round(amount * float64(convFactor)))
}

// dexAssetID parses the assetID query parameter, defaulting to Decred when it is
// absent. Only assets whose wallet is a NewAddresser (BTC, LTC, DCR) ever supply
// one; account-based chains reuse a static address and never reach this path.
func dexAssetID(r *http.Request) uint32 {
	v := r.URL.Query().Get("assetID")
	if v == "" {
		return bisonw.AssetDCR
	}
	id, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return bisonw.AssetDCR
	}
	return uint32(id)
}

// NewDexDepositAddressHandler returns a fresh deposit address for the DEX wallet
// of the requested asset so the user can avoid reusing the persisted address. The
// new-address route lives only on bisonw's webserver, so this uses the web
// client; the asset backend manages the address index and returns its next unused
// one.
func NewDexDepositAddressHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	addr, err := client.NewDepositAddress(ctx, appPass, dexAssetID(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"address": addr})
}

// DexAddressUsedHandler reports whether an address has already received funds,
// used to warn against deposit-address reuse on the Wallets page.
func DexAddressUsedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	addr := r.URL.Query().Get("addr")
	if addr == "" {
		http.Error(w, "addr is required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	used, err := client.AddressUsed(ctx, appPass, dexAssetID(r), addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"used": used})
}

// DexWalletState is the funding view of a single DCRDEX-managed wallet. Balances
// are converted to conventional units in the backend.
type DexWalletState struct {
	AssetID      uint32  `json:"assetID"`
	Symbol       string  `json:"symbol"`
	WalletType   string  `json:"walletType"`
	Traits       uint64  `json:"traits"`
	Running      bool    `json:"running"`
	Open         bool    `json:"open"`
	Encrypted    bool    `json:"encrypted"`
	Disabled     bool    `json:"disabled"`
	Synced       bool    `json:"synced"`
	SyncProgress float32 `json:"syncProgress"`
	PeerCount    uint32  `json:"peerCount"`
	Units        string  `json:"units"`
	Address      string  `json:"address"`
	Available    float64 `json:"available"`
	Locked       float64 `json:"locked"`
	Immature     float64 `json:"immature"`
	OrderLocked  float64 `json:"orderLocked"`
	BondLocked   float64 `json:"bondLocked"`
	Total        float64 `json:"total"`
}

// GetDcrdexWalletsHandler returns the DCRDEX-managed wallets and their balances
// (in conventional units), so the Wallets tab can show the funding picture.
func GetDcrdexWalletsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	raw, err := client.Wallets(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var states []struct {
		AssetID      uint32  `json:"assetID"`
		Symbol       string  `json:"symbol"`
		WalletType   string  `json:"type"`
		Traits       uint64  `json:"traits"`
		Running      bool    `json:"running"`
		Open         bool    `json:"open"`
		Encrypted    bool    `json:"encrypted"`
		Disabled     bool    `json:"disabled"`
		Synced       bool    `json:"synced"`
		SyncProgress float32 `json:"syncProgress"`
		PeerCount    uint32  `json:"peerCount"`
		Units        string  `json:"units"`
		Address      string  `json:"address"`
		Balance      *struct {
			Available   uint64 `json:"available"`
			Immature    uint64 `json:"immature"`
			Locked      uint64 `json:"locked"`
			OrderLocked uint64 `json:"orderlocked"`
			BondLocked  uint64 `json:"bondlocked"`
		} `json:"balance"`
	}
	if err := json.Unmarshal(raw, &states); err != nil {
		http.Error(w, "decode wallets: "+err.Error(), http.StatusBadGateway)
		return
	}
	out := make([]DexWalletState, 0, len(states))
	for _, s := range states {
		ws := DexWalletState{
			AssetID:      s.AssetID,
			Symbol:       strings.ToUpper(s.Symbol),
			WalletType:   s.WalletType,
			Traits:       s.Traits,
			Running:      s.Running,
			Open:         s.Open,
			Encrypted:    s.Encrypted,
			Disabled:     s.Disabled,
			Synced:       s.Synced,
			SyncProgress: s.SyncProgress,
			PeerCount:    s.PeerCount,
			Units:        s.Units,
			Address:      s.Address,
		}
		if s.Balance != nil {
			cf := dexassets.ConvFactor(s.AssetID)
			ws.Available = atomsToConv(s.Balance.Available, cf)
			ws.Locked = atomsToConv(s.Balance.Locked, cf)
			ws.Immature = atomsToConv(s.Balance.Immature, cf)
			ws.OrderLocked = atomsToConv(s.Balance.OrderLocked, cf)
			ws.BondLocked = atomsToConv(s.Balance.BondLocked, cf)
			// Total mirrors bisonw's own definition (client/webserver assets.ts):
			// available + locked + immature. OrderLocked is already part of Locked,
			// so it is not added again.
			ws.Total = atomsToConv(s.Balance.Available+s.Balance.Locked+s.Balance.Immature, cf)
		}
		out = append(out, ws)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AssetID < out[j].AssetID })
	json.NewEncoder(w).Encode(out)
}

// DexPendingBond is a bond awaiting confirmations.
type DexPendingBond struct {
	Symbol  string `json:"symbol"`
	AssetID uint32 `json:"assetID"`
	Confs   uint32 `json:"confs"`
}

// DexBondAsset is an asset a DEX accepts for bonds.
type DexBondAsset struct {
	Symbol  string `json:"symbol"`
	AssetID uint32 `json:"assetID"`
}

// DexAccount is the per-server account view (tier, reputation, bonds) for the
// Account tab. Bond amounts are converted to DCR in the backend.
type DexAccount struct {
	Host             string           `json:"host"`
	AcctID           string           `json:"acctID"`
	Registered       bool             `json:"registered"`
	ConnectionStatus int              `json:"connectionStatus"`
	ViewOnly         bool             `json:"viewOnly"`
	Disabled         bool             `json:"disabled"`
	TargetTier       uint64           `json:"targetTier"`
	EffectiveTier    int64            `json:"effectiveTier"`
	BondedTier       int64            `json:"bondedTier"`
	Penalties        uint16           `json:"penalties"`
	Score            int32            `json:"score"`
	PenaltyThreshold uint32           `json:"penaltyThreshold"`
	MaxScore         uint32           `json:"maxScore"`
	BondAssetID        uint32           `json:"bondAssetID"`
	BondExpiryDays     int              `json:"bondExpiryDays"`
	BondPerTierAtoms   uint64           `json:"bondPerTierAtoms"`
	BondPerTierDcr     float64          `json:"bondPerTierDcr"`
	MaxBondedDcr       float64          `json:"maxBondedDcr"`
	PenaltyComps       uint16           `json:"penaltyComps"`
	BondsPendingRefund int              `json:"bondsPendingRefund"`
	BondAssets         []DexBondAsset   `json:"bondAssets"`
	AutoRenew          bool             `json:"autoRenew"`
	PendingBonds       []DexPendingBond `json:"pendingBonds"`
}

// GetDcrdexAccountHandler returns the account state (tier, reputation, bonds)
// for the DEX server given in the `host` query parameter.
func GetDcrdexAccountHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	host := r.URL.Query().Get("host")
	if host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	raw, err := client.Exchanges(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var xcs map[string]struct {
		Host             string `json:"host"`
		AcctID           string `json:"acctID"`
		ConnectionStatus int    `json:"connectionStatus"`
		ViewOnly         bool   `json:"viewOnly"`
		Disabled         bool   `json:"disabled"`
		BondExpiry       uint64 `json:"bondExpiry"`
		PenaltyThreshold uint32 `json:"penaltyThreshold"`
		MaxScore         uint32 `json:"maxScore"`
		BondAssets       map[string]struct {
			ID  uint32 `json:"id"`
			Amt uint64 `json:"amount"`
		} `json:"bondAssets"`
		Auth struct {
			Rep struct {
				BondedTier int64  `json:"bondedTier"`
				Penalties  uint16 `json:"penalties"`
				Score      int32  `json:"score"`
			} `json:"rep"`
			BondAssetID   uint32     `json:"bondAssetID"`
			TargetTier    uint64     `json:"targetTier"`
			EffectiveTier int64      `json:"effectiveTier"`
			MaxBondedAmt  uint64     `json:"maxBondedAmt"`
			PenaltyComps  uint16     `json:"penaltyComps"`
			ExpiredBonds  []struct{} `json:"expiredBonds"`
			PendingBonds  []struct {
				Symbol  string `json:"symbol"`
				AssetID uint32 `json:"assetID"`
				Confs   uint32 `json:"confs"`
			} `json:"pendingBonds"`
		} `json:"auth"`
	}
	if err := json.Unmarshal(raw, &xcs); err != nil {
		http.Error(w, "decode exchanges: "+err.Error(), http.StatusBadGateway)
		return
	}
	xc, ok := xcs[host]
	if !ok {
		http.Error(w, "unknown DEX host", http.StatusNotFound)
		return
	}
	pending := make([]DexPendingBond, 0, len(xc.Auth.PendingBonds))
	for _, b := range xc.Auth.PendingBonds {
		pending = append(pending, DexPendingBond{Symbol: strings.ToUpper(b.Symbol), AssetID: b.AssetID, Confs: b.Confs})
	}
	bondAssets := make([]DexBondAsset, 0, len(xc.BondAssets))
	for sym, ba := range xc.BondAssets {
		bondAssets = append(bondAssets, DexBondAsset{Symbol: strings.ToUpper(sym), AssetID: ba.ID})
	}
	sort.Slice(bondAssets, func(i, j int) bool { return bondAssets[i].Symbol < bondAssets[j].Symbol })
	json.NewEncoder(w).Encode(DexAccount{
		Host:               host,
		AcctID:             xc.AcctID,
		Registered:         xc.AcctID != "",
		ConnectionStatus:   xc.ConnectionStatus,
		ViewOnly:           xc.ViewOnly,
		Disabled:           xc.Disabled,
		TargetTier:         xc.Auth.TargetTier,
		EffectiveTier:      xc.Auth.EffectiveTier,
		BondedTier:         xc.Auth.Rep.BondedTier,
		Penalties:          xc.Auth.Rep.Penalties,
		Score:              xc.Auth.Rep.Score,
		PenaltyThreshold:   xc.PenaltyThreshold,
		MaxScore:           xc.MaxScore,
		BondAssetID:        xc.Auth.BondAssetID,
		BondExpiryDays:     int(xc.BondExpiry / 86400),
		BondPerTierAtoms:   xc.BondAssets["dcr"].Amt,
		BondPerTierDcr:     dcrutil.Amount(xc.BondAssets["dcr"].Amt).ToCoin(),
		MaxBondedDcr:       atomsToConv(xc.Auth.MaxBondedAmt, dexassets.ConvFactor(xc.Auth.BondAssetID)),
		PenaltyComps:       xc.Auth.PenaltyComps,
		BondsPendingRefund: len(xc.Auth.ExpiredBonds),
		BondAssets:         bondAssets,
		AutoRenew:          xc.Auth.TargetTier > 0,
		PendingBonds:       pending,
	})
}

// SetDcrdexBondOptionsHandler updates a DEX account's auto-bond options. Any
// field left nil is unchanged; targetTier 0 disables auto-renewal. maxBondedDcr
// is conventional and converted to atoms here (0 resets to the server default).
// Requires the DEX session to be unlocked.
func SetDcrdexBondOptionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host         string   `json:"host"`
		TargetTier   *int     `json:"targetTier"`
		MaxBondedDcr *float64 `json:"maxBondedDcr"`
		BondAssetID  *int     `json:"bondAssetID"`
		PenaltyComps *int     `json:"penaltyComps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}
	if _, ok := rpc.DcrdexAppPass(); !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// -1 leaves an option unchanged (see bisonw.SetBondOptions).
	targetTier, maxBonded, bondAsset, penaltyComps := -1, -1, -1, -1
	if req.TargetTier != nil {
		targetTier = *req.TargetTier
	}
	if req.BondAssetID != nil {
		bondAsset = *req.BondAssetID
	}
	if req.PenaltyComps != nil {
		penaltyComps = *req.PenaltyComps
	}
	if req.MaxBondedDcr != nil {
		assetID := bisonw.AssetDCR
		if bondAsset >= 0 {
			assetID = uint32(bondAsset)
		}
		maxBonded = int(convToAtoms(*req.MaxBondedDcr, dexassets.ConvFactor(assetID)))
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.SetBondOptions(ctx, req.Host, targetTier, maxBonded, bondAsset, penaltyComps); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// DexConfigResponse is the registration-screen view of a DEX server's config.
// Amounts are converted to DCR in the backend with dcrutil so the frontend does
// not hardcode the atoms-per-coin ratio.
type DexMarket struct {
	Base            string   `json:"base"`
	Quote           string   `json:"quote"`
	BaseID          uint32   `json:"baseID"`
	QuoteID         uint32   `json:"quoteID"`
	LotSize         uint64   `json:"lotSize"`         // atomic
	RateStep        uint64   `json:"rateStep"`        // atomic message-rate
	BaseConvFactor  uint64   `json:"baseConvFactor"`  // base atoms per conventional unit
	QuoteConvFactor uint64   `json:"quoteConvFactor"` // quote atoms per conventional unit
	Spot            *DexSpot `json:"spot,omitempty"`  // last/24h snapshot, when connected
}

// DexSpot is a market's current spot price plus 24h stats, mirroring
// decred.org/dcrdex/dex/msgjson.Spot. Rate/High24/Low24 are atomic message-rates
// and Change24 is a fraction (0.05 == +5%); the frontend converts to display
// units with the market's conversion factors.
type DexSpot struct {
	Rate       uint64  `json:"rate"`
	Change24   float64 `json:"change24"`
	Vol24      uint64  `json:"vol24"`
	High24     uint64  `json:"high24"`
	Low24      uint64  `json:"low24"`
	BookVolume uint64  `json:"bookVolume"`
	Stamp      uint64  `json:"stamp"`
}

type DexConfigResponse struct {
	Host             string      `json:"host"`
	ConnectionStatus int         `json:"connectionStatus"`
	Registered       bool        `json:"registered"`
	BondExpiryDays   int         `json:"bondExpiryDays"`
	BondConfs        uint32      `json:"bondConfs"`
	BondPerTierAtoms uint64      `json:"bondPerTierAtoms"`
	BondPerTierDcr   float64     `json:"bondPerTierDcr"`
	MarketCount      int         `json:"marketCount"`
	Markets          []DexMarket `json:"markets"`
	CandleDurs       []string    `json:"candleDurs"`
}

// dexConfigRaw returns a DEX server's config JSON for host. For a host the
// client is already connected to (registered/known), it returns the cached
// entry from exchanges() so NO new DEX-server connection is opened; only an
// unknown host (the pre-registration preview) triggers a one-shot getdexconfig
// fetch. This mirrors the reference client, which fetches config once and reuses
// its single persistent connection.
func dexConfigRaw(ctx context.Context, client *bisonw.Client, host string) (json.RawMessage, error) {
	if raw, err := client.Exchanges(ctx); err == nil {
		var xcs map[string]json.RawMessage
		if json.Unmarshal(raw, &xcs) == nil {
			if hostRaw, ok := xcs[host]; ok {
				return hostRaw, nil
			}
		}
	}
	return cachedDEXConfig(ctx, client, host)
}

// dexCfgEntry caches one host's getdexconfig result and serializes concurrent
// loads for that host so two registration-screen requests in short order open
// only one DEX-server connection.
type dexCfgEntry struct {
	mu  sync.Mutex
	raw json.RawMessage
	err error
	at  time.Time
}

var (
	dexCfgMu    sync.Mutex
	dexCfgCache = map[string]*dexCfgEntry{}
)

const (
	dexCfgTTL    = 5 * time.Minute  // config (markets, bond reqs) is near-static
	dexCfgErrTTL = 30 * time.Second // brief negative cache so a down/banning server is not hammered
)

// cachedDEXConfig fetches a host's getdexconfig at most once per host per TTL,
// coalescing concurrent callers behind a per-host lock so a burst of requests
// opens only one DEX-server connection.
func cachedDEXConfig(ctx context.Context, client *bisonw.Client, host string) (json.RawMessage, error) {
	dexCfgMu.Lock()
	e := dexCfgCache[host]
	if e == nil {
		e = &dexCfgEntry{}
		dexCfgCache[host] = e
	}
	dexCfgMu.Unlock()

	e.mu.Lock() // serialize loads for this host; a second caller blocks then hits the fresh cache
	defer e.mu.Unlock()
	if !e.at.IsZero() {
		age := time.Since(e.at)
		if e.err == nil && age < dexCfgTTL {
			return e.raw, nil
		}
		if e.err != nil && age < dexCfgErrTTL {
			return nil, e.err
		}
	}
	raw, err := client.GetDEXConfig(ctx, host, "")
	e.raw, e.err, e.at = raw, err, time.Now()
	return raw, err
}

// GetDcrdexConfigHandler fetches a DEX server's public configuration (markets,
// bond requirements) for the host given in the `host` query parameter, with the
// DCR bond amount converted to coins. Read-only; populates the registration screen.
func GetDcrdexConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	host := r.URL.Query().Get("host")
	if host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	// Allow the DEX server ample time to answer the one-shot getdexconfig
	// (slow/cold servers can take a while); the result is cached so this long
	// wait happens at most once per host per TTL.
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	raw, err := dexConfigRaw(ctx, client, host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	var xc struct {
		Host             string `json:"host"`
		AcctID           string `json:"acctID"`
		ConnectionStatus int      `json:"connectionStatus"`
		BondExpiry       uint64   `json:"bondExpiry"`
		BinSizes         []string `json:"binSizes"`
		CandleDurs       []string `json:"candleDurs"`
		BondAssets       map[string]struct {
			Confs uint32 `json:"confs"`
			Amt   uint64 `json:"amount"`
		} `json:"bondAssets"`
		Markets map[string]struct {
			BaseSymbol  string   `json:"basesymbol"`
			QuoteSymbol string   `json:"quotesymbol"`
			BaseID      uint32   `json:"baseid"`
			QuoteID     uint32   `json:"quoteid"`
			LotSize     uint64   `json:"lotsize"`
			RateStep    uint64   `json:"ratestep"`
			Spot        *DexSpot `json:"spot"`
		} `json:"markets"`
		Assets map[string]struct {
			UnitInfo struct {
				Conventional struct {
					ConversionFactor uint64 `json:"conversionFactor"`
				} `json:"conventional"`
			} `json:"unitInfo"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &xc); err != nil {
		http.Error(w, "decode dex config: "+err.Error(), http.StatusBadGateway)
		return
	}
	convFactor := func(assetID uint32) uint64 {
		return xc.Assets[itoa(assetID)].UnitInfo.Conventional.ConversionFactor
	}
	dcr := xc.BondAssets["dcr"]
	markets := make([]DexMarket, 0, len(xc.Markets))
	for _, m := range xc.Markets {
		markets = append(markets, DexMarket{
			Base:            strings.ToUpper(m.BaseSymbol),
			Quote:           strings.ToUpper(m.QuoteSymbol),
			BaseID:          m.BaseID,
			QuoteID:         m.QuoteID,
			LotSize:         m.LotSize,
			RateStep:        m.RateStep,
			BaseConvFactor:  convFactor(m.BaseID),
			QuoteConvFactor: convFactor(m.QuoteID),
			Spot:            m.Spot,
		})
	}
	sort.Slice(markets, func(i, j int) bool {
		if markets[i].Base != markets[j].Base {
			return markets[i].Base < markets[j].Base
		}
		return markets[i].Quote < markets[j].Quote
	})
	// getdexconfig reports bin sizes as "binSizes"; the cached exchanges() entry
	// uses "candleDurs".
	durs := xc.CandleDurs
	if len(durs) == 0 {
		durs = xc.BinSizes
	}
	cfgHost := xc.Host
	if cfgHost == "" {
		cfgHost = host
	}
	json.NewEncoder(w).Encode(DexConfigResponse{
		Host:             cfgHost,
		ConnectionStatus: xc.ConnectionStatus,
		Registered:       xc.AcctID != "",
		BondExpiryDays:   int(xc.BondExpiry / 86400),
		BondConfs:        dcr.Confs,
		BondPerTierAtoms: dcr.Amt,
		BondPerTierDcr:   dcrutil.Amount(dcr.Amt).ToCoin(),
		MarketCount:      len(markets),
		Markets:          markets,
		CandleDurs:       durs,
	})
}

// PostDcrdexBondHandler posts a fidelity bond to register/maintain a DEX
// account. This spends real funds on mainnet; the dashboard only calls it on
// explicit user action.
func PostDcrdexBondHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host         string `json:"host"`
		Bond         uint64 `json:"bond"`
		MaintainTier *bool  `json:"maintainTier,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" || req.Bond == 0 {
		http.Error(w, "host and bond are required", http.StatusBadRequest)
		return
	}
	appPass, ok := rpc.DcrdexAppPass()
	if !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	raw, err := client.PostBond(ctx, bisonw.PostBondParams{
		AppPass:      appPass,
		Host:         req.Host,
		Bond:         req.Bond,
		MaintainTier: req.MaintainTier,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write(raw)
}

// DcrdexWSHandler is a transparent WebSocket relay between the browser and
// bisonw's RPC server /ws endpoint. The frontend speaks bisonw's msgjson
// protocol (loadmarket, loadcandles, ...) and receives the order book feed; the
// dashboard supplies the pinned TLS + auth to bisonw. The RPC server does not
// stream notifications; those are relayed separately by DcrdexNotifyWSHandler.
func DcrdexWSHandler(w http.ResponseWriter, r *http.Request) {
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	relayBisonwWS(w, r, client)
}

// DcrdexNotifyWSHandler relays bisonw's notification feed. The feed is broadcast
// only over the webserver's /ws endpoint (the RPC server never streams it), so
// this is the one place the dashboard uses the webserver socket: a bare
// connection receives the global `notify` push without subscribing to a market.
func DcrdexNotifyWSHandler(w http.ResponseWriter, r *http.Request) {
	client, err := rpc.DcrdexWSClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	relayBisonwWS(w, r, client)
}

// relayBisonwWS upgrades the browser connection and pipes it bidirectionally to
// the given bisonw client's /ws endpoint, supplying the pinned TLS + auth.
func relayBisonwWS(w http.ResponseWriter, r *http.Request, client *bisonw.Client) {
	tlsConfig, wsURL, basicAuth := client.WSDialInfo()

	upgrader := websocket.Upgrader{CheckOrigin: middleware.SameOriginWS}
	front, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("dcrdex ws upgrade: %v", err)
		return
	}
	defer front.Close()

	dialer := &websocket.Dialer{TLSClientConfig: tlsConfig, HandshakeTimeout: 15 * time.Second}
	up, resp, err := dialer.Dial(wsURL, http.Header{"Authorization": {basicAuth}})
	if err != nil {
		msg := "dcrdex ws: " + err.Error()
		if resp != nil {
			msg += " (http " + itoa(uint32(resp.StatusCode)) + ")"
		}
		front.WriteJSON(map[string]string{"error": msg})
		return
	}
	defer up.Close()

	errc := make(chan struct{}, 2)
	pipe := func(dst, src *websocket.Conn) {
		for {
			mt, data, err := src.ReadMessage()
			if err != nil {
				errc <- struct{}{}
				return
			}
			if err := dst.WriteMessage(mt, data); err != nil {
				errc <- struct{}{}
				return
			}
		}
	}
	go pipe(front, up) // bisonw -> browser
	go pipe(up, front) // browser -> bisonw
	<-errc
}

// GetDcrdexMyOrdersHandler returns the user's active and recent orders (raw),
// optionally filtered to the `host` query parameter.
func GetDcrdexMyOrdersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	raw, err := client.MyOrders(ctx, r.URL.Query().Get("host"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write(raw)
}

// bwCoin/bwMatch/bwOrder mirror the core.Order JSON the webserver /api/orders
// route returns (numeric enums, qty/market field names), for normalizing into
// the string-shaped myorders JSON the dashboard already consumes.
type bwConfs struct {
	Count    int64 `json:"count"`
	Required int64 `json:"required"`
}
type bwCoin struct {
	StringID string   `json:"stringID"`
	AssetID  uint32   `json:"assetID"`
	Confs    *bwConfs `json:"confs"`
}
type bwMatch struct {
	MatchID       string  `json:"matchID"`
	Status        uint16  `json:"status"`
	Revoked       bool    `json:"revoked"`
	Rate          uint64  `json:"rate"`
	Qty           uint64  `json:"qty"`
	Side          uint8   `json:"side"`
	FeeRate       uint64  `json:"feeRate"`
	Swap          *bwCoin `json:"swap"`
	CounterSwap   *bwCoin `json:"counterSwap"`
	Redeem        *bwCoin `json:"redeem"`
	CounterRedeem *bwCoin `json:"counterRedeem"`
	Refund        *bwCoin `json:"refund"`
	Stamp         uint64  `json:"stamp"`
	IsCancel      bool    `json:"isCancel"`
}
type bwOrder struct {
	Host        string     `json:"host"`
	MarketID    string     `json:"market"`
	BaseID      uint32     `json:"baseID"`
	QuoteID     uint32     `json:"quoteID"`
	ID          string     `json:"id"`
	Type        uint8      `json:"type"`
	Sell        bool       `json:"sell"`
	Stamp       uint64     `json:"stamp"`
	SubmitTime  uint64     `json:"submitTime"`
	Rate        uint64     `json:"rate"`
	Qty         uint64     `json:"qty"`
	Filled      uint64     `json:"filled"`
	Status      uint16     `json:"status"`
	Cancelling  bool       `json:"cancelling"`
	Canceled    bool       `json:"canceled"`
	TimeInForce uint8      `json:"tif"`
	Matches     []*bwMatch `json:"matches"`
}

// dexHistMatch/dexHistOrder mirror the bisonw myorders JSON (string enums) the
// dashboard DexOrder/DexMatch types consume.
type dexHistMatch struct {
	MatchID       string `json:"matchID"`
	Status        string `json:"status"`
	Revoked       bool   `json:"revoked"`
	Rate          uint64 `json:"rate"`
	Qty           uint64 `json:"qty"`
	Side          string `json:"side"`
	FeeRate       uint64 `json:"feeRate"`
	Swap          string `json:"swap,omitempty"`
	CounterSwap   string `json:"counterSwap,omitempty"`
	Redeem        string `json:"redeem,omitempty"`
	CounterRedeem string `json:"counterRedeem,omitempty"`
	Refund        string `json:"refund,omitempty"`
	Stamp         uint64 `json:"stamp"`
	IsCancel      bool   `json:"isCancel"`
}
type dexHistOrder struct {
	Host        string          `json:"host"`
	MarketName  string          `json:"marketName"`
	BaseID      uint32          `json:"baseID"`
	QuoteID     uint32          `json:"quoteID"`
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Sell        bool            `json:"sell"`
	Stamp       uint64          `json:"stamp"`
	SubmitTime  uint64          `json:"submitTime"`
	Rate        uint64          `json:"rate,omitempty"`
	Quantity    uint64          `json:"quantity"`
	Filled      uint64          `json:"filled"`
	Settled     uint64          `json:"settled"`
	Status      string          `json:"status"`
	Cancelling  bool            `json:"cancelling,omitempty"`
	Canceled    bool            `json:"canceled,omitempty"`
	TimeInForce string          `json:"tif,omitempty"`
	Matches     []*dexHistMatch `json:"matches,omitempty"`
}

// Enum -> string maps mirroring dcrdex's order/match String() methods
// (dex/order status.go, order.go, match.go), so the normalized history matches
// the myorders shape exactly.
var (
	dexOrderTypeNames   = map[uint8]string{1: "limit", 2: "market", 3: "cancel"}
	dexOrderStatusNames = map[uint16]string{0: "unknown", 1: "epoch", 2: "booked", 3: "executed", 4: "canceled", 5: "revoked"}
	dexStatusByName     = map[string]uint16{"unknown": 0, "epoch": 1, "booked": 2, "executed": 3, "canceled": 4, "revoked": 5}
	dexTifNames         = map[uint8]string{0: "immediate", 1: "standing"}
	dexMatchStatusNames = map[uint16]string{0: "NewlyMatched", 1: "MakerSwapCast", 2: "TakerSwapCast", 3: "MakerRedeemed", 4: "MatchComplete", 5: "MatchConfirmed"}
	dexMatchSideNames   = map[uint8]string{0: "Maker", 1: "Taker"}
)

func bwCoinString(c *bwCoin) string {
	if c == nil {
		return ""
	}
	return c.StringID
}

// normalizeDexHistOrder maps a webserver core.Order into the myorders shape,
// mirroring rpcserver parseCoreOrder/parseMatches (settled = matched value past
// the redeem step; cancelling cleared once executed).
func normalizeDexHistOrder(o *bwOrder) *dexHistOrder {
	matches := make([]*dexHistMatch, 0, len(o.Matches))
	var settled uint64
	for _, m := range o.Matches {
		if (m.Side == 0 && m.Status >= 3) || (m.Side != 0 && m.Status >= 4) {
			settled += m.Qty
		}
		matches = append(matches, &dexHistMatch{
			MatchID:       m.MatchID,
			Status:        dexMatchStatusNames[m.Status],
			Revoked:       m.Revoked,
			Rate:          m.Rate,
			Qty:           m.Qty,
			Side:          dexMatchSideNames[m.Side],
			FeeRate:       m.FeeRate,
			Swap:          bwCoinString(m.Swap),
			CounterSwap:   bwCoinString(m.CounterSwap),
			Redeem:        bwCoinString(m.Redeem),
			CounterRedeem: bwCoinString(m.CounterRedeem),
			Refund:        bwCoinString(m.Refund),
			Stamp:         m.Stamp,
			IsCancel:      m.IsCancel,
		})
	}
	cancelling := o.Cancelling
	if o.Status >= 3 {
		cancelling = false
	}
	return &dexHistOrder{
		Host:        o.Host,
		MarketName:  o.MarketID,
		BaseID:      o.BaseID,
		QuoteID:     o.QuoteID,
		ID:          o.ID,
		Type:        dexOrderTypeNames[o.Type],
		Sell:        o.Sell,
		Stamp:       o.Stamp,
		SubmitTime:  o.SubmitTime,
		Rate:        o.Rate,
		Quantity:    o.Qty,
		Filled:      o.Filled,
		Settled:     settled,
		Status:      dexOrderStatusNames[o.Status],
		Cancelling:  cancelling,
		Canceled:    o.Canceled,
		TimeInForce: dexTifNames[o.TimeInForce],
		Matches:     matches,
	}
}

// GetDcrdexOrdersHandler returns the user's full order history, including
// canceled/executed/revoked orders, from the webserver /api/orders archive route
// (the RPC myorders route returns only active + recently-tracked orders). It
// supports a status filter, a market filter, and offset-based pagination, and
// normalizes the result to the myorders shape the dashboard consumes.
func GetDcrdexOrdersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host   string `json:"host"`
		N      int    `json:"n"`
		Offset string `json:"offset"`
		Status string `json:"status"`
		Market *struct {
			BaseID  uint32 `json:"baseID"`
			QuoteID uint32 `json:"quoteID"`
		} `json:"market"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	n := req.N
	if n <= 0 {
		n = 50
	}
	filter := map[string]any{"n": n}
	if req.Host != "" {
		filter["hosts"] = []string{req.Host}
	}
	if req.Offset != "" {
		filter["offset"] = req.Offset
	}
	if s, ok := dexStatusByName[req.Status]; ok {
		filter["statuses"] = []uint16{s}
	}
	if req.Market != nil {
		filter["market"] = map[string]any{"baseID": req.Market.BaseID, "quoteID": req.Market.QuoteID}
	}

	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	raw, err := client.Orders(ctx, appPass, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var in []*bwOrder
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			http.Error(w, "decode orders: "+err.Error(), http.StatusBadGateway)
			return
		}
	}
	out := make([]*dexHistOrder, 0, len(in))
	for _, o := range in {
		out = append(out, normalizeDexHistOrder(o))
	}
	json.NewEncoder(w).Encode(out)
}

// dexFullCoin/dexFullMatch/dexFullOrder mirror the myorders shape but keep each
// swap coin as an object with its asset and live confirmation counts, which the
// single-order route provides (for active orders) and the order-detail swap
// tracker needs.
type dexFullCoin struct {
	StringID string   `json:"stringID"`
	AssetID  uint32   `json:"assetID"`
	Confs    *bwConfs `json:"confs,omitempty"`
}
type dexFullMatch struct {
	MatchID       string       `json:"matchID"`
	Status        string       `json:"status"`
	Revoked       bool         `json:"revoked"`
	Rate          uint64       `json:"rate"`
	Qty           uint64       `json:"qty"`
	Side          string       `json:"side"`
	FeeRate       uint64       `json:"feeRate"`
	Swap          *dexFullCoin `json:"swap,omitempty"`
	CounterSwap   *dexFullCoin `json:"counterSwap,omitempty"`
	Redeem        *dexFullCoin `json:"redeem,omitempty"`
	CounterRedeem *dexFullCoin `json:"counterRedeem,omitempty"`
	Refund        *dexFullCoin `json:"refund,omitempty"`
	Stamp         uint64       `json:"stamp"`
	IsCancel      bool         `json:"isCancel"`
}
type dexFullOrder struct {
	Host        string          `json:"host"`
	MarketName  string          `json:"marketName"`
	BaseID      uint32          `json:"baseID"`
	QuoteID     uint32          `json:"quoteID"`
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Sell        bool            `json:"sell"`
	Stamp       uint64          `json:"stamp"`
	SubmitTime  uint64          `json:"submitTime"`
	Rate        uint64          `json:"rate,omitempty"`
	Quantity    uint64          `json:"quantity"`
	Filled      uint64          `json:"filled"`
	Settled     uint64          `json:"settled"`
	Status      string          `json:"status"`
	Cancelling  bool            `json:"cancelling,omitempty"`
	Canceled    bool            `json:"canceled,omitempty"`
	TimeInForce string          `json:"tif,omitempty"`
	Matches     []*dexFullMatch `json:"matches,omitempty"`
}

func fullCoin(c *bwCoin) *dexFullCoin {
	if c == nil || c.StringID == "" {
		return nil
	}
	return &dexFullCoin{StringID: c.StringID, AssetID: c.AssetID, Confs: c.Confs}
}

// normalizeDexFullOrder maps a webserver core.Order into the rich (confs-bearing)
// shape, mirroring rpcserver parseCoreOrder/parseMatches but preserving each
// coin's asset and confirmation counts.
func normalizeDexFullOrder(o *bwOrder) *dexFullOrder {
	matches := make([]*dexFullMatch, 0, len(o.Matches))
	var settled uint64
	for _, m := range o.Matches {
		if (m.Side == 0 && m.Status >= 3) || (m.Side != 0 && m.Status >= 4) {
			settled += m.Qty
		}
		matches = append(matches, &dexFullMatch{
			MatchID:       m.MatchID,
			Status:        dexMatchStatusNames[m.Status],
			Revoked:       m.Revoked,
			Rate:          m.Rate,
			Qty:           m.Qty,
			Side:          dexMatchSideNames[m.Side],
			FeeRate:       m.FeeRate,
			Swap:          fullCoin(m.Swap),
			CounterSwap:   fullCoin(m.CounterSwap),
			Redeem:        fullCoin(m.Redeem),
			CounterRedeem: fullCoin(m.CounterRedeem),
			Refund:        fullCoin(m.Refund),
			Stamp:         m.Stamp,
			IsCancel:      m.IsCancel,
		})
	}
	cancelling := o.Cancelling
	if o.Status >= 3 {
		cancelling = false
	}
	return &dexFullOrder{
		Host:        o.Host,
		MarketName:  o.MarketID,
		BaseID:      o.BaseID,
		QuoteID:     o.QuoteID,
		ID:          o.ID,
		Type:        dexOrderTypeNames[o.Type],
		Sell:        o.Sell,
		Stamp:       o.Stamp,
		SubmitTime:  o.SubmitTime,
		Rate:        o.Rate,
		Quantity:    o.Qty,
		Filled:      o.Filled,
		Settled:     settled,
		Status:      dexOrderStatusNames[o.Status],
		Cancelling:  cancelling,
		Canceled:    o.Canceled,
		TimeInForce: dexTifNames[o.TimeInForce],
		Matches:     matches,
	}
}

// GetDcrdexSingleOrderHandler returns one order with live swap-coin confirmation
// counts, from the webserver /api/order route (core.Order). The RPC myorders feed
// and the orders archive omit confs; this is the only source of live confs, used
// by the order-detail swap tracker.
func GetDcrdexSingleOrderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	raw, err := client.Order(ctx, appPass, req.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var o bwOrder
	if err := json.Unmarshal(raw, &o); err != nil {
		http.Error(w, "decode order: "+err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(normalizeDexFullOrder(&o))
}

// CancelDcrdexOrderHandler cancels an active order by its hex order ID.
func CancelDcrdexOrderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		OrderID string `json:"orderID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OrderID == "" {
		http.Error(w, "orderID is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.Cancel(ctx, req.OrderID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// PlaceDcrdexOrderHandler places a limit or market order. Qty and Rate are in
// atomic units. This spends real funds on mainnet; the dashboard calls it only
// on explicit user action.
func PlaceDcrdexOrderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host    string            `json:"host"`
		IsLimit bool              `json:"isLimit"`
		Sell    bool              `json:"sell"`
		Base    uint32            `json:"base"`
		Quote   uint32            `json:"quote"`
		Qty     uint64            `json:"qty"`
		Rate    uint64            `json:"rate"`
		TifNow  bool              `json:"tifNow"`
		Options map[string]string `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" || req.Qty == 0 {
		http.Error(w, "host and qty are required", http.StatusBadRequest)
		return
	}
	appPass, ok := rpc.DcrdexAppPass()
	if !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	opts := make(map[string]any, len(req.Options))
	for k, v := range req.Options {
		opts[k] = v
	}
	raw, err := client.Trade(ctx, bisonw.TradeParams{
		AppPass: appPass,
		Host:    req.Host,
		IsLimit: req.IsLimit,
		Sell:    req.Sell,
		Base:    req.Base,
		Quote:   req.Quote,
		Qty:     req.Qty,
		Rate:    req.Rate,
		TifNow:  req.TifNow,
		Options: opts,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write(raw)
}

// PreDcrdexOrderHandler returns bisonw's pre-order estimate (swap + redeem fee
// estimates and the per-asset order options) for a prospective order, so the
// order form can show fees and options before the user commits. Read-only; goes
// through the webserver, which the RPC server has no equivalent for.
func PreDcrdexOrderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host    string            `json:"host"`
		IsLimit bool              `json:"isLimit"`
		Sell    bool              `json:"sell"`
		Base    uint32            `json:"base"`
		Quote   uint32            `json:"quote"`
		Qty     uint64            `json:"qty"`
		Rate    uint64            `json:"rate"`
		TifNow  bool              `json:"tifNow"`
		Options map[string]string `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" || req.Qty == 0 {
		http.Error(w, "host and qty are required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	raw, err := client.PreOrder(ctx, appPass, req.Host, req.IsLimit, req.Sell, req.Base, req.Quote, req.Qty, req.Rate, req.TifNow, req.Options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write(raw)
}

// MaxDcrdexBuyHandler returns the largest buy order fundable at the given rate on
// the market, with fee estimates. Webserver-only route.
func MaxDcrdexBuyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host  string `json:"host"`
		Base  uint32 `json:"base"`
		Quote uint32 `json:"quote"`
		Rate  uint64 `json:"rate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" || req.Rate == 0 {
		http.Error(w, "host and rate are required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	raw, err := client.MaxBuy(ctx, appPass, req.Host, req.Base, req.Quote, req.Rate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write(raw)
}

// MaxDcrdexSellHandler returns the largest sell order fundable on the market,
// with fee estimates. Webserver-only route.
func MaxDcrdexSellHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host  string `json:"host"`
		Base  uint32 `json:"base"`
		Quote uint32 `json:"quote"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	raw, err := client.MaxSell(ctx, appPass, req.Host, req.Base, req.Quote)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write(raw)
}

// GetDcrdexAssetsHandler serves the embedded DCRDEX supported-asset catalog
// (wallet definitions and config-option schemas), which the bisonw RPC does not
// expose. Used by the frontend to drive the create-wallet forms.
func GetDcrdexAssetsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(dexassets.Raw())
}

// CreateDcrdexAssetWalletHandler creates a wallet for an arbitrary asset from a
// schema-driven config map. The DCR onboarding wallet has its own dedicated
// handler (CreateDcrdexWalletHandler) that wires the pinned dex account; this
// generic path serves every other asset. Requires the DEX session unlocked.
func CreateDcrdexAssetWalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		AssetID    uint32            `json:"assetID"`
		WalletType string            `json:"walletType"`
		Config     map[string]string `json:"config"`
		WalletPass string            `json:"walletPass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WalletType == "" {
		http.Error(w, "assetID and walletType are required", http.StatusBadRequest)
		return
	}
	appPass, ok := rpc.DcrdexAppPass()
	if !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.NewWallet(ctx, bisonw.NewWalletParams{
		AppPass:    appPass,
		WalletPass: req.WalletPass,
		AssetID:    req.AssetID,
		WalletType: req.WalletType,
		Config:     req.Config,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// DexWalletTx is a wallet transaction with amounts converted to conventional
// units in the backend.
type DexWalletTx struct {
	Type        uint32  `json:"type"`
	ID          string  `json:"id"`
	Amount      float64 `json:"amount"`
	Fees        float64 `json:"fees"`
	BlockNumber uint64  `json:"blockNumber"`
	Timestamp   uint64  `json:"timestamp"`
	Recipient   string  `json:"recipient,omitempty"`
	TokenID     *uint32 `json:"tokenID,omitempty"`
}

type rawWalletTx struct {
	Type        uint32  `json:"type"`
	ID          string  `json:"id"`
	Amount      uint64  `json:"amount"`
	Fees        uint64  `json:"fees"`
	BlockNumber uint64  `json:"blockNumber"`
	Timestamp   uint64  `json:"timestamp"`
	TokenID     *uint32 `json:"tokenID"`
	Recipient   *string `json:"recipient"`
}

func convWalletTx(assetID uint32, t rawWalletTx) DexWalletTx {
	amtFactor := dexassets.ConvFactor(assetID)
	if t.TokenID != nil {
		amtFactor = dexassets.ConvFactor(*t.TokenID)
	}
	out := DexWalletTx{
		Type:        t.Type,
		ID:          t.ID,
		Amount:      atomsToConv(t.Amount, amtFactor),
		Fees:        atomsToConv(t.Fees, dexassets.ConvFactor(assetID)),
		BlockNumber: t.BlockNumber,
		Timestamp:   t.Timestamp,
		TokenID:     t.TokenID,
	}
	if t.Recipient != nil {
		out.Recipient = *t.Recipient
	}
	return out
}

// pendingDexTx builds a pending (mempool) wallet transaction for the dcrwallet
// "dex" account from a gRPC transaction detail, classifying it as a receive or
// send by the net change to the account. bisonw's txhistory only records
// external receives once they are mined, so these surface incoming deposits (and
// outgoing sends) that are still in the mempool.
func pendingDexTx(assetID uint32, t *pb.TransactionDetails, txid string, credit, debit int64) DexWalletTx {
	conv := dexassets.ConvFactor(assetID)
	var typ uint32
	var amtAtoms, feeAtoms uint64
	if credit >= debit {
		typ = 2 // Receive
		amtAtoms = uint64(credit - debit)
	} else {
		typ = 1 // Send
		fee := t.GetFee()
		feeAtoms = uint64(fee)
		spent := debit - credit // amount sent + fee
		if spent > fee {
			amtAtoms = uint64(spent - fee)
		} else {
			amtAtoms = uint64(spent)
		}
	}
	return DexWalletTx{
		Type:        typ,
		ID:          txid,
		Amount:      atomsToConv(amtAtoms, conv),
		Fees:        atomsToConv(feeAtoms, conv),
		BlockNumber: 0,
		Timestamp:   uint64(t.GetTimestamp()),
	}
}

// dexTxIDs caches the set of txids belonging to the dcrwallet "dex" account, used
// to scope the DEX Decred wallet history (bisonw's txhistory is wallet-wide and
// carries no account field), plus the account's current unmined txs so mempool
// deposits surface before bisonw records them. Refreshed on first-page loads and
// after a short TTL.
var (
	dexTxIDMu     sync.Mutex
	dexTxIDSet    map[string]struct{}
	dexUnminedTxs []DexWalletTx
	dexTxIDAt     time.Time
)

const dexTxIDTTL = 30 * time.Second

// dexAccountTxIDs returns the txids in the dcrwallet "dex" account via
// listtransactions (account-scoped). fresh forces a recompute (used on first-page
// loads so new deposits appear); otherwise a cached set is reused across "Load
// more" pagination.
func dexAccountTxIDs(ctx context.Context, fresh bool) (map[string]struct{}, []DexWalletTx, error) {
	dexTxIDMu.Lock()
	defer dexTxIDMu.Unlock()
	if !fresh && dexTxIDSet != nil && time.Since(dexTxIDAt) < dexTxIDTTL {
		return dexTxIDSet, dexUnminedTxs, nil
	}
	if rpc.WalletGrpcClient == nil {
		return nil, nil, fmt.Errorf("wallet gRPC client not initialized")
	}
	// Resolve the dcrwallet "dex" account number. (listtransactions cannot filter
	// by account, and its per-entry account tag is unreliable for sends to
	// non-wallet scripts like fidelity bonds, so use GetTransactions instead.)
	accts, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{})
	if err != nil {
		return nil, nil, err
	}
	var dexAcct uint32
	found := false
	for _, a := range accts.GetAccounts() {
		if a.GetAccountName() == dexAccountName {
			dexAcct = a.GetAccountNumber()
			found = true
			break
		}
	}
	if !found {
		set := map[string]struct{}{}
		dexTxIDSet, dexUnminedTxs, dexTxIDAt = set, nil, time.Now()
		return set, nil, nil
	}

	// Stream the wallet's transactions (only the wallet's own, across all
	// accounts) and keep those that credit or spend the dex account. Per-tx
	// Credits[].Account / Debits[].PreviousAccount reliably attribute bond
	// posts, swaps and deposits to the dex account; the net of the two also
	// sizes the unmined txs for the pending rows.
	stream, err := rpc.WalletGrpcClient.GetTransactions(ctx, &pb.GetTransactionsRequest{
		StartingBlockHeight: 0,
		EndingBlockHeight:   -1,
	})
	if err != nil {
		return nil, nil, err
	}
	set := map[string]struct{}{}
	var pending []DexWalletTx
	add := func(details []*pb.TransactionDetails, unmined bool) {
		for _, t := range details {
			var credit, debit int64
			touches := false
			for _, c := range t.GetCredits() {
				if c.GetAccount() == dexAcct {
					credit += c.GetAmount()
					touches = true
				}
			}
			for _, d := range t.GetDebits() {
				if d.GetPreviousAccount() == dexAcct {
					debit += d.GetPreviousAmount()
					touches = true
				}
			}
			if !touches {
				continue
			}
			h, herr := chainhash.NewHash(t.GetHash())
			if herr != nil {
				continue
			}
			txid := h.String()
			set[txid] = struct{}{}
			if unmined {
				pending = append(pending, pendingDexTx(bisonw.AssetDCR, t, txid, credit, debit))
			}
		}
	}
	for {
		resp, rerr := stream.Recv()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, nil, rerr
		}
		if mined := resp.GetMinedTransactions(); mined != nil {
			add(mined.GetTransactions(), false)
		}
		add(resp.GetUnminedTransactions(), true)
	}
	dexTxIDSet, dexUnminedTxs, dexTxIDAt = set, pending, time.Now()
	return set, pending, nil
}

// GetDcrdexWalletTxsHandler returns a wallet's transaction history (amounts in
// conventional units). Query: assetID (required), n, refID, past. For the Decred
// wallet the history is scoped to the dcrwallet "dex" account.
func GetDcrdexWalletTxsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	assetID, err := strconv.ParseUint(r.URL.Query().Get("assetID"), 10, 32)
	if err != nil {
		http.Error(w, "assetID is required", http.StatusBadRequest)
		return
	}
	num, _ := strconv.Atoi(r.URL.Query().Get("n"))
	refID := r.URL.Query().Get("refID")
	past := r.URL.Query().Get("past") == "true"
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if uint32(assetID) != bisonw.AssetDCR {
		raw, err := client.TxHistory(ctx, uint32(assetID), num, refID, past)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		var txs []rawWalletTx
		if err := json.Unmarshal(raw, &txs); err != nil {
			http.Error(w, "decode txs: "+err.Error(), http.StatusBadGateway)
			return
		}
		out := make([]DexWalletTx, 0, len(txs))
		for _, t := range txs {
			out = append(out, convWalletTx(uint32(assetID), t))
		}
		json.NewEncoder(w).Encode(out)
		return
	}

	// Decred: keep only txs belonging to the "dex" account. bisonw returns the
	// whole wallet's history with no account field, so accumulate across bisonw
	// pages until we have a full page of dex-account txs (the frontend treats a
	// short page as "no more"). refID == "" means a first-page load/reload.
	want := num
	if want <= 0 {
		want = 25
	}
	dexSet, dexPending, err := dexAccountTxIDs(ctx, refID == "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	cursor, cursorPast := refID, past
	out := make([]DexWalletTx, 0, want)
	const maxPages = 20
	for i := 0; i < maxPages && len(out) < want; i++ {
		raw, err := client.TxHistory(ctx, uint32(assetID), want, cursor, cursorPast)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		var txs []rawWalletTx
		if err := json.Unmarshal(raw, &txs); err != nil {
			http.Error(w, "decode txs: "+err.Error(), http.StatusBadGateway)
			return
		}
		if len(txs) == 0 {
			break
		}
		for _, t := range txs {
			if _, ok := dexSet[t.ID]; ok {
				out = append(out, convWalletTx(uint32(assetID), t))
			}
		}
		if len(txs) < want {
			break // bisonw history exhausted
		}
		cursor, cursorPast = txs[len(txs)-1].ID, true
	}
	// On a first-page load, prepend any unmined dex-account txs that bisonw's
	// history does not have yet (an incoming deposit still in the mempool) so they
	// show as pending, newest first.
	if refID == "" && len(dexPending) > 0 {
		have := make(map[string]bool, len(out))
		for _, t := range out {
			have[t.ID] = true
		}
		pend := make([]DexWalletTx, 0, len(dexPending))
		for _, u := range dexPending {
			if !have[u.ID] {
				pend = append(pend, u)
			}
		}
		sort.Slice(pend, func(i, j int) bool { return pend[i].Timestamp > pend[j].Timestamp })
		out = append(pend, out...)
	}
	if len(out) > want {
		out = out[:want]
	}
	json.NewEncoder(w).Encode(out)
}

// GetDcrdexWalletTxHandler returns a single wallet transaction. Query: assetID,
// txID.
func GetDcrdexWalletTxHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	assetID, err := strconv.ParseUint(r.URL.Query().Get("assetID"), 10, 32)
	if err != nil {
		http.Error(w, "assetID is required", http.StatusBadRequest)
		return
	}
	txID := r.URL.Query().Get("txID")
	if txID == "" {
		http.Error(w, "txID is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	raw, err := client.WalletTx(ctx, uint32(assetID), txID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var t rawWalletTx
	if err := json.Unmarshal(raw, &t); err != nil {
		http.Error(w, "decode tx: "+err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(convWalletTx(uint32(assetID), t))
}

// SendDcrdexWalletHandler sends a conventional amount of an asset to an address.
// The amount is converted to atoms in the backend. Spends real funds; requires
// the DEX session unlocked.
func SendDcrdexWalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		AssetID uint32  `json:"assetID"`
		Value   float64 `json:"value"`
		Address string  `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" || req.Value <= 0 {
		http.Error(w, "assetID, value and address are required", http.StatusBadRequest)
		return
	}
	appPass, ok := rpc.DcrdexAppPass()
	if !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	atoms := convToAtoms(req.Value, dexassets.ConvFactor(req.AssetID))
	coin, err := client.Send(ctx, appPass, req.AssetID, atoms, req.Address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"coin": coin})
}

// EstimateDcrdexSendFeeHandler estimates the network fee to send from a wallet and
// validates the address, over the bisonw webserver (the RPC server does not expose
// fee estimation). The fee is returned in conventional units of the fee asset (the
// parent chain for a token), with that asset's symbol.
func EstimateDcrdexSendFeeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		AssetID  uint32  `json:"assetID"`
		Value    float64 `json:"value"`
		Address  string  `json:"address"`
		Subtract bool    `json:"subtract"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" || req.Value <= 0 {
		http.Error(w, "assetID, value and address are required", http.StatusBadRequest)
		return
	}
	appPass, ok := rpc.DcrdexAppPass()
	if !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return
	}
	client, err := rpc.DcrdexWebClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	atoms := convToAtoms(req.Value, dexassets.ConvFactor(req.AssetID))
	txFee, validAddr, err := client.EstimateSendTxFee(ctx, appPass, req.AssetID, req.Address, atoms, req.Subtract, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	feeAsset := dexassets.FeeAsset(req.AssetID)
	json.NewEncoder(w).Encode(map[string]any{
		"fee":          atomsToConv(txFee, dexassets.ConvFactor(feeAsset)),
		"feeSymbol":    dexassets.Symbol(feeAsset),
		"validAddress": validAddr,
	})
}

// dexWalletActionAsset decodes a {assetID} body for simple wallet actions.
func dexWalletActionAsset(r *http.Request) (uint32, error) {
	var req struct {
		AssetID uint32 `json:"assetID"`
		Force   bool   `json:"force"`
		Disable bool   `json:"disable"`
		Address string `json:"address"`
	}
	err := json.NewDecoder(r.Body).Decode(&req)
	return req.AssetID, err
}

// OpenDcrdexWalletHandler unlocks a wallet. Requires the DEX session unlocked.
func OpenDcrdexWalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		AssetID uint32 `json:"assetID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "assetID is required", http.StatusBadRequest)
		return
	}
	appPass, ok := rpc.DcrdexAppPass()
	if !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.OpenWallet(ctx, appPass, req.AssetID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// CloseDcrdexWalletHandler locks a wallet.
func CloseDcrdexWalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	assetID, err := dexWalletActionAsset(r)
	if err != nil {
		http.Error(w, "assetID is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.CloseWallet(ctx, assetID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ToggleDcrdexWalletHandler enables or disables a wallet.
func ToggleDcrdexWalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		AssetID uint32 `json:"assetID"`
		Disable bool   `json:"disable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "assetID is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.ToggleWalletStatus(ctx, req.AssetID, req.Disable); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// RescanDcrdexWalletHandler triggers a wallet rescan.
func RescanDcrdexWalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		AssetID uint32 `json:"assetID"`
		Force   bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "assetID is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.RescanWallet(ctx, req.AssetID, req.Force); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// GetDcrdexWalletPeersHandler returns a wallet's peers (raw). Query: assetID.
func GetDcrdexWalletPeersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	assetID, err := strconv.ParseUint(r.URL.Query().Get("assetID"), 10, 32)
	if err != nil {
		http.Error(w, "assetID is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	raw, err := client.WalletPeers(ctx, uint32(assetID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write(raw)
}

// AddDcrdexWalletPeerHandler adds a persistent peer to a wallet.
func AddDcrdexWalletPeerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		AssetID uint32 `json:"assetID"`
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" {
		http.Error(w, "assetID and address are required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.AddWalletPeer(ctx, req.AssetID, req.Address); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// RemoveDcrdexWalletPeerHandler removes a persistent peer from a wallet.
func RemoveDcrdexWalletPeerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		AssetID uint32 `json:"assetID"`
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" {
		http.Error(w, "assetID and address are required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.RemoveWalletPeer(ctx, req.AssetID, req.Address); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// GetDcrdexNotificationsHandler returns up to `n` recent bisonw notifications
// (raw: type, topic, subject, details, severity, stamp, acked, id) for the
// notifications panel. Defaults to 50.
func GetDcrdexNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	n := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("n")); err == nil && v > 0 {
		n = v
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	raw, err := client.Notifications(ctx, n)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write(raw)
}

// dexRateCache caches Kraken USD rates across requests.
var dexRateCache = exchangerate.New()

// GetDcrdexRatesHandler returns a map of asset symbol to USD price for the DEX
// fiat display. Rates come from Kraken (direct USD pair, or BTC pair converted
// via BTC/USD); if Kraken is unreachable it falls back to Bison Relay's feed
// for DCR and BTC.
func GetDcrdexRatesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	rates, err := dexRateCache.USD(ctx)
	if err != nil || len(rates) == 0 {
		rates = map[string]float64{}
		if raw, berr := rpc.BrclientdRates(ctx); berr == nil {
			var br struct {
				DcrUsd float64 `json:"dcr_usd"`
				BtcUsd float64 `json:"btc_usd"`
			}
			if json.Unmarshal(raw, &br) == nil {
				if br.DcrUsd > 0 {
					rates["dcr"] = br.DcrUsd
				}
				if br.BtcUsd > 0 {
					rates["btc"] = br.BtcUsd
				}
			}
		}
	}
	json.NewEncoder(w).Encode(rates)
}

// ExportDcrdexSeedHandler returns the bisonw application seed for backup. The
// app password must be re-entered in the request body (not taken from the
// session) for this sensitive action. The seed is never persisted.
func ExportDcrdexSeedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		AppPass string `json:"appPass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AppPass == "" {
		http.Error(w, "appPass is required", http.StatusBadRequest)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	seed, err := client.AppSeed(ctx, req.AppPass)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"seed": seed})
}

// MarkDcrdexSeedBackedUpHandler records that the user has backed up the app seed,
// clearing the unlock backup reminder. dcrdex keeps no such flag, so it lives in
// the dashboard config.
func MarkDcrdexSeedBackedUpHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := setDcrdexSeedBackedUp(true); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// DiscoverDcrdexAccountHandler re-discovers the account on a DEX server (used
// after a seed restore, since a restored client has no record of which servers
// it registered with) and reports whether it is already paid, i.e. has a live
// fidelity bond. A successful discover records the account locally, so the UI
// can then treat the account as registered and skip bond posting.
func DiscoverDcrdexAccountHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host string `json:"host"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}
	appPass, ok := rpc.DcrdexAppPass()
	if !ok {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return
	}
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	paid, err := client.DiscoverAccount(ctx, appPass, req.Host, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"paid": paid})
}
