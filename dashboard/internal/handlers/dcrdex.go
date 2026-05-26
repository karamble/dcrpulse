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
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/pkg/bisonw"
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
