// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"dcrpulse/internal/types"
)

// fakeSwapSource pages like bisonw: orders newest first with an exclusive
// offset, tx history newest first with an inclusive cursor.
type fakeSwapSource struct {
	orders      []*bwOrder
	txs         []rawWalletTx
	err         error
	ordersCalls int
	orderCalls  []string
	txCalls     int
}

func (f *fakeSwapSource) Orders(_ context.Context, filter map[string]any) (json.RawMessage, error) {
	f.ordersCalls++
	if f.err != nil {
		return nil, f.err
	}
	start := 0
	if off, _ := filter["offset"].(string); off != "" {
		for i, o := range f.orders {
			if o.ID == off {
				start = i + 1
			}
		}
	}
	end := min(start+filter["n"].(int), len(f.orders))
	return json.Marshal(f.orders[start:end])
}

func (f *fakeSwapSource) Order(_ context.Context, id string) (json.RawMessage, error) {
	f.orderCalls = append(f.orderCalls, id)
	for _, o := range f.orders {
		if o.ID == id {
			return json.Marshal(o)
		}
	}
	return json.RawMessage("null"), nil
}

func (f *fakeSwapSource) TxHistory(_ context.Context, _ uint32, num int, refID string, _ bool) (json.RawMessage, error) {
	f.txCalls++
	start := 0
	if refID != "" {
		for i, t := range f.txs {
			if t.ID == refID {
				start = i
			}
		}
	}
	end := min(start+num, len(f.txs))
	return json.Marshal(f.txs[start:end])
}

// useSwapSource installs src behind an unlocked DEX for one wallet profile
// and a fresh cache, restoring the real seams afterwards.
func useSwapSource(t *testing.T, src dexSwapSource, unlocked bool) *any {
	t.Helper()
	oldUnlocked, oldProfile, oldNew, oldCache := dexSwapUnlocked, dexSwapProfile, dexSwapNewSource, dexSwaps
	profile := any("wallet-a")
	dexSwapUnlocked = func() bool { return unlocked }
	dexSwapProfile = func() any { return profile }
	dexSwapNewSource = func() (dexSwapSource, error) { return src, nil }
	dexSwaps = &dexSwapCache{}
	t.Cleanup(func() {
		dexSwapUnlocked, dexSwapProfile, dexSwapNewSource, dexSwaps = oldUnlocked, oldProfile, oldNew, oldCache
	})
	return &profile
}

func dcrCoin(txid string, vout string) *bwCoin {
	return &bwCoin{StringID: txid + ":" + vout, AssetID: dexSwapAsset}
}

func btcCoin(txid string) *bwCoin {
	return &bwCoin{StringID: txid + ":0", AssetID: 0}
}

const (
	execStatus   = 3
	bookedStatus = 2
	cancelStatus = 4
	confirmed    = 5
)

func sellOrder(id string, stamp uint64, status uint16, funding *bwCoin, matches ...*bwMatch) *bwOrder {
	o := &bwOrder{Host: "dex.example:7232", ID: id, BaseID: 42, QuoteID: 0, Sell: true, Stamp: stamp, Status: status, Matches: matches}
	if funding != nil {
		o.FundingCoins = []*bwCoin{funding}
	}
	return o
}

func swapMatch(swap *bwCoin, stamp, rate uint64) *bwMatch {
	return &bwMatch{Status: confirmed, Qty: 40e8, Rate: rate, Swap: swap, Stamp: stamp}
}

func walletRows(txids ...string) []types.Transaction {
	rows := make([]types.Transaction, len(txids))
	for i, id := range txids {
		rows[i] = types.Transaction{TxID: id, TxType: "regular"}
	}
	return rows
}

// bisonw's records label each DCR tx it made for a trade, and fold a split
// into the earliest swap of the orders it funded.
func TestBuildDexSwapIndex(t *testing.T) {
	now := uint64(time.Now().UnixMilli())
	at := func(minutesAgo uint64) uint64 { return now - minutesAgo*60_000 }
	txs := map[string]rawWalletTx{
		"split-a":  {Type: dexSwapTxSplit, ID: "split-a", Fees: 2510},
		"split-c":  {Type: dexSwapTxSplit, ID: "split-c", Fees: 2520},
		"split-d":  {Type: dexSwapTxSplit, ID: "split-d", Fees: 4150},
		"recv-f":   {Type: 2, ID: "recv-f"},
		"swap-a1":  {Type: 3, ID: "swap-a1"},
		"swap-b":   {Type: 3, ID: "swap-b"},
		"redeem-g": {Type: 4, ID: "redeem-g"},
	}
	orders := map[string]*bwOrder{
		// Two swaps of one standing order: the split folds into the earlier.
		"a": sellOrder("a", at(90), bookedStatus, dcrCoin("split-a", "0"),
			swapMatch(dcrCoin("swap-a2", "0"), at(10), 17110),
			swapMatch(dcrCoin("swap-a1", "0"), at(60), 17120)),
		// Three matches settled by one tx.
		"b": sellOrder("b", at(80), execStatus, nil,
			swapMatch(dcrCoin("swap-b", "0"), at(70), 17120),
			swapMatch(dcrCoin("swap-b", "1"), at(70), 17110),
			swapMatch(dcrCoin("swap-b", "2"), at(70), 17060)),
		// On the book, unmatched: its split stays a row of its own.
		"c": sellOrder("c", at(50), bookedStatus, dcrCoin("split-c", "0")),
		// One split funding a canceled order and an executed one.
		"d1": sellOrder("d1", at(40), cancelStatus, dcrCoin("split-d", "1")),
		"d2": sellOrder("d2", at(41), execStatus, dcrCoin("split-d", "0"),
			swapMatch(dcrCoin("swap-d2", "0"), at(30), 17000)),
		// Funded by a plain receive: never touched.
		"f": sellOrder("f", at(20), execStatus, dcrCoin("recv-f", "0"),
			swapMatch(dcrCoin("swap-f", "0"), at(15), 16900)),
		// A DCR buy: our swap is BTC, our redeem is DCR.
		"g": {ID: "g", Host: "dex.example:7232", BaseID: 42, QuoteID: 0, Stamp: at(12), Status: execStatus,
			Matches: []*bwMatch{{Status: confirmed, Qty: 10e8, Rate: 17000, Stamp: at(11),
				Swap: btcCoin("btc-swap"), Redeem: dcrCoin("redeem-g", "0")}}},
		// Our swap timed out and came back.
		"h": sellOrder("h", at(9), execStatus, nil,
			&bwMatch{Status: 2, Qty: 40e8, Rate: 17000, Stamp: at(8), Revoked: true,
				Swap: dcrCoin("swap-h", "0"), Refund: dcrCoin("refund-h", "0")}),
	}
	idx := buildDexSwapIndex(orders, txs)

	a1 := idx["swap-a1"]
	if a1 == nil || a1.Role != "swap" || a1.FundingTxID != "split-a" || a1.FundingFee != 0.0000251 {
		t.Fatalf("swap-a1 = %+v, want a swap carrying split-a's 0.0000251 fee", a1)
	}
	if a1.BaseSymbol != "dcr" || a1.QuoteSymbol != "btc" || a1.BaseConv != 1e8 || a1.QuoteConv != 1e8 || !a1.Sell {
		t.Errorf("swap-a1 market = %+v, want a dcr/btc sell with both factors", a1)
	}
	if a2 := idx["swap-a2"]; a2 == nil || a2.FundingTxID != "" {
		t.Errorf("swap-a2 = %+v, want a swap without the split", a2)
	}
	if s := idx["split-a"]; s == nil || s.Role != "funding" || s.FoldedInto != "swap-a1" {
		t.Errorf("split-a = %+v, want folded into swap-a1", s)
	}
	if b := idx["swap-b"]; b == nil || len(b.Matches) != 3 || b.Matches[2].Rate != 17060 || b.Matches[0].Status != "MatchConfirmed" {
		t.Errorf("swap-b = %+v, want three confirmed matches", b)
	}
	if c := idx["split-c"]; c == nil || c.Role != "funding" || c.FoldedInto != "" || c.OrderID != "c" {
		t.Errorf("split-c = %+v, want a visible funding row for order c", c)
	}
	if d := idx["split-d"]; d == nil || d.FoldedInto != "swap-d2" || idx["swap-d2"].FundingFee != 0.0000415 {
		t.Errorf("split-d = %+v, want folded into swap-d2", d)
	}
	if f, ok := idx["recv-f"]; ok {
		t.Errorf("recv-f = %+v, want a plain receive left alone", f)
	}
	if f := idx["swap-f"]; f == nil || f.FundingTxID != "" {
		t.Errorf("swap-f = %+v, want no funding attached", f)
	}
	if g := idx["redeem-g"]; g == nil || g.Role != "redeem" || g.Sell {
		t.Errorf("redeem-g = %+v, want a redeem of a buy", g)
	}
	if _, ok := idx["btc-swap"]; ok {
		t.Error("a BTC coin was labelled as a DCR tx")
	}
	if h := idx["refund-h"]; h == nil || h.Role != "refund" {
		t.Errorf("refund-h = %+v, want a refund", h)
	}
	if h := idx["swap-h"]; h == nil || !h.Matches[0].Refunded || !h.Matches[0].Revoked {
		t.Errorf("swap-h = %+v, want a revoked match that was refunded", h)
	}
}

// With the DEX locked nothing is asked of bisonw and the rows stay as they are.
func TestAnnotateDexSwapsLockedDoesNothing(t *testing.T) {
	src := &fakeSwapSource{orders: []*bwOrder{sellOrder("a", 1, execStatus, nil, swapMatch(dcrCoin("swap", "0"), 1, 17000))}}
	useSwapSource(t, src, false)
	rows := walletRows("swap")
	annotateDexSwaps(context.Background(), rows)
	if rows[0].DexSwap != nil || src.ordersCalls+src.txCalls+len(src.orderCalls) != 0 {
		t.Errorf("locked: row %+v after %d calls, want untouched and no calls", rows[0].DexSwap, src.ordersCalls+src.txCalls)
	}
}

// When bisonw cannot be read the rows stay as they are, also once it had
// answered before.
func TestAnnotateDexSwapsUnreachableDoesNothing(t *testing.T) {
	now := uint64(time.Now().UnixMilli())
	src := &fakeSwapSource{err: errors.New("connection refused")}
	useSwapSource(t, src, true)
	rows := walletRows("swap")
	annotateDexSwaps(context.Background(), rows)
	if rows[0].DexSwap != nil {
		t.Fatalf("unreachable: row = %+v, want untouched", rows[0].DexSwap)
	}

	src.err = nil
	src.orders = []*bwOrder{sellOrder("a", now, execStatus, nil, swapMatch(dcrCoin("swap", "0"), now, 17000))}
	dexSwaps.mu.Lock()
	dexSwaps.builtAt = time.Time{}
	dexSwaps.mu.Unlock()
	rows = walletRows("swap")
	annotateDexSwaps(context.Background(), rows)
	if rows[0].DexSwap == nil {
		t.Fatal("reachable again: swap not labelled")
	}

	src.err = errors.New("connection refused")
	dexSwaps.mu.Lock()
	dexSwaps.builtAt = time.Time{}
	dexSwaps.mu.Unlock()
	dexSwaps.index(context.Background())
	dexSwaps.mu.Lock()
	running := dexSwaps.refreshing
	dexSwaps.mu.Unlock()
	if running != nil {
		<-running
	}
	rows = walletRows("swap")
	annotateDexSwaps(context.Background(), rows)
	if rows[0].DexSwap != nil {
		t.Errorf("down again: row = %+v, want the old label dropped", rows[0].DexSwap)
	}
}

func TestAnnotateDexSwapsLabelsRegularRows(t *testing.T) {
	now := uint64(time.Now().UnixMilli())
	src := &fakeSwapSource{
		orders: []*bwOrder{sellOrder("a", now, execStatus, nil, swapMatch(dcrCoin("swap", "0"), now, 17000))},
	}
	useSwapSource(t, src, true)
	rows := walletRows("swap", "other")
	rows = append(rows, types.Transaction{TxID: "swap", TxType: "ticket"})
	annotateDexSwaps(context.Background(), rows)
	if rows[0].DexSwap == nil || rows[0].DexSwap.Role != "swap" {
		t.Errorf("swap row = %+v, want labelled", rows[0].DexSwap)
	}
	if rows[1].DexSwap != nil || rows[2].DexSwap != nil {
		t.Errorf("other rows = %+v / %+v, want untouched", rows[1].DexSwap, rows[2].DexSwap)
	}
}

// A refresh reads only what can still change: new orders above the first
// final one, and the known orders still in flight. The tx history cursor
// repeats its own tx and must not loop.
func TestDexSwapRefreshReadsOnlyWhatCanChange(t *testing.T) {
	now := uint64(time.Now().UnixMilli())
	final := sellOrder("final", now-3, execStatus, nil, swapMatch(dcrCoin("swap-final", "0"), now-3, 17000))
	booked := sellOrder("booked", now-4, bookedStatus, nil)
	src := &fakeSwapSource{orders: []*bwOrder{final, booked}}
	for i := 0; i < dexSwapTxPage+5; i++ {
		src.txs = append(src.txs, rawWalletTx{ID: string(rune('a'+i%26)) + string(rune('a'+i/26)), Type: 1})
	}
	useSwapSource(t, src, true)
	annotateDexSwaps(context.Background(), walletRows("swap-final"))
	if got := len(dexSwaps.state.txs); got != dexSwapTxPage+5 {
		t.Fatalf("first build read %d txs, want %d", got, dexSwapTxPage+5)
	}

	// A new swap on the booked order, which bisonw lists below the final one,
	// and three new txs on top of the history.
	booked.Status = execStatus
	booked.Matches = []*bwMatch{swapMatch(dcrCoin("swap-booked", "0"), now, 17100)}
	src.txs = append([]rawWalletTx{{ID: "new1"}, {ID: "new2"}, {ID: "new3"}}, src.txs...)
	src.ordersCalls, src.orderCalls, src.txCalls = 0, nil, 0
	dexSwaps.mu.Lock()
	dexSwaps.builtAt = time.Time{}
	dexSwaps.mu.Unlock()
	dexSwaps.index(context.Background()) // stale: starts a refresh, serves the old labels
	dexSwaps.mu.Lock()
	running := dexSwaps.refreshing
	dexSwaps.mu.Unlock()
	if running == nil {
		t.Fatal("a stale index started no refresh")
	}
	<-running
	idx := dexSwaps.index(context.Background())
	if src.ordersCalls != 1 || len(src.orderCalls) != 1 || src.orderCalls[0] != "booked" {
		t.Errorf("refresh made %d page calls and re-read %v, want 1 page and only the booked order", src.ordersCalls, src.orderCalls)
	}
	if idx["swap-booked"] == nil {
		t.Error("the booked order's new swap is missing after the refresh")
	}
	if _, ok := dexSwaps.state.txs["new3"]; src.txCalls != 1 || !ok {
		t.Errorf("refresh read %d tx history pages (new3 read: %v), want 1 page with the new txs", src.txCalls, ok)
	}
}

// Another wallet's trades never label this wallet's rows.
func TestDexSwapCacheResetsOnWalletSwitch(t *testing.T) {
	now := uint64(time.Now().UnixMilli())
	src := &fakeSwapSource{orders: []*bwOrder{sellOrder("a", now, execStatus, nil, swapMatch(dcrCoin("swap", "0"), now, 17000))}}
	profile := useSwapSource(t, src, true)
	rows := walletRows("swap")
	annotateDexSwaps(context.Background(), rows)
	if rows[0].DexSwap == nil {
		t.Fatal("wallet a: swap not labelled")
	}
	*profile = "wallet-b"
	src.orders = []*bwOrder{sellOrder("b", now, execStatus, nil, swapMatch(dcrCoin("swap-b", "0"), now, 17000))}
	rows = walletRows("swap", "swap-b")
	annotateDexSwaps(context.Background(), rows)
	if rows[0].DexSwap != nil {
		t.Errorf("wallet b: row = %+v, want wallet a's label gone", rows[0].DexSwap)
	}
	if rows[1].DexSwap == nil {
		t.Error("wallet b: its own swap is not labelled")
	}
}
