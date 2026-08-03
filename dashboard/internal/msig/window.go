// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"dcrpulse/internal/services"
)

// HD wallet seams, mirroring the handshake ones.
var (
	createAccountSeam  = services.CreateAccount
	accountXpubSeam    = services.GetAccountExtendedPubKey
	syncBranchSeam     = services.SyncAccountAddressIndex
	accountsSeam       = services.FetchAllAccounts
	listUTXOsMultiSeam = func(ctx context.Context, addresses []string) ([]UTXO, error) {
		rows, err := services.ListSharedUTXOs(ctx, addresses)
		if err != nil {
			return nil, err
		}
		out := make([]UTXO, 0, len(rows))
		for _, r := range rows {
			out = append(out, UTXO{TxID: r.TxID, Vout: r.Vout, Tree: r.Tree, Atoms: r.Atoms, Address: r.Address, Confirmations: r.Confirmations})
		}
		return out, nil
	}
	// rescanSeam starts one deferred wallet rescan from the given height.
	// Wired at startup to the gRPC rescan starter; imports that can race
	// an already-confirmed funding (observation-class) schedule it, since
	// importscript with rescan=false only watches future transactions.
	rescanSeam = func(beginHeight int64) {
		log.Printf("msig: rescan from %d requested but no rescanner is wired", beginHeight)
	}
)

// SetRescanner wires the deferred-rescan starter. main.go injects the
// handlers' gRPC rescan helper here to avoid an import cycle.
func SetRescanner(fn func(beginHeight int64)) {
	if fn != nil {
		rescanSeam = fn
	}
}

// accountNameByNumber resolves a dcrwallet account name at call time;
// numbers are stable across renames, names are not.
func accountNameByNumber(ctx context.Context, number uint32) (string, error) {
	accounts, err := accountsSeam(ctx)
	if err != nil {
		return "", err
	}
	for _, a := range accounts {
		if a.AccountNumber == number {
			return a.AccountName, nil
		}
	}
	return "", fmt.Errorf("account %d does not exist in this wallet", number)
}

// ladderView memoizes address<->index mapping for one record's roster.
// Purely derived state: never persisted, invalidated by roster identity.
type ladderView struct {
	key     string
	byAddr  map[string]LadderEntry
	derived map[uint32]uint32 // branch -> derived-through
}

var (
	ladderMu    sync.Mutex
	ladderViews = map[string]*ladderView{}
)

// ladderFor returns the memoized view of a record's ladder, extended to
// cover both branches' imported windows plus the gap ahead.
func ladderFor(rec *WalletRecord) (*ladderView, error) {
	if !rec.HD || len(rec.Xpubs) == 0 {
		return nil, fmt.Errorf("not an HD shared wallet")
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return nil, err
	}
	key := rec.Network + "|" + strings.Join(rec.Xpubs, ",")
	ladderMu.Lock()
	defer ladderMu.Unlock()
	view, ok := ladderViews[key]
	if !ok {
		view = &ladderView{key: key, byAddr: map[string]LadderEntry{}, derived: map[uint32]uint32{}}
		ladderViews[key] = view
	}
	want := map[uint32]uint32{
		BranchExternal: windowEnd(rec.Ext, GapExt),
		BranchInternal: windowEnd(rec.Int, GapInt),
	}
	for branch, through := range want {
		if view.derived[branch] >= through {
			continue
		}
		entries, err := DeriveWindow(rec.M, rec.Xpubs, branch, view.derived[branch], through, params)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			view.byAddr[e.Address] = e
		}
		view.derived[branch] = through
	}
	return view, nil
}

func windowEnd(c *CursorState, gap uint32) uint32 {
	if c == nil {
		return gap
	}
	end := c.Next + gap
	if c.LastUsed+gap > end {
		end = c.LastUsed + gap
	}
	if c.ImportedThrough > end {
		end = c.ImportedThrough
	}
	return end
}

// entryAt derives a single rung without touching the memo.
func entryAt(rec *WalletRecord, branch, index uint32) (LadderEntry, error) {
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return LadderEntry{}, err
	}
	script, _, err := ScriptAt(rec.M, rec.Xpubs, branch, index, params)
	if err != nil {
		return LadderEntry{}, err
	}
	addr, err := P2SHAddress(script, params)
	if err != nil {
		return LadderEntry{}, err
	}
	return LadderEntry{Branch: branch, Index: index, Address: addr, Script: script}, nil
}

// cursorFor returns the branch's cursor, materializing the zero value.
func cursorFor(rec *WalletRecord, branch uint32) *CursorState {
	if branch == BranchInternal {
		if rec.Int == nil {
			rec.Int = &CursorState{}
		}
		return rec.Int
	}
	if rec.Ext == nil {
		rec.Ext = &CursorState{}
	}
	return rec.Ext
}

func gapFor(branch uint32) uint32 {
	if branch == BranchInternal {
		return GapInt
	}
	return GapExt
}

// ensureWindowImported imports every ladder script both branches need to
// reach their windows (cursor + gap, and past any observed use), then
// syncs the wallet's own branch indices so signing at any imported index
// works. ImportedThrough persists only after the imports succeed, so a
// crash can never leave the registry claiming coverage the wallet lacks;
// importscript is idempotent, so replays re-import harmlessly. These are
// pre-use imports: the addresses have never been revealed, so no rescan
// is needed. Observation-class callers schedule their own rescan.
func ensureWindowImported(ctx context.Context, store *Store, tempID string) error {
	rec, ok := store.Wallet(tempID)
	if !ok || !rec.HD {
		return fmt.Errorf("unknown HD shared wallet %s", tempID)
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return err
	}
	type branchPlan struct {
		branch  uint32
		from    uint32
		through uint32
	}
	plans := []branchPlan{
		{BranchExternal, cursorValue(rec.Ext).ImportedThrough, windowEnd(rec.Ext, GapExt)},
		{BranchInternal, cursorValue(rec.Int).ImportedThrough, windowEnd(rec.Int, GapInt)},
	}
	var acctName string
	imported := false
	for _, p := range plans {
		if p.from >= p.through {
			continue
		}
		entries, err := DeriveWindow(rec.M, rec.Xpubs, p.branch, p.from, p.through, params)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := importScriptSeam(ctx, fmt.Sprintf("%x", e.Script), false, 0); err != nil {
				return fmt.Errorf("import ladder script %d/%d: %v", e.Branch, e.Index, err)
			}
		}
		if acctName == "" && rec.OwnHD != nil {
			if acctName, err = accountNameByNumber(ctx, rec.OwnHD.Account); err != nil {
				return err
			}
		}
		if acctName != "" {
			if err := syncBranchSeam(ctx, acctName, p.branch, p.through); err != nil {
				return fmt.Errorf("sync own branch %d: %v", p.branch, err)
			}
		}
		imported = true
		branch := p.branch
		through := p.through
		if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
			c := cursorFor(r, branch)
			if c.ImportedThrough < through {
				c.ImportedThrough = through
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if imported {
		log.Printf("msig: %q ladder imported through ext %d / int %d", rec.Label,
			windowEnd(rec.Ext, GapExt), windowEnd(rec.Int, GapInt))
	}
	return nil
}

func cursorValue(c *CursorState) CursorState {
	if c == nil {
		return CursorState{}
	}
	return *c
}

// ErrGapExhausted refuses a fresh address while a full gap window of
// earlier ones is still unpaid; handing out further indices would put
// them beyond other members' imported windows.
var ErrGapExhausted = fmt.Errorf("this wallet has a full window of unpaid addresses; new ones unlock when one receives funds")

// AllocateReceiveAddress hands out the next external address: the cursor
// advances under the store lock (never reclaimed), the window import
// happens before the address is revealed.
func AllocateReceiveAddress(ctx context.Context, id string) (string, uint32, error) {
	network, err := networkSeam(ctx)
	if err != nil {
		return "", 0, err
	}
	store, rec := manager(network).Route(id, nil)
	if rec == nil || !rec.HD {
		return "", 0, fmt.Errorf("unknown HD shared wallet %s", id)
	}
	if err := requireActive(store, rec); err != nil {
		return "", 0, err
	}
	var index uint32
	err = store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		c := cursorFor(r, BranchExternal)
		if c.Next >= c.LastUsed+GapExt {
			return ErrGapExhausted
		}
		index = c.Next
		c.Next++
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	if err := ensureWindowImported(ctx, store, rec.TempID); err != nil {
		return "", 0, err
	}
	rec, _ = store.Wallet(rec.TempID)
	entry, err := entryAt(rec, BranchExternal, index)
	if err != nil {
		return "", 0, err
	}
	return entry.Address, index, nil
}

// allocateChangeIndex reserves the next internal index for a proposal's
// change output, importing ahead before the transaction is built.
func allocateChangeIndex(ctx context.Context, store *Store, rec *WalletRecord) (uint32, string, error) {
	var index uint32
	err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		c := cursorFor(r, BranchInternal)
		index = c.Next
		c.Next++
		return nil
	})
	if err != nil {
		return 0, "", err
	}
	if err := ensureWindowImported(ctx, store, rec.TempID); err != nil {
		return 0, "", err
	}
	fresh, _ := store.Wallet(rec.TempID)
	entry, err := entryAt(fresh, BranchInternal, index)
	if err != nil {
		return 0, "", err
	}
	return index, entry.Address, nil
}

// windowAddresses lists every imported ladder address, the only set that
// may be handed to listunspent (the wallet fails the whole call on an
// unknown address).
func windowAddresses(rec *WalletRecord) ([]string, error) {
	view, err := ladderFor(rec)
	if err != nil {
		return nil, err
	}
	addrs := make([]string, 0, len(view.byAddr))
	extThrough := cursorValue(rec.Ext).ImportedThrough
	intThrough := cursorValue(rec.Int).ImportedThrough
	for addr, e := range view.byAddr {
		switch e.Branch {
		case BranchExternal:
			if e.Index < extThrough {
				addrs = append(addrs, addr)
			}
		case BranchInternal:
			if e.Index < intThrough {
				addrs = append(addrs, addr)
			}
		}
	}
	return addrs, nil
}

// listWindowUTXOs returns the wallet's view of every unspent ladder
// output. When the wallet reports an unknown address (a fresh seed
// restore, or a crashed import), the whole window is re-imported once
// and the call retried; a second failure points at the backup card.
func listWindowUTXOs(ctx context.Context, store *Store, rec *WalletRecord) ([]UTXO, error) {
	addrs, err := windowAddresses(rec)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, nil
	}
	utxos, err := listUTXOsMultiSeam(ctx, addrs)
	if err == nil {
		return utxos, nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "address") {
		return nil, err
	}
	if rerr := reimportWindow(ctx, store, rec); rerr != nil {
		return nil, fmt.Errorf("%v; restore this shared wallet from its backup card", rerr)
	}
	utxos, err = listUTXOsMultiSeam(ctx, addrs)
	if err != nil {
		return nil, fmt.Errorf("%v; restore this shared wallet from its backup card", err)
	}
	return utxos, nil
}

// reimportWindow idempotently re-imports every script the registry
// claims coverage for, healing a wallet.db that lost them.
func reimportWindow(ctx context.Context, store *Store, rec *WalletRecord) error {
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return err
	}
	for _, b := range []struct {
		branch  uint32
		through uint32
	}{
		{BranchExternal, cursorValue(rec.Ext).ImportedThrough},
		{BranchInternal, cursorValue(rec.Int).ImportedThrough},
	} {
		entries, err := DeriveWindow(rec.M, rec.Xpubs, b.branch, 0, b.through, params)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := importScriptSeam(ctx, fmt.Sprintf("%x", e.Script), false, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

// observeUsage advances the branch cursors past every on-chain-used
// index and tops the windows up. An import forced by observation may
// postdate the funding it needs to see, so one deferred rescan is
// scheduled, bounded by the wallet's creation height.
func observeUsage(ctx context.Context, store *Store, rec *WalletRecord, utxos []UTXO) {
	view, err := ladderFor(rec)
	if err != nil {
		return
	}
	extUsed := cursorValue(rec.Ext).LastUsed
	intUsed := cursorValue(rec.Int).LastUsed
	advanced := false
	for _, u := range utxos {
		e, ok := view.byAddr[u.Address]
		if !ok {
			continue
		}
		switch e.Branch {
		case BranchExternal:
			if e.Index >= extUsed {
				extUsed = e.Index + 1
				advanced = true
			}
		case BranchInternal:
			if e.Index >= intUsed {
				intUsed = e.Index + 1
				advanced = true
			}
		}
	}
	if !advanced {
		return
	}
	prevExtImported := cursorValue(rec.Ext).ImportedThrough
	prevIntImported := cursorValue(rec.Int).ImportedThrough
	if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		ec := cursorFor(r, BranchExternal)
		if ec.LastUsed < extUsed {
			ec.LastUsed = extUsed
		}
		if ec.Next < extUsed {
			ec.Next = extUsed
		}
		ic := cursorFor(r, BranchInternal)
		if ic.LastUsed < intUsed {
			ic.LastUsed = intUsed
		}
		if ic.Next < intUsed {
			ic.Next = intUsed
		}
		return nil
	}); err != nil {
		return
	}
	if err := ensureWindowImported(ctx, store, rec.TempID); err != nil {
		log.Printf("msig: window top-up for %q: %v", rec.Label, err)
		return
	}
	// Usage at or beyond the previously imported edge means a peer
	// disclosed addresses this wallet imported only now; whatever funded
	// them predates the import and needs the one deferred rescan.
	if extUsed > prevExtImported || intUsed > prevIntImported {
		from := rec.CreatedHeight - 1
		if from < 0 {
			from = 0
		}
		log.Printf("msig: %q used indices beyond the imported window; rescanning from %d", rec.Label, from)
		rescanSeam(from)
	}
}

// ReceiveInfo reports the wallet's current receive address without
// allocating: the most recently handed-out external index, or index 0
// before any hand-out. Exhausted warns that AllocateReceiveAddress would
// refuse.
func ReceiveInfo(ctx context.Context, id string) (string, uint32, bool, error) {
	network, err := networkSeam(ctx)
	if err != nil {
		return "", 0, false, err
	}
	_, rec := manager(network).Route(id, nil)
	if rec == nil || !rec.HD {
		return "", 0, false, fmt.Errorf("unknown HD shared wallet %s", id)
	}
	if rec.Status != StatusActive || rec.Address == "" {
		return "", 0, false, fmt.Errorf("this shared wallet is not active yet")
	}
	ext := cursorValue(rec.Ext)
	index := uint32(0)
	if ext.Next > 0 {
		index = ext.Next - 1
	}
	entry, err := entryAt(rec, BranchExternal, index)
	if err != nil {
		return "", 0, false, err
	}
	return entry.Address, index, ext.Next >= ext.LastUsed+GapExt, nil
}

// WindowUTXOs lists the unspent outputs across the wallet's imported
// ladder window.
func WindowUTXOs(ctx context.Context, id string) ([]UTXO, error) {
	network, err := networkSeam(ctx)
	if err != nil {
		return nil, err
	}
	store, rec := manager(network).Route(id, nil)
	if rec == nil || !rec.HD {
		return nil, fmt.Errorf("unknown HD shared wallet %s", id)
	}
	return listWindowUTXOs(ctx, store, rec)
}
