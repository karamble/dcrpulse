// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

// mainnetTSpendBlock is mainnet block 1080576 and the updates dcrd reports for
// it: its treasurybase, then the 32 outputs and the fee of one treasury spend.
func mainnetTSpendBlock(t *testing.T) (*wire.MsgBlock, []int64) {
	t.Helper()
	raw, err := os.ReadFile("testdata/block1080576.hex")
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var block wire.MsgBlock
	if err := block.Deserialize(bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}
	rawUpdates, err := os.ReadFile("testdata/block1080576.updates.json")
	if err != nil {
		t.Fatal(err)
	}
	var updates []int64
	if err := json.Unmarshal(rawUpdates, &updates); err != nil {
		t.Fatal(err)
	}
	return &block, updates
}

func TestTreasuryBlockFlowsReadsAMainnetTreasurySpend(t *testing.T) {
	block, updates := mainnetTSpendBlock(t)
	const hash = "c09e3112e2407d156a4c9841ee17486bedceba095a5ad187535b0c6d484a9639"
	if block.BlockHash().String() != hash {
		t.Fatalf("fixture is block %s", block.BlockHash())
	}
	f, err := treasuryBlockFlows(1080576, hash, block.Header.Timestamp.Unix(), updates, block, chaincfg.MainNetParams())
	if err != nil {
		t.Fatal(err)
	}
	if f.tbase != 54683468 {
		t.Fatalf("treasurybase %d", f.tbase)
	}
	if len(f.tadds) != 0 || len(f.tspends) != 1 {
		t.Fatalf("got %d contributions and %d spends", len(f.tadds), len(f.tspends))
	}
	s := f.tspends[0]
	if s.TxHash != "9f65d78531acd1603fb82b985409bc63977bce03106564c0361d25f76d7da864" ||
		s.AmountAtoms != 606308751309 || s.FeeAtoms != 14020 ||
		s.Payee != "DsgYFgHVxYmCC9pVo5PRU8YrTsw7r1C226e" || s.BlockHeight != 1080576 ||
		s.Timestamp.UTC().Format("2006-01-02T15:04:05Z") != "2026-05-17T04:02:09Z" {
		t.Fatalf("spend %+v", s)
	}
	if flowMonth(f.time) != "2026-05" {
		t.Fatalf("month %s", flowMonth(f.time))
	}
}

func TestTreasuryBlockFlowsRefusesUpdatesItsTransactionsDoNotGive(t *testing.T) {
	block, updates := mainnetTSpendBlock(t)
	for name, change := range map[string]func([]int64) []int64{
		"fee off by one":  func(u []int64) []int64 { u[len(u)-1]--; return u },
		"missing output":  func(u []int64) []int64 { return slices.Delete(u, 5, 6) },
		"extra credit":    func(u []int64) []int64 { return append(u, 1) },
		"reordered spend": func(u []int64) []int64 { u[1], u[2] = u[2], u[1]; return u },
	} {
		t.Run(name, func(t *testing.T) {
			bad := change(slices.Clone(updates))
			if _, err := treasuryBlockFlows(1080576, "h", 0, bad, block, chaincfg.MainNetParams()); err == nil {
				t.Fatal("accepted updates the block does not give")
			}
		})
	}
}

// treasuryBase and tadd build transactions that pass dcrd's own stake checks.
func treasuryBase(atoms int64) *wire.MsgTx {
	tx := wire.NewMsgTx()
	tx.Version = wire.TxVersionTreasury
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{}, math.MaxUint32, wire.TxTreeRegular), 0, nil))
	tx.AddTxOut(wire.NewTxOut(atoms, []byte{txscript.OP_TADD}))
	tx.AddTxOut(wire.NewTxOut(0, append([]byte{txscript.OP_RETURN, txscript.OP_DATA_12}, make([]byte, 12)...)))
	return tx
}

func tadd(atoms int64) *wire.MsgTx {
	tx := wire.NewMsgTx()
	tx.Version = wire.TxVersionTreasury
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{1}, 0, wire.TxTreeRegular), atoms+2000, nil))
	tx.AddTxOut(wire.NewTxOut(atoms, []byte{txscript.OP_TADD}))
	return tx
}

func TestTreasuryBlockFlowsSeparatesContributionsFromTheBlockReward(t *testing.T) {
	block := &wire.MsgBlock{STransactions: []*wire.MsgTx{treasuryBase(54000000), tadd(250000000000)}}
	f, err := treasuryBlockFlows(1100000, "h", 1780000000, []int64{54000000, 250000000000}, block, chaincfg.MainNetParams())
	if err != nil {
		t.Fatal(err)
	}
	if f.tbase != 54000000 || len(f.tspends) != 0 || len(f.tadds) != 1 {
		t.Fatalf("flows %+v", f)
	}
	a := f.tadds[0]
	if a.AmountAtoms != 250000000000 || a.TxHash != tadd(250000000000).TxHash().String() || a.BlockHeight != 1100000 {
		t.Fatalf("contribution %+v", a)
	}
}

func TestTreasuryBlockFlowsTakesASingleUpdateAsTheTreasurybase(t *testing.T) {
	f, err := treasuryBlockFlows(1100001, "h", 1780000300, []int64{53075235}, nil, chaincfg.MainNetParams())
	if err != nil || f.tbase != 53075235 || len(f.tadds) != 0 || len(f.tspends) != 0 {
		t.Fatalf("flows %+v, %v", f, err)
	}
	for name, updates := range map[string][]int64{"negative": {-5}, "zero": {0}, "none": nil, "two": {1, 2}} {
		if _, err := treasuryBlockFlows(1100001, "h", 0, updates, nil, chaincfg.MainNetParams()); err == nil {
			t.Fatalf("%s: accepted without the block", name)
		}
	}
}
