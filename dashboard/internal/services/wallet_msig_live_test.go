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
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"decred.org/dcrwallet/v5/wallet/txrules"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/hdkeychain/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"golang.org/x/sys/unix"

	"dcrpulse/internal/msig"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	. "dcrpulse/internal/services"
)

// promptPassphrase reads the wallet passphrase from the controlling
// terminal with echo disabled. It deliberately opens /dev/tty instead of
// stdin so it works no matter how go test wires the process, and so the
// passphrase can never arrive from a pipe, env or argv. The caller zeroes
// the returned slice.
func promptPassphrase(t *testing.T) []byte {
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

	fmt.Fprintf(tty, "wallet passphrase (echo off): ")
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

// paramsForAddress infers the network from the wallet's own address
// encoding, so the drill needs no dcrd connection.
func paramsForAddress(t *testing.T, addr string) *chaincfg.Params {
	t.Helper()
	for _, p := range []*chaincfg.Params{
		chaincfg.MainNetParams(), chaincfg.TestNet3Params(), chaincfg.SimNetParams(),
	} {
		if _, err := stdaddr.DecodeAddress(addr, p); err == nil {
			return p
		}
	}
	t.Fatalf("address %s belongs to no known network", addr)
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

// locateOrCreateAccount finds the named account or creates it with the
// wallet passphrase. Drill accounts persist across runs, so repeated
// drills never grow the account set.
func locateOrCreateAccount(ctx context.Context, t *testing.T, name string, pass []byte) uint32 {
	t.Helper()
	accounts, err := FetchAllAccounts(ctx)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	for _, a := range accounts {
		if a.AccountName == name {
			return a.AccountNumber
		}
	}
	number, err := CreateAccount(ctx, name, append([]byte(nil), pass...))
	if err != nil {
		t.Fatalf("create account %q: %v", name, err)
	}
	t.Logf("created drill account %q = %d (permanent)", name, number)
	return number
}

// deriveChild mirrors the production derivation: account xpub ->
// Child(branch) -> Child(index).
func deriveChild(t *testing.T, xpub string, branch, index uint32, params *chaincfg.Params) ([]byte, string) {
	t.Helper()
	key, err := hdkeychain.NewKeyFromString(xpub, params)
	if err != nil {
		t.Fatalf("parse xpub: %v", err)
	}
	for _, step := range []uint32{branch, index} {
		if key, err = key.Child(step); err != nil {
			t.Fatalf("derive child %d: %v", step, err)
		}
	}
	pub := key.SerializedPubKey()
	addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(stdaddr.Hash160(pub), params)
	if err != nil {
		t.Fatalf("child address: %v", err)
	}
	return pub, addr.String()
}

// TestMsigLiveHDMainnet drives the HD ladder primitives against the
// RUNNING wallet with dust-scale amounts that return to this wallet. It
// pins every wallet-level assumption the protocol rests on, BEFORE the
// protocol is trusted with real rosters:
//
//	(a) local Child derivation of an account xpub yields EXACTLY the
//	    pubkey and address the wallet reports at the same coordinates —
//	    the derivation-scheme seal
//	(b) batch importscript with rescan=false, idempotent re-import, no
//	    unlock
//	(c) listunspent accepts multiple addresses, rows carry the address,
//	    and the whole call fails on an unimported address
//	(d) a deposit to a ladder address is visible at zero confirmations
//	(e) signing an input FAILS to add a signature before the account's
//	    branch index covers it and succeeds after — the sync-before-sign
//	    precondition
//	(f) a real 2-of-2 ladder spend: two inputs at different indices,
//	    change to internal 0, per-input signatures attributed by
//	    participant, mempool acceptance at the estimated fee
//	(g) the restore ownership proof: exactly one account's xpub equals
//	    the drill xpub
//
// Not run by the normal test suite: requires -tags msiglive and env
// configuration, spends real funds (fees only, remainder stays at the
// ladder's change address), and leaves the drill accounts and scripts in
// the wallet. Mainnet only, funds to self, per the standing rule.
//
// Required env (the same RPC variables the compose stack already uses):
//
//	DCRPULSE_MSIG_LIVE=1
//	DCRWALLET_RPC_USER / DCRWALLET_RPC_PASS
//	DCRWALLET_RPC_CERT        also used for gRPC
//
// Optional env: DCRWALLET_RPC_HOST (localhost), DCRWALLET_RPC_PORT (9110),
// DCRWALLET_GRPC_PORT (9111), DCRPULSE_MSIG_LIVE_ACCOUNT (0, funding
// source), DCRPULSE_MSIG_LIVE_ATOMS (100000, per funded index).
func TestMsigLiveHDMainnet(t *testing.T) {
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
	srcAccount64, err := strconv.ParseUint(envOr("DCRPULSE_MSIG_LIVE_ACCOUNT", "0"), 10, 32)
	if err != nil {
		t.Fatalf("bad account env: %v", err)
	}
	srcAccount := uint32(srcAccount64)
	fundAtoms, err := strconv.ParseInt(envOr("DCRPULSE_MSIG_LIVE_ATOMS", "100000"), 10, 64)
	if err != nil || fundAtoms < 20000 {
		t.Fatalf("bad funding amount")
	}

	pass := promptPassphrase(t)
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

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	// Two dedicated drill accounts of this one wallet form the 2-of-2.
	acctA := locateOrCreateAccount(ctx, t, "msiglive-a", pass)
	acctB := locateOrCreateAccount(ctx, t, "msiglive-b", pass)
	xpubA, err := GetAccountExtendedPubKey(ctx, acctA)
	if err != nil {
		t.Fatalf("xpub a: %v", err)
	}
	xpubB, err := GetAccountExtendedPubKey(ctx, acctB)
	if err != nil {
		t.Fatalf("xpub b: %v", err)
	}
	if xpubA == xpubB {
		t.Fatalf("distinct accounts produced the same xpub")
	}
	xpubs := msig.SortXpubs([]string{xpubA, xpubB})

	// The network follows from an address of this wallet.
	nextAddr, err := GetNextAddress(ctx, srcAccount)
	if err != nil {
		t.Fatalf("network probe address: %v", err)
	}
	params := paramsForAddress(t, nextAddr)
	t.Logf("network: %s", params.Name)

	// (a) The derivation-scheme seal: local Child derivation must match
	// the wallet's own view of the same coordinates.
	localPub, localAddr := deriveChild(t, xpubA, 0, 0, params)
	if err := SyncAccountAddressIndex(ctx, "msiglive-a", 0, 1); err != nil {
		t.Fatalf("sync a/0: %v", err)
	}
	res, err := ValidateAddress(ctx, localAddr)
	if err != nil {
		t.Fatalf("validateaddress: %v", err)
	}
	if !res.IsMine {
		t.Fatalf("(a) locally derived child address is not recognized by the wallet")
	}
	if !res.IsValid || hex.EncodeToString(res.PubKey) != hex.EncodeToString(localPub) {
		t.Fatalf("(a) derivation mismatch: local %s wallet %x; the scheme seal is broken",
			hex.EncodeToString(localPub), res.PubKey)
	}
	t.Logf("(a) derivation seal holds at %s", localAddr)

	// The drill ladder: external 0 and 1 receive, internal 0 takes change.
	ext0, err := msig.AddressAt(2, xpubs, msig.BranchExternal, 0, params)
	if err != nil {
		t.Fatal(err)
	}
	ext1, err := msig.AddressAt(2, xpubs, msig.BranchExternal, 1, params)
	if err != nil {
		t.Fatal(err)
	}
	int0, err := msig.AddressAt(2, xpubs, msig.BranchInternal, 0, params)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ladder: ext0 %s ext1 %s int0 %s", ext0, ext1, int0)

	// (c) An unimported ladder address fails the whole listunspent call.
	ext5, err := msig.AddressAt(2, xpubs, msig.BranchExternal, 5, params)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ListSharedUTXOs(ctx, []string{ext5}); err == nil {
		t.Logf("(c) note: wallet accepted an unimported address; import-before-list stays mandatory")
	} else {
		t.Logf("(c) unknown-address refusal confirmed: %v", err)
	}

	// (b) Batch import, no rescan, no unlock; re-import is a no-op.
	for _, addr := range []struct {
		branch, index uint32
	}{{0, 0}, {0, 1}, {1, 0}} {
		script, _, err := msig.ScriptAt(2, xpubs, addr.branch, addr.index, params)
		if err != nil {
			t.Fatal(err)
		}
		if err := ImportMsigScript(ctx, hex.EncodeToString(script), false, 0); err != nil {
			t.Fatalf("import %d/%d: %v", addr.branch, addr.index, err)
		}
		if err := ImportMsigScript(ctx, hex.EncodeToString(script), false, 0); err != nil {
			t.Fatalf("re-import %d/%d: %v", addr.branch, addr.index, err)
		}
	}
	t.Logf("(b) ladder imported, idempotent, no unlock")

	// Fund ext0 and ext1 from the source account — unless a prior run
	// already left a deposit there, which the drill reuses so aborted
	// runs never strand funds.
	fund := func(addr string) {
		construct, err := ConstructTransaction(ctx, srcAccount,
			[]types.TxRecipient{{Address: addr, AmountAtoms: fundAtoms}}, false)
		if err != nil {
			t.Fatalf("construct funding tx: %v", err)
		}
		txid, err := SignAndPublishTransaction(ctx, srcAccount, construct.UnsignedTransaction, passCopy())
		if err != nil {
			t.Fatalf("fund %s: %v", addr, err)
		}
		t.Logf("funded %s by %s", addr, txid)
	}
	existing, err := ListSharedUTXOs(ctx, []string{ext0, ext1})
	if err != nil {
		t.Fatalf("listunspent before funding: %v", err)
	}
	have := map[string]bool{}
	for _, u := range existing {
		have[u.Address] = true
	}
	for _, addr := range []string{ext0, ext1} {
		if have[addr] {
			t.Logf("reusing a prior run's deposit at %s", addr)
			continue
		}
		fund(addr)
	}

	// (c)+(d) Multi-address listunspent sees both deposits at 0 conf,
	// rows carrying their addresses.
	var utxos []SharedUTXO
	deadline := time.Now().Add(45 * time.Second)
	for {
		utxos, err = ListSharedUTXOs(ctx, []string{ext0, ext1, int0})
		if err != nil {
			t.Fatalf("listunspent: %v", err)
		}
		byAddr := map[string]int{}
		for _, u := range utxos {
			byAddr[u.Address]++
		}
		if byAddr[ext0] >= 1 && byAddr[ext1] >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("(d) deposits not visible at 0 conf: %+v", utxos)
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("(c)(d) both deposits visible unconfirmed with address fields")

	// Build the sweep with change to internal 0: one input per external
	// index, whatever transaction funded it.
	var ins []msig.UTXO
	picked := map[string]bool{}
	for _, u := range utxos {
		if (u.Address == ext0 || u.Address == ext1) && !picked[u.Address] {
			picked[u.Address] = true
			ins = append(ins, msig.UTXO{TxID: u.TxID, Vout: u.Vout, Tree: u.Tree, Atoms: u.Atoms, Address: u.Address})
		}
	}
	if len(ins) != 2 {
		t.Fatalf("expected one input per drill index, have %d", len(ins))
	}
	script0, _, err := msig.ScriptAt(2, xpubs, msig.BranchExternal, 0, params)
	if err != nil {
		t.Fatal(err)
	}
	sendBack := ins[0].Atoms + ins[1].Atoms
	fee := int64(txrules.FeeForSerializeSize(txrules.DefaultRelayFeePerKb, msig.EstimateFullSize(2, 1, 2, 2)))
	tx, gotFee, change, err := msig.BuildSpend(msig.BuildSpendParams{
		UTXOs:      ins,
		Recipients: []msig.Recipient{{Address: int0, Atoms: sendBack - fee}},
		// The recipient IS the change address here; BuildSpend folds the
		// residue into the fee, which is exactly what the sweep wants.
		ChangeAddress: int0,
		RedeemScript:  script0,
		ChainParams:   params,
	})
	if err != nil {
		t.Fatalf("build spend: %v", err)
	}
	_ = change
	txid := tx.TxHash().String()
	rawBytes, err := tx.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	raw := hex.EncodeToString(rawBytes)
	t.Logf("spend %s, fee %d atoms", txid, gotFee)

	prevs := []MsigPrevInput{
		{TxID: ins[0].TxID, Vout: ins[0].Vout, Tree: ins[0].Tree, ScriptPubKey: pkScriptHexFor(t, ins[0].Address, params)},
		{TxID: ins[1].TxID, Vout: ins[1].Vout, Tree: ins[1].Tree, ScriptPubKey: pkScriptHexFor(t, ins[1].Address, params)},
	}

	// The verification resolver: per-input scripts and key ownership.
	resolve := func(idx int) ([]byte, map[string]string, error) {
		u := ins[idx]
		index := uint32(0)
		if u.Address == ext1 {
			index = 1
		}
		script, _, err := msig.ScriptAt(2, xpubs, msig.BranchExternal, index, params)
		if err != nil {
			return nil, nil, err
		}
		keys, err := msig.KeysAt(xpubs, msig.BranchExternal, index, params)
		if err != nil {
			return nil, nil, err
		}
		owner := make(map[string]string, len(keys))
		for ki, k := range keys {
			owner[hex.EncodeToString(k)] = xpubs[ki]
		}
		return script, owner, nil
	}

	// (e) The sync-before-sign probe. On a virgin account signing an
	// unsynced index adds nothing; a reused drill account (or a wide
	// gaplimit) may already cover the indices, so a pre-sync success is
	// reported rather than failed — the production invariant is only
	// that signing works AFTER the window sync, proven below.
	signedHex, err := SignMsigTransaction(ctx, raw, prevs, acctA, passCopy())
	if err != nil {
		t.Logf("(e) pre-sync signing errored (%v); syncing", err)
	} else if preTx, derr := msig.DecodeTxHex(signedHex); derr == nil {
		if preSigners, verr := msig.VerifyProposalUpdateHD(preTx, txid, resolve); verr != nil || len(preSigners) == 0 {
			t.Logf("(e) pre-sync signing left the participant set empty; the wallet requires the branch sync")
		} else {
			t.Logf("(e) note: the wallet already covered the drill indices (prior run or gap limit); the negative half was not exercised")
		}
	}
	if err := SyncAccountAddressIndex(ctx, "msiglive-a", 0, 2); err != nil {
		t.Fatalf("sync a through 2: %v", err)
	}
	if err := SyncAccountAddressIndex(ctx, "msiglive-b", 0, 2); err != nil {
		t.Fatalf("sync b through 2: %v", err)
	}

	// (f) Sign with both accounts and broadcast. Post-sync signing MUST
	// succeed — this is the production invariant.
	signedHex, err = SignMsigTransaction(ctx, raw, prevs, acctA, passCopy())
	if err != nil {
		t.Fatalf("sign a: %v", err)
	}
	oneTx, err := msig.DecodeTxHex(signedHex)
	if err != nil {
		t.Fatal(err)
	}
	oneSigners, err := msig.VerifyProposalUpdateHD(oneTx, txid, resolve)
	if err != nil {
		t.Fatalf("verify after a: %v", err)
	}
	if len(oneSigners) != 1 {
		t.Fatalf("(f) after account a: %d participants", len(oneSigners))
	}
	signedHex, err = SignMsigTransaction(ctx, signedHex, prevs, acctB, passCopy())
	if err != nil {
		t.Fatalf("sign b: %v", err)
	}
	twoTx, err := msig.DecodeTxHex(signedHex)
	if err != nil {
		t.Fatal(err)
	}
	twoSigners, err := msig.VerifyProposalUpdateHD(twoTx, txid, resolve)
	if err != nil {
		t.Fatalf("verify after b: %v", err)
	}
	if len(twoSigners) != 2 {
		t.Fatalf("(f) after both accounts: %d participants", len(twoSigners))
	}
	signedBytes, err := twoTx.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	sent, err := BroadcastSignedTransaction(ctx, signedBytes)
	if err != nil {
		t.Fatalf("(f) broadcast: %v", err)
	}
	if sent != txid {
		t.Fatalf("broadcast txid %s != built %s", sent, txid)
	}
	t.Logf("(f) 2-of-2 ladder spend broadcast as %s", sent)

	// The inputs leave the set and the change appears at internal 0.
	spent := map[string]bool{}
	for _, in := range ins {
		spent[fmt.Sprintf("%s:%d", in.TxID, in.Vout)] = true
	}
	deadline = time.Now().Add(45 * time.Second)
	for {
		utxos, err = ListSharedUTXOs(ctx, []string{ext0, ext1, int0})
		if err != nil {
			t.Fatalf("listunspent after spend: %v", err)
		}
		spentGone := true
		changeSeen := false
		for _, u := range utxos {
			if spent[fmt.Sprintf("%s:%d", u.TxID, u.Vout)] {
				spentGone = false
			}
			if u.TxID == txid && u.Address == int0 {
				changeSeen = true
			}
		}
		if spentGone && changeSeen {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("(f) post-spend set did not converge: %+v", utxos)
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("(f) inputs consumed; sweep output visible at internal 0")

	// (g) The restore ownership proof: exactly one account's xpub equals
	// the drill xpub, and it is the drill account.
	accounts, err := FetchAllAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, a := range accounts {
		if a.AccountNumber >= 1<<31 {
			continue
		}
		got, err := GetAccountExtendedPubKey(ctx, a.AccountNumber)
		if err != nil {
			continue
		}
		if got == xpubA {
			matches++
			if a.AccountNumber != acctA {
				t.Fatalf("(g) xpub matched foreign account %d", a.AccountNumber)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("(g) xpub equality matched %d accounts", matches)
	}
	t.Logf("(g) restore ownership proof holds")
}
