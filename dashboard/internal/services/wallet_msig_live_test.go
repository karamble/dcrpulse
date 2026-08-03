// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build msiglive

// The live test sits in the external test package because it imports
// internal/msig, which imports services back; only exported services
// identifiers are used, brought in by the dot import.
package services_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"decred.org/dcrwallet/v5/wallet/txrules"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"golang.org/x/sys/unix"

	"dcrpulse/internal/msig"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	. "dcrpulse/internal/services"
)

// promptPassphrase reads the account passphrase from the controlling
// terminal with echo disabled. It deliberately opens /dev/tty instead of
// stdin so it works no matter how go test wires the process, and so the
// passphrase can never arrive from a pipe, env or argv. The caller zeroes
// the returned slice.
func promptPassphrase(t *testing.T, account uint32) []byte {
	t.Helper()
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no controlling terminal for the passphrase prompt (%v); run interactively", err)
	}
	defer tty.Close()

	fd := int(tty.Fd())
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		t.Fatalf("read terminal state: %v", err)
	}
	noEcho := *old
	noEcho.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &noEcho); err != nil {
		t.Fatalf("disable terminal echo: %v", err)
	}
	defer func() {
		if err := unix.IoctlSetTermios(fd, unix.TCSETS, old); err != nil {
			t.Logf("restore terminal echo: %v", err)
		}
	}()

	fmt.Fprintf(tty, "passphrase for account %d (echo off): ", account)
	var pass []byte
	buf := make([]byte, 1)
	for {
		n, err := tty.Read(buf)
		if err != nil {
			t.Fatalf("read passphrase: %v", err)
		}
		if n == 0 {
			continue
		}
		if buf[0] == '\n' {
			break
		}
		if buf[0] != '\r' {
			pass = append(pass, buf[0])
		}
	}
	fmt.Fprintln(tty)
	if len(pass) == 0 {
		t.Skip("empty passphrase")
	}
	return pass
}

// TestMsigLiveMainnet drives the multisig wallet primitives against the
// RUNNING wallet with dust-scale amounts that return to this wallet. It is
// the assertion form of the phase 1 spike:
//
//	(a) listunspent covers imported-script outputs at zero confirmations
//	(b) signrawtransaction resolves the redeem script from the imported
//	    script store without being handed one
//	(c) unlocking only the participating account is sufficient authority
//	(d) both signatures land and the counter agrees (shape details are
//	    unit-tested; both keys here live in one account so one signing
//	    pass completes the input)
//	(e) importscript needs no unlock and re-import is a no-op
//	(f) the mempool accepts the manually built spend at the estimated fee
//	(g) accountsyncaddressindex plus validateaddress recover the own-key
//	    coordinates (the cheap half of the restore drill)
//
// Not run by the normal test suite: requires -tags msiglive and env
// configuration, spends real funds (fees only, remainder returns to the
// source account), and leaves the throwaway script imported in the wallet.
//
// The account passphrase is prompted from /dev/tty with echo disabled and
// is never accepted through env, argv or files. It lives in this process
// only for the duration of the run and is zeroed afterwards, matching the
// dashboard's own passphrase discipline.
//
// Required env (the same RPC variables the compose stack already uses):
//
//	DCRPULSE_MSIG_LIVE=1
//	DCRWALLET_RPC_USER / DCRWALLET_RPC_PASS
//	DCRWALLET_RPC_CERT        also used for gRPC
//
// Optional env: DCRWALLET_RPC_HOST (localhost), DCRWALLET_RPC_PORT (9110),
// DCRWALLET_GRPC_PORT (9111), DCRPULSE_MSIG_LIVE_ACCOUNT (0),
// DCRPULSE_MSIG_LIVE_ATOMS (100000).
func TestMsigLiveMainnet(t *testing.T) {
	if os.Getenv("DCRPULSE_MSIG_LIVE") != "1" {
		t.Skip("set DCRPULSE_MSIG_LIVE=1 to run the live wallet drill")
	}
	envOr := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	user := os.Getenv("DCRWALLET_RPC_USER")
	rpcPass := os.Getenv("DCRWALLET_RPC_PASS")
	cert := os.Getenv("DCRWALLET_RPC_CERT")
	if user == "" || rpcPass == "" || cert == "" {
		t.Skip("DCRWALLET_RPC_USER, DCRWALLET_RPC_PASS and DCRWALLET_RPC_CERT are required")
	}
	account64, err := strconv.ParseUint(envOr("DCRPULSE_MSIG_LIVE_ACCOUNT", "0"), 10, 32)
	if err != nil {
		t.Fatalf("bad account env: %v", err)
	}
	account := uint32(account64)
	fundAtoms, err := strconv.ParseInt(envOr("DCRPULSE_MSIG_LIVE_ATOMS", "100000"), 10, 64)
	if err != nil || fundAtoms < 20000 {
		t.Fatalf("bad funding amount")
	}

	// Prompt before anything connects: the passphrase is typed once, with
	// no time limit, and the rest of the drill runs unattended. Copies are
	// handed out per use because the service functions zero their argument.
	pass := promptPassphrase(t, account)
	defer func() {
		for i := range pass {
			pass[i] = 0
		}
	}()
	passCopy := func() []byte { return append([]byte(nil), pass...) }

	host := envOr("DCRWALLET_RPC_HOST", "localhost")
	if err := rpc.InitWalletClient(rpc.Config{
		RPCHost: host, RPCPort: envOr("DCRWALLET_RPC_PORT", "9110"),
		RPCUser: user, RPCPassword: rpcPass, RPCCert: cert,
	}); err != nil {
		t.Fatalf("wallet JSON-RPC init: %v", err)
	}
	// Mutual TLS: the gRPC dial loads the rpc.cert/rpc.key pair, with the
	// key path derived from the cert path, so both files must sit side by
	// side wherever the cert lives.
	if err := rpc.InitWalletGrpcClient(rpc.GrpcConfig{
		GrpcHost: host, GrpcPort: envOr("DCRWALLET_GRPC_PORT", "9111"), GrpcCert: cert,
	}); err != nil {
		t.Fatalf("wallet gRPC init: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// Two fresh keys from the same account form the throwaway 2-of-2.
	k1, err := DeriveMsigKey(ctx, account)
	if err != nil {
		t.Fatalf("derive key 1: %v", err)
	}
	k2, err := DeriveMsigKey(ctx, account)
	if err != nil {
		t.Fatalf("derive key 2: %v", err)
	}
	if k1.PubKey == k2.PubKey {
		t.Fatalf("derived keys are not distinct")
	}
	// The drill needs no dcrd connection: the network follows from the
	// wallet's own address encoding.
	params := paramsForAddress(t, k1.Address)
	t.Logf("network: %s", params.Name)
	pk1, err := hex.DecodeString(k1.PubKey)
	if err != nil {
		t.Fatalf("pubkey 1 hex: %v", err)
	}
	pk2, err := hex.DecodeString(k2.PubKey)
	if err != nil {
		t.Fatalf("pubkey 2 hex: %v", err)
	}

	redeem, _, err := msig.MultiSigScript(2, [][]byte{pk1, pk2})
	if err != nil {
		t.Fatalf("build script: %v", err)
	}
	sharedAddr, err := msig.P2SHAddress(redeem, params)
	if err != nil {
		t.Fatalf("shared address: %v", err)
	}
	t.Logf("shared address: %s (stays imported after the drill)", sharedAddr)

	// (e) import without any unlock, then re-import as a no-op.
	redeemHex := hex.EncodeToString(redeem)
	if err := ImportMsigScript(ctx, redeemHex, false, 0); err != nil {
		t.Fatalf("importscript: %v", err)
	}
	if err := ImportMsigScript(ctx, redeemHex, false, 0); err != nil {
		t.Fatalf("importscript re-import: %v", err)
	}

	// Fund the shared address from the source account.
	construct, err := ConstructTransaction(ctx, account,
		[]types.TxRecipient{{Address: sharedAddr, AmountAtoms: fundAtoms}}, false)
	if err != nil {
		t.Fatalf("construct funding tx: %v", err)
	}
	fundTxID, err := SignAndPublishTransaction(ctx, account,
		construct.UnsignedTransaction, passCopy())
	if err != nil {
		t.Fatalf("fund shared address: %v", err)
	}
	t.Logf("funding tx: %s", fundTxID)

	// (a) the imported-script output must show up unconfirmed.
	var funded *SharedUTXO
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		utxos, err := ListSharedUTXOs(ctx, []string{sharedAddr})
		if err != nil {
			t.Fatalf("listunspent: %v", err)
		}
		for i := range utxos {
			if utxos[i].TxID == fundTxID {
				funded = &utxos[i]
			}
		}
		if funded != nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if funded == nil {
		t.Fatalf("funding output never appeared in listunspent for %s", sharedAddr)
	}
	if funded.Atoms != fundAtoms {
		t.Fatalf("funding amount mismatch: got %d, want %d", funded.Atoms, fundAtoms)
	}
	t.Logf("shared UTXO visible at %d confirmations", funded.Confirmations)

	// Build the spend back to the source account, sweeping the whole
	// output so no change is needed.
	destAddr, err := GetNextAddress(ctx, account)
	if err != nil {
		t.Fatalf("destination address: %v", err)
	}
	fee := int64(txrules.FeeForSerializeSize(txrules.DefaultRelayFeePerKb,
		msig.EstimateFullSize(1, 1, 2, 2)))
	tx, actualFee, change, err := msig.BuildSpend(msig.BuildSpendParams{
		UTXOs:         []msig.UTXO{{TxID: funded.TxID, Vout: funded.Vout, Tree: funded.Tree, Atoms: funded.Atoms}},
		Recipients:    []msig.Recipient{{Address: destAddr, Atoms: funded.Atoms - fee}},
		ChangeAddress: sharedAddr,
		RedeemScript:  redeem,
		ChainParams:   params,
	})
	if err != nil {
		t.Fatalf("build spend: %v", err)
	}
	if change != 0 {
		t.Fatalf("sweep produced change: %d", change)
	}
	if actualFee > 20000 {
		t.Fatalf("fee out of bounds: %d atoms", actualFee)
	}
	txid := tx.TxHash().String()

	var buf []byte
	{
		b, err := tx.Bytes()
		if err != nil {
			t.Fatalf("serialize: %v", err)
		}
		buf = b
	}
	rawHex := hex.EncodeToString(buf)

	// The shared address pkScript for prevout resolution: the funding tx
	// may still be unconfirmed.
	prevInputs := []MsigPrevInput{{
		TxID: funded.TxID, Vout: funded.Vout, Tree: funded.Tree,
		ScriptPubKey: pkScriptHexFor(t, sharedAddr, params),
	}}

	// (b), (c), (d): one signing pass with only the source account
	// unlocked must resolve the imported redeem script and land both
	// signatures.
	signedHex, err := SignMsigTransaction(ctx, rawHex, prevInputs, account, passCopy())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	signedTx, err := msig.DecodeTxHex(signedHex)
	if err != nil {
		t.Fatalf("decode signed: %v", err)
	}
	if got := signedTx.TxHash().String(); got != txid {
		t.Fatalf("txid changed by signing: %s", got)
	}
	signers, err := msig.VerifyProposalUpdate(signedTx, txid, redeem, [][]byte{pk1, pk2})
	if err != nil {
		t.Fatalf("verify signatures: %v", err)
	}
	if len(signers) != 2 {
		t.Fatalf("signature count: got %d, want 2", len(signers))
	}

	// (f) broadcast through the wallet so it tracks the spend.
	signedBytes, err := hex.DecodeString(signedHex)
	if err != nil {
		t.Fatalf("signed hex: %v", err)
	}
	broadcastID, err := BroadcastSignedTransaction(ctx, signedBytes)
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if broadcastID != txid {
		t.Fatalf("broadcast txid %s does not match built txid %s", broadcastID, txid)
	}
	t.Logf("spend tx accepted: %s (fee %d atoms)", broadcastID, actualFee)

	// The spent output must leave the shared UTXO set.
	deadline = time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		utxos, err := ListSharedUTXOs(ctx, []string{sharedAddr})
		if err != nil {
			t.Fatalf("listunspent after spend: %v", err)
		}
		gone := true
		for _, u := range utxos {
			if u.TxID == funded.TxID && u.Vout == funded.Vout {
				gone = false
			}
		}
		if gone {
			finishRestoreDrill(ctx, t, account, k1, k2)
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("spent output still listed as unspent")
}

func paramsForAddress(t *testing.T, addr string) *chaincfg.Params {
	t.Helper()
	candidates := []*chaincfg.Params{
		chaincfg.MainNetParams(), chaincfg.TestNet3Params(), chaincfg.SimNetParams(),
	}
	for _, p := range candidates {
		if _, err := stdaddr.DecodeAddress(addr, p); err == nil {
			return p
		}
	}
	t.Fatalf("address %s matches no known network", addr)
	return nil
}

func pkScriptHexFor(t *testing.T, addr string, params *chaincfg.Params) string {
	t.Helper()
	a, err := stdaddr.DecodeAddress(addr, params)
	if err != nil {
		t.Fatalf("decode %s: %v", addr, err)
	}
	_, script := a.PaymentScript()
	return hex.EncodeToString(script)
}

func recheckOwnKey(ctx context.Context, address string) (string, error) {
	p, err := json.Marshal(address)
	if err != nil {
		return "", err
	}
	raw, err := rpc.WalletClient.RawRequest(ctx, "validateaddress", []json.RawMessage{p})
	if err != nil {
		return "", err
	}
	var res struct {
		IsMine bool   `json:"ismine"`
		PubKey string `json:"pubkey"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if !res.IsMine {
		return "", fmt.Errorf("address %s is not recognized as owned", address)
	}
	return res.PubKey, nil
}

// finishRestoreDrill covers (g): the address cursor fast-forward and the
// own-key coordinate recovery a backup-card restore relies on.
func finishRestoreDrill(ctx context.Context, t *testing.T, account uint32, k1, k2 *MsigOwnKey) {
	t.Helper()
	accounts, err := FetchAllAccounts(ctx)
	if err != nil {
		t.Fatalf("accounts: %v", err)
	}
	accountName := ""
	for _, a := range accounts {
		if a.AccountNumber == account {
			accountName = a.AccountName
		}
	}
	if accountName == "" {
		t.Fatalf("account %d not found", account)
	}
	if err := SyncAccountAddressIndex(ctx, accountName, k2.Branch, k2.Index+1); err != nil {
		t.Fatalf("accountsyncaddressindex: %v", err)
	}
	recovered, err := recheckOwnKey(ctx, k1.Address)
	if err != nil {
		t.Fatalf("validateaddress after sync: %v", err)
	}
	if recovered != k1.PubKey {
		t.Fatalf("recovered pubkey %s does not match %s", recovered, k1.PubKey)
	}
	t.Logf("restore drill: own key coordinates recovered for %s", k1.Address)
}
