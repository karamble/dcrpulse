// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/dexassets"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
	"dcrpulse/pkg/bisonw"
)

// Wallet history rows that bisonw made for DEX trades are labelled from
// bisonw's own records: its order archive links each swap, redeem and refund
// to its match, and its DCR tx history says which funding txs are splits.
// Nothing is inferred from the chain. While the DEX is locked or bisonw is
// unreachable the rows are left exactly as they are.

const (
	dexSwapAsset      = 42 // DCR
	dexSwapTTL        = 15 * time.Second
	dexSwapWait       = 3 * time.Second
	dexSwapBuildLimit = 60 * time.Second
	dexSwapOrderPage  = 50
	dexSwapMaxOrders  = 2000
	dexSwapTxPage     = 100
	dexSwapMaxTxs     = 4000
	dexSwapTxSplit    = 6 // asset.Split
)

// dexSwapSource is the bisonw surface the labels are read from.
type dexSwapSource interface {
	Orders(ctx context.Context, filter map[string]any) (json.RawMessage, error)
	Order(ctx context.Context, id string) (json.RawMessage, error)
	TxHistory(ctx context.Context, assetID uint32, num int, refID string, past bool) (json.RawMessage, error)
}

type bisonwSwapSource struct {
	web *bisonw.WebClient
	rpc *bisonw.Client
}

func (s bisonwSwapSource) Orders(ctx context.Context, filter map[string]any) (json.RawMessage, error) {
	return s.web.Orders(ctx, filter)
}

func (s bisonwSwapSource) Order(ctx context.Context, id string) (json.RawMessage, error) {
	return s.web.Order(ctx, id)
}

func (s bisonwSwapSource) TxHistory(ctx context.Context, assetID uint32, num int, refID string, past bool) (json.RawMessage, error) {
	return s.rpc.TxHistory(ctx, assetID, num, refID, past)
}

// Seams the tests replace.
var (
	dexSwapUnlocked  = rpc.DcrdexUnlocked
	dexSwapProfile   = func() any { return rpc.WalletGrpcClient }
	dexSwapNewSource = func() (dexSwapSource, error) {
		web, err := rpc.DcrdexWebClient()
		if err != nil {
			return nil, err
		}
		cl, err := rpc.DcrdexClient()
		if err != nil {
			return nil, err
		}
		return bisonwSwapSource{web: web, rpc: cl}, nil
	}
	dexSwaps = &dexSwapCache{}
)

// annotateDexSwaps labels the rows bisonw made for DEX trades. It does
// nothing while the DEX is locked, and leaves every row as it is when bisonw
// cannot be read.
func annotateDexSwaps(ctx context.Context, txs []types.Transaction) {
	if len(txs) == 0 || !dexSwapUnlocked() {
		return
	}
	idx := dexSwaps.index(ctx)
	for i := range txs {
		if txs[i].TxType != "regular" {
			continue
		}
		if e, ok := idx[txs[i].TxID]; ok {
			txs[i].DexSwap = e
		}
	}
}

// dexSwapState is what one wallet profile has read from bisonw so far. Only
// the refresh in flight touches it; a wallet switch replaces it.
type dexSwapState struct {
	profile    any
	orders     map[string]*bwOrder
	final      map[string]bool
	txs        map[string]rawWalletTx
	ordersDone bool
	txsDone    bool
}

func newDexSwapState(profile any) *dexSwapState {
	return &dexSwapState{
		profile: profile,
		orders:  make(map[string]*bwOrder),
		final:   make(map[string]bool),
		txs:     make(map[string]rawWalletTx),
	}
}

type dexSwapCache struct {
	mu         sync.Mutex
	state      *dexSwapState
	idx        map[string]*types.TxDexSwap
	builtAt    time.Time
	refreshing chan struct{}
}

// index returns the current labels, starting a refresh when they are stale.
// Only the call that starts the first build waits for it, and at most
// dexSwapWait; every other call returns what is there.
func (c *dexSwapCache) index(ctx context.Context) map[string]*types.TxDexSwap {
	profile := dexSwapProfile()
	c.mu.Lock()
	if c.state == nil || c.state.profile != profile {
		c.state = newDexSwapState(profile)
		c.idx, c.builtAt = nil, time.Time{}
	}
	var wait chan struct{}
	if c.refreshing == nil && time.Since(c.builtAt) >= dexSwapTTL {
		src, err := dexSwapNewSource()
		if err != nil {
			c.idx, c.builtAt = nil, time.Now()
			c.mu.Unlock()
			return nil
		}
		done := make(chan struct{})
		c.refreshing = done
		go c.refresh(c.state, src, done)
		if c.idx == nil {
			wait = done
		}
	}
	c.mu.Unlock()

	if wait != nil {
		t := time.NewTimer(dexSwapWait)
		defer t.Stop()
		select {
		case <-wait:
		case <-t.C:
		case <-ctx.Done():
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == nil || c.state.profile != profile {
		return nil
	}
	return c.idx
}

// refresh reads bisonw and rebuilds the labels. A failed read clears them, so
// rows show plain until bisonw answers again.
func (c *dexSwapCache) refresh(st *dexSwapState, src dexSwapSource, done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), dexSwapBuildLimit)
	defer cancel()
	err := st.fetchOrders(ctx, src)
	if err == nil {
		err = st.fetchTxs(ctx, src)
	}
	var idx map[string]*types.TxDexSwap
	if err == nil {
		idx = buildDexSwapIndex(st.orders, st.txs)
	}
	c.mu.Lock()
	if c.state == st {
		c.idx, c.builtAt = idx, time.Now()
	}
	c.refreshing = nil
	c.mu.Unlock()
	close(done)
	if err != nil {
		dexcLog.Debugf("Wallet history DEX labels unavailable: %v", err)
	}
}

// fetchOrders pages bisonw's DCR orders, most recently updated first. The
// first build reads back to dexSwapMaxOrders; later refreshes stop at the
// first order already known final, then re-read the known orders still in
// flight. A final order never changes, so it is read once.
func (st *dexSwapState) fetchOrders(ctx context.Context, src dexSwapSource) error {
	seen := make(map[string]bool)
	offset := ""
	for fetched := 0; fetched < dexSwapMaxOrders; {
		filter := map[string]any{"n": dexSwapOrderPage, "assets": []uint32{dexSwapAsset}}
		if offset != "" {
			filter["offset"] = offset
		}
		raw, err := src.Orders(ctx, filter)
		if err != nil {
			return err
		}
		var page []*bwOrder
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &page); err != nil {
				return err
			}
		}
		stop := len(page) < dexSwapOrderPage
		for _, o := range page {
			if o == nil {
				continue
			}
			if st.ordersDone && st.final[o.ID] {
				stop = true
				break
			}
			st.orders[o.ID] = o
			st.final[o.ID] = dexOrderFinal(o)
			seen[o.ID] = true
		}
		fetched += len(page)
		if stop {
			break
		}
		offset = page[len(page)-1].ID
	}
	st.ordersDone = true

	var inFlight []string
	for id := range st.orders {
		if !st.final[id] && !seen[id] {
			inFlight = append(inFlight, id)
		}
	}
	for _, id := range inFlight {
		raw, err := src.Order(ctx, id)
		if err != nil {
			return err
		}
		var o *bwOrder
		if err := json.Unmarshal(raw, &o); err != nil {
			return err
		}
		if o != nil {
			st.orders[id] = o
			st.final[id] = dexOrderFinal(o)
		}
	}
	return nil
}

// fetchTxs pages bisonw's DCR tx history, newest first, for the split types.
// The first build reads back to dexSwapMaxTxs; later refreshes stop at the
// first page that reaches a known tx. A tx's type never changes.
func (st *dexSwapState) fetchTxs(ctx context.Context, src dexSwapSource) error {
	ref := ""
	for fetched := 0; fetched < dexSwapMaxTxs; {
		raw, err := src.TxHistory(ctx, dexSwapAsset, dexSwapTxPage, ref, ref != "")
		if err != nil {
			return err
		}
		var page []rawWalletTx
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &page); err != nil {
				return err
			}
		}
		fresh, known := 0, false
		for _, t := range page {
			if t.ID == ref {
				continue // the cursor tx starts the next page too
			}
			if _, ok := st.txs[t.ID]; ok {
				known = true
				continue
			}
			st.txs[t.ID] = t
			fresh++
		}
		fetched += fresh
		if fresh == 0 || len(page) < dexSwapTxPage || (st.txsDone && known) {
			break
		}
		ref = page[len(page)-1].ID
	}
	st.txsDone = true
	return nil
}

// dexOrderFinal reports whether an order can no longer change: executed,
// canceled or revoked, with no match still settling.
func dexOrderFinal(o *bwOrder) bool {
	if o.Status < dexStatusByName["executed"] {
		return false
	}
	for _, m := range o.Matches {
		if m != nil && m.Active {
			return false
		}
	}
	return true
}

// dexCoinTxID returns the txid of a DCR coin, or "" for another asset's coin.
func dexCoinTxID(c *bwCoin) string {
	if c == nil || c.AssetID != dexSwapAsset {
		return ""
	}
	txid, _, _ := strings.Cut(c.StringID, ":")
	return txid
}

func dexSwapEntry(o *bwOrder, role string) *types.TxDexSwap {
	return &types.TxDexSwap{
		Role:        role,
		OrderID:     o.ID,
		Host:        o.Host,
		BaseID:      o.BaseID,
		QuoteID:     o.QuoteID,
		BaseSymbol:  dexassets.Symbol(o.BaseID),
		QuoteSymbol: dexassets.Symbol(o.QuoteID),
		BaseConv:    dexassets.ConvFactor(o.BaseID),
		QuoteConv:   dexassets.ConvFactor(o.QuoteID),
		Sell:        o.Sell,
	}
}

// buildDexSwapIndex maps each DCR txid bisonw made for a trade to its label.
// A split folds into the earliest swap of the orders it funded; a split with
// no swap yet keeps its own row.
func buildDexSwapIndex(orders map[string]*bwOrder, txs map[string]rawWalletTx) map[string]*types.TxDexSwap {
	ids := make([]string, 0, len(orders))
	for id := range orders {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := orders[ids[i]], orders[ids[j]]
		if a.Stamp != b.Stamp {
			return a.Stamp < b.Stamp
		}
		return a.ID < b.ID
	})

	type swapAt struct {
		txid  string
		stamp uint64
	}
	earlier := func(a swapAt, b swapAt) bool {
		return a.stamp < b.stamp || (a.stamp == b.stamp && a.txid < b.txid)
	}
	idx := make(map[string]*types.TxDexSwap)
	firstSwap := make(map[string]swapAt)
	for _, id := range ids {
		o := orders[id]
		for _, m := range o.Matches {
			if m == nil || m.IsCancel {
				continue
			}
			for _, c := range []struct {
				coin *bwCoin
				role string
			}{{m.Swap, "swap"}, {m.Redeem, "redeem"}, {m.Refund, "refund"}} {
				txid := dexCoinTxID(c.coin)
				if txid == "" {
					continue
				}
				e := idx[txid]
				if e == nil {
					e = dexSwapEntry(o, c.role)
					idx[txid] = e
				}
				e.Matches = append(e.Matches, types.TxDexMatch{
					Qty:      m.Qty,
					Rate:     m.Rate,
					Status:   dexMatchStatusNames[m.Status],
					Active:   m.Active,
					Revoked:  m.Revoked,
					Refunded: m.Refund != nil && m.Refund.StringID != "",
				})
				if c.role == "swap" {
					at := swapAt{txid, m.Stamp}
					if fs, ok := firstSwap[o.ID]; !ok || earlier(at, fs) {
						firstSwap[o.ID] = at
					}
				}
			}
		}
	}

	funded := make(map[string][]*bwOrder)
	var splits []string
	for _, id := range ids {
		o := orders[id]
		for _, c := range o.FundingCoins {
			txid := dexCoinTxID(c)
			if txid == "" {
				continue
			}
			if t, ok := txs[txid]; !ok || t.Type != dexSwapTxSplit {
				continue
			}
			if _, ok := funded[txid]; !ok {
				splits = append(splits, txid)
			}
			funded[txid] = append(funded[txid], o)
		}
	}
	for _, split := range splits {
		if _, taken := idx[split]; taken {
			continue
		}
		var host swapAt
		var hostOrder *bwOrder
		for _, o := range funded[split] {
			if fs, ok := firstSwap[o.ID]; ok && (hostOrder == nil || earlier(fs, host)) {
				host, hostOrder = fs, o
			}
		}
		if hostOrder != nil && idx[host.txid].FundingTxID == "" {
			e := dexSwapEntry(hostOrder, "funding")
			e.FoldedInto = host.txid
			idx[split] = e
			h := idx[host.txid]
			h.FundingTxID = split
			h.FundingFee = convWalletTx(dexSwapAsset, txs[split]).Fees
			continue
		}
		idx[split] = dexSwapEntry(funded[split][0], "funding")
	}
	return idx
}
