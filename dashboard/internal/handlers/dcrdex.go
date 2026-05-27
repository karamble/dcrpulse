// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/dexassets"
	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/pkg/bisonw"
	"dcrpulse/pkg/exchangerate"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/gorilla/websocket"
)

// dexAccountName is the dedicated dcrwallet account DCRDEX trades from.
const dexAccountName = "dex"

// DcrdexStatus reports reachability and versions of the backend-only bisonw
// daemon. Richer onboarding state (initialized/logged-in/registered) is added
// alongside the init/login routes.
type DcrdexStatus struct {
	Reachable     bool   `json:"reachable"`
	Initialized   bool   `json:"initialized"`
	Unlocked      bool   `json:"unlocked"`
	Stage         string `json:"stage"` // unavailable | needs-init | needs-unlock | ready
	BisonwVersion string `json:"bisonwVersion,omitempty"`
	RPCServerVer  string `json:"rpcServerVersion,omitempty"`
	Error         string `json:"error,omitempty"`
}

// GetDcrdexStatusHandler reports whether the bisonw RPC server is reachable and
// its versions, via the version route (no app initialization required).
func GetDcrdexStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	st := DcrdexStatus{Initialized: dcrdexInitialized()}
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

func dcrdexInitialized() bool {
	cfg, err := config.LoadGlobalCfg()
	if err != nil {
		return false
	}
	var b bool
	cfg.Get(config.KeyDcrdexInitialized, &b)
	return b
}

func setDcrdexInitialized() error {
	cfg, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	if err := cfg.Set(config.KeyDcrdexInitialized, true); err != nil {
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
	if client, err := rpc.DcrdexClient(); err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		client.Logout(ctx)
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
	Base            string `json:"base"`
	Quote           string `json:"quote"`
	BaseID          uint32 `json:"baseID"`
	QuoteID         uint32 `json:"quoteID"`
	LotSize         uint64 `json:"lotSize"`         // atomic
	RateStep        uint64 `json:"rateStep"`        // atomic message-rate
	BaseConvFactor  uint64 `json:"baseConvFactor"`  // base atoms per conventional unit
	QuoteConvFactor uint64 `json:"quoteConvFactor"` // quote atoms per conventional unit
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
	return client.GetDEXConfig(ctx, host, "")
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
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
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
			BaseSymbol  string `json:"basesymbol"`
			QuoteSymbol string `json:"quotesymbol"`
			BaseID      uint32 `json:"baseid"`
			QuoteID     uint32 `json:"quoteid"`
			LotSize     uint64 `json:"lotsize"`
			RateStep    uint64 `json:"ratestep"`
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
// bisonw's /ws endpoint. The frontend speaks bisonw's msgjson protocol
// (loadmarket, loadcandles, ...) and receives its order book and notification
// pushes; the dashboard supplies the pinned TLS + auth to bisonw.
func DcrdexWSHandler(w http.ResponseWriter, r *http.Request) {
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
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
		Host    string `json:"host"`
		IsLimit bool   `json:"isLimit"`
		Sell    bool   `json:"sell"`
		Base    uint32 `json:"base"`
		Quote   uint32 `json:"quote"`
		Qty     uint64 `json:"qty"`
		Rate    uint64 `json:"rate"`
		TifNow  bool   `json:"tifNow"`
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
	})
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

// GetDcrdexWalletTxsHandler returns a wallet's transaction history (amounts in
// conventional units). Query: assetID (required), n, refID, past.
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
