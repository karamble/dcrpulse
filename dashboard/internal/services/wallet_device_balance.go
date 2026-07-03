// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
	"dcrpulse/pkg/exchangerate"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/dcrutil/v4"
)

// The air-gapped device cannot see the chain, so the companion reports balances
// for its display. The BalanceUpdate payload is CBOR in the SignRequest's
// minicbor conventions (positional arrays, byte fields as arrays of integers):
//
//	BalanceUpdate  = array[4]{format_version=1, asof unix seconds,
//	                          rate_micro_usd (USD per DCR x 1e6, 0=unavailable),
//	                          accounts []AccountBalance}
//	AccountBalance = array[2]{fp [4]byte (account fingerprint, as in the
//	                          SignRequest's trailing field), balance_atoms}
//
// The device matches entries to its accounts by fingerprint, computes fiat from
// the rate, and sums entries for a wallet total. Decoders ignore extra trailing
// array elements, so the format extends by appending fields. The same bytes ship
// as a balance.dcr file (microSD, read when the device app launches) and as a
// single-part "dcr-balance" UR QR. Display only - nothing on the device spends
// or verifies against these figures.

const deviceBalanceFormatVersion = 1

// deviceBalanceFileName is the file name the device looks for on the microSD card.
const deviceBalanceFileName = "balance.dcr"

type deviceBalanceAccount struct {
	fp    []byte
	atoms uint64
}

// encodeBalanceUpdate serializes a BalanceUpdate. Pure function of its inputs so
// tests pin the exact bytes the device fixtures use.
func encodeBalanceUpdate(asof uint64, rateMicroUSD uint64, accounts []deviceBalanceAccount) []byte {
	w := &cborWriter{}
	w.arrayHead(4)
	w.uint(deviceBalanceFormatVersion)
	w.uint(asof)
	w.uint(rateMicroUSD)
	w.arrayHead(len(accounts))
	for _, a := range accounts {
		w.arrayHead(2)
		w.byteArray(a.fp)
		w.uint(a.atoms)
	}
	return w.buf
}

// deviceRates queries Kraken through the Tor-aware external transport. The
// export must not depend on per-wallet daemons: watch-only wallets - the main
// consumers - run without a brclientd, so the rate comes from Kraken directly.
var deviceRates = exchangerate.New(ExternalTransport())

// deviceBalanceRate returns a FRESH Kraken DCR/USD price: exactly one ticker
// request per export preparation, no caching. 0 = unavailable; the export
// stays useful and the device hides fiat.
func deviceBalanceRate(ctx context.Context) float64 {
	rctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	rate, err := deviceRates.KrakenUSD(rctx, "dcr")
	if err != nil {
		log.Printf("WARN: device balance without exchange rate: %v", err)
		return 0
	}
	return rate
}

// BuildDeviceBalance assembles the BalanceUpdate for the active wallet: every
// account except dcrwallet's literal "imported" private-key bucket (it has no
// xpub, so no fingerprint), keyed by account fingerprint, with total balances
// in atoms. It uses no private keys and works for watch-only wallets.
func BuildDeviceBalance(ctx context.Context) (*types.DeviceBalanceExport, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC client not initialized")
	}
	acctsResp, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	accounts := make([]*pb.AccountsResponse_Account, 0, len(acctsResp.Accounts))
	for _, a := range acctsResp.Accounts {
		if a.AccountName == "imported" {
			continue
		}
		accounts = append(accounts, a)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].AccountNumber < accounts[j].AccountNumber
	})

	entries := make([]deviceBalanceAccount, 0, len(accounts))
	display := make([]types.DeviceBalanceAccount, 0, len(accounts))
	for _, a := range accounts {
		fp, ferr := accountFingerprint(ctx, a.AccountNumber)
		if ferr != nil {
			// An account the device cannot match is useless in the payload;
			// skipping it keeps the rest of the export working.
			log.Printf("WARN: device balance skips account %q: %v", a.AccountName, ferr)
			continue
		}
		entries = append(entries, deviceBalanceAccount{fp: fp, atoms: uint64(a.TotalBalance)})
		display = append(display, types.DeviceBalanceAccount{
			Name:   a.AccountName,
			Number: a.AccountNumber,
			Fp:     hex.EncodeToString(fp),
			Atoms:  a.TotalBalance,
			Dcr:    dcrutil.Amount(a.TotalBalance).ToCoin(),
		})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no account fingerprints available for the balance export")
	}

	rate := deviceBalanceRate(ctx)
	asof := time.Now().Unix()
	cbor := encodeBalanceUpdate(uint64(asof), uint64(math.Round(rate*1e6)), entries)
	return &types.DeviceBalanceExport{
		BalanceB64: base64.StdEncoding.EncodeToString(cbor),
		BalanceUR:  EncodeUR("dcr-balance", cbor),
		Accounts:   display,
		RateUsd:    rate,
		AsOf:       asof,
		FileName:   deviceBalanceFileName,
	}, nil
}
