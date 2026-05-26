// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/pkg/bisonw"

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

// DexConfigResponse is the registration-screen view of a DEX server's config.
// Amounts are converted to DCR in the backend with dcrutil so the frontend does
// not hardcode the atoms-per-coin ratio.
type DexMarket struct {
	Base  string `json:"base"`
	Quote string `json:"quote"`
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
	raw, err := client.GetDEXConfig(ctx, host, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	var xc struct {
		Host             string `json:"host"`
		AcctID           string `json:"acctID"`
		ConnectionStatus int    `json:"connectionStatus"`
		BondExpiry       uint64 `json:"bondExpiry"`
		BondAssets       map[string]struct {
			Confs uint32 `json:"confs"`
			Amt   uint64 `json:"amount"`
		} `json:"bondAssets"`
		Markets map[string]struct {
			BaseSymbol  string `json:"basesymbol"`
			QuoteSymbol string `json:"quotesymbol"`
		} `json:"markets"`
	}
	if err := json.Unmarshal(raw, &xc); err != nil {
		http.Error(w, "decode dex config: "+err.Error(), http.StatusBadGateway)
		return
	}
	dcr := xc.BondAssets["dcr"]
	markets := make([]DexMarket, 0, len(xc.Markets))
	for _, m := range xc.Markets {
		markets = append(markets, DexMarket{
			Base:  strings.ToUpper(m.BaseSymbol),
			Quote: strings.ToUpper(m.QuoteSymbol),
		})
	}
	sort.Slice(markets, func(i, j int) bool {
		if markets[i].Base != markets[j].Base {
			return markets[i].Base < markets[j].Base
		}
		return markets[i].Quote < markets[j].Quote
	})
	json.NewEncoder(w).Encode(DexConfigResponse{
		Host:             xc.Host,
		ConnectionStatus: xc.ConnectionStatus,
		Registered:       xc.AcctID != "",
		BondExpiryDays:   int(xc.BondExpiry / 86400),
		BondConfs:        dcr.Confs,
		BondPerTierAtoms: dcr.Amt,
		BondPerTierDcr:   dcrutil.Amount(dcr.Amt).ToCoin(),
		MarketCount:      len(markets),
		Markets:          markets,
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
