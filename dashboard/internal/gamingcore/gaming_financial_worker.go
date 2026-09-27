package gamingcore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dcrpulse/internal/gamingfunds"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var gamingFinancialWorker struct {
	sync.Mutex
	running bool
}
var gamingFinancialInbox = make(chan GamingFrameEvent, 128)

// observeGamingOperation asks dcrd first, which answers for the mempool and,
// with the transaction index, for anything mined.
//
// The index is required, not optional: the gaming bridge will not switch on
// without it. Without it dcrd answers this with ErrRPCInternal rather than
// ErrRPCNoTxInfo, so the fallback below never runs and reconciliation stops
// before it can broadcast - a payout every seat signed then sits at publishing
// for good.
//
// The dcrwallet fallback is a backstop for the wallet's own transactions, and
// nothing wider. A settlement this operator receives nothing from is a foreign
// transaction the wallet has no record of, so the fallback cannot stand in for
// the index.
func observeGamingOperation(ctx context.Context, id string) (gamingfunds.ChainObservation, bool, error) {
	node := hostNode()
	if node == nil {
		return gamingfunds.ChainObservation{}, false, ErrGamingChainUnavailable
	}
	hash, err := chainhash.NewHashFromStr(id)
	if err != nil {
		return gamingfunds.ChainObservation{}, false, err
	}
	facts, err := node.GetRawTransactionVerbose(ctx, hash)
	if err == nil {
		observation := gamingfunds.ChainObservation{Known: true, Confirmations: facts.Confirmations, BlockHash: facts.BlockHash, Height: facts.BlockHeight}
		if facts.Confirmations == 0 {
			observation.Height = 0
			observation.BlockHash = ""
		}
		return observation, true, nil
	}
	var rpcErr *dcrjson.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != dcrjson.ErrRPCNoTxInfo {
		return gamingfunds.ChainObservation{}, false, err
	}
	walletTx, ok, err := hostWallet().Transaction(ctx, *hash)
	if err != nil {
		return gamingfunds.ChainObservation{}, false, err
	}
	if !ok {
		return gamingfunds.ChainObservation{}, false, nil
	}
	known, err := finance.DecodeTransaction(walletTx.Raw)
	if err != nil || known.TxHash() != *hash {
		return gamingfunds.ChainObservation{}, false, fmt.Errorf("wallet returned inconsistent financial transaction")
	}
	confirmations := int64(walletTx.Confirmations)
	if confirmations <= 0 {
		// dcrd did not find it in the mempool. The wallet merely remembering an
		// unmined transaction is not chain presence, so the exact journaled
		// bytes remain eligible for rebroadcast.
		return gamingfunds.ChainObservation{}, false, nil
	}
	if walletTx.BlockHash == nil {
		return gamingfunds.ChainObservation{}, false, fmt.Errorf("confirmed wallet transaction has no block hash")
	}
	blockHash := *walletTx.BlockHash
	header, err := node.GetBlockHeaderVerbose(ctx, &blockHash)
	if err != nil {
		return gamingfunds.ChainObservation{}, false, err
	}
	return gamingfunds.ChainObservation{Known: true, Confirmations: confirmations, BlockHash: blockHash.String(), Height: int64(header.Height)}, true, nil
}

// gamingIndexComplained keeps the reconcile pass from saying the same thing
// every thirty seconds, without it going unsaid.
var gamingIndexComplained atomic.Bool

// noteGamingIndexTrouble says why a reconcile pass could not look a transaction
// up, when the reason is the one it will not recover from.
//
// A lookup can fail for a moment - dcrd restarting, a connection dropped - and
// those pass by themselves, which is why the pass swallows them. A node running
// without its transaction index does not pass: every later pass fails the same
// way, an approved payout every seat signed is never broadcast, and the only
// trace is a spend stuck at publishing.
//
// The node is asked rather than the error read. getinfo answers this exactly,
// and the alternative is matching on an error string dcrd is free to reword.
func noteGamingIndexTrouble(ctx context.Context, cause error) {
	has, err := DcrdHasTxIndex(ctx)
	if err != nil || has {
		return
	}
	if gamingIndexComplained.CompareAndSwap(false, true) {
		gameLog.Errorf("dcrd is running without its transaction index, so approved payouts cannot be broadcast. Set txindex=1 and restart dcrd. Last lookup failed with: %v", cause)
	}
}

// noteGamingLookupWorks retracts that complaint, once, when dcrd answers again.
func noteGamingLookupWorks() {
	if gamingIndexComplained.CompareAndSwap(true, false) {
		gameLog.Infof("dcrd is answering gaming transaction lookups again; approved payouts will be broadcast")
	}
}

type observedGamingSpend struct {
	txid          string
	confirmations int64
	conflict      bool
}

func noteGamingSpend(found map[string]observedGamingSpend, outpoint, txid string, confirmations int64) {
	old, ok := found[outpoint]
	if ok && old.txid != txid {
		old.conflict = true
		found[outpoint] = old
		return
	}
	found[outpoint] = observedGamingSpend{txid: txid, confirmations: confirmations}
}

func scanGamingMempoolSpends(ctx context.Context, targets map[string]bool) (map[string]observedGamingSpend, error) {
	node := hostNode()
	if node == nil {
		return nil, ErrGamingChainUnavailable
	}
	found := map[string]observedGamingSpend{}
	hashes, err := node.GetRawMempool(ctx, chainjson.GRMRegular)
	if err != nil {
		return nil, err
	}
	for _, hash := range hashes {
		tx, err := node.GetRawTransaction(ctx, hash)
		if err != nil {
			return nil, err
		}
		for _, in := range tx.MsgTx().TxIn {
			outpoint := in.PreviousOutPoint.String()
			if targets[outpoint] {
				noteGamingSpend(found, outpoint, hash.String(), 0)
			}
		}
	}
	return found, nil
}

// scanGamingWalletSpends finds confirmed spends through dcrwallet's own block
// history. This works without dcrd's optional transaction index and includes a
// cooperative payout published by another participant when the watched escrow
// script is imported.
func scanGamingWalletSpends(ctx context.Context, deposits []gamingfunds.Deposit, targets map[string]bool) (map[string]observedGamingSpend, error) {
	start := int64(-1)
	for _, dep := range deposits {
		if !targets[dep.Outpoint] || dep.FundingHeight <= 0 {
			continue
		}
		h := dep.FundingHeight - 1
		if start < 0 || h < start {
			start = h
		}
	}
	if start < 0 {
		return map[string]observedGamingSpend{}, nil
	}
	return scanGamingWalletSpendsFrom(ctx, start, targets)
}

// scanGamingWalletSpendsFrom finds the wallet's mined spends of targets from
// block start on.
func scanGamingWalletSpendsFrom(ctx context.Context, start int64, targets map[string]bool) (map[string]observedGamingSpend, error) {
	if start > int64(^uint32(0)>>1) {
		return nil, fmt.Errorf("gaming scan height is out of range")
	}
	node := hostNode()
	if node == nil {
		return nil, ErrGamingChainUnavailable
	}
	tip, err := node.GetBlockCount(ctx)
	if err != nil {
		return nil, err
	}
	found := map[string]observedGamingSpend{}
	err = hostWallet().MinedTransactions(ctx, int32(start), func(height int32, txs [][]byte) error {
		if int64(height) > tip {
			return nil
		}
		confirmations := tip - int64(height) + 1
		for _, raw := range txs {
			tx, err := finance.DecodeTransaction(raw)
			if err != nil {
				return err
			}
			for _, in := range tx.TxIn {
				outpoint := in.PreviousOutPoint.String()
				if targets[outpoint] {
					noteGamingSpend(found, outpoint, tx.TxHash().String(), confirmations)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// gamingAbandonDepth is how deep another spend of a funding's input must be
// before the funding counts as dead.
const gamingAbandonDepth = 2

// transientBroadcastError reports a broadcast that got no answer, as opposed
// to one the node rejected.
func transientBroadcastError(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// fundingSpender names the other transaction that spent one of a funding's
// inputs deep enough to make it dead, or "" when nothing proves that.
func fundingSpender(opID string, inputs []string, spends map[string]observedGamingSpend) string {
	for _, in := range inputs {
		sp := spends[in]
		if sp.conflict || sp.txid == "" || sp.txid == opID || sp.confirmations < gamingAbandonDepth {
			continue
		}
		return sp.txid
	}
	return ""
}

// fundingSpentElsewhere asks the chain whether a rejected funding's inputs are
// gone, and to whom. Its inputs are this wallet's own coins, so the wallet has
// every spend of them.
func fundingSpentElsewhere(ctx context.Context, op gamingfunds.Operation) string {
	missing := map[string]bool{}
	start := int64(-1)
	for _, in := range op.Inputs {
		txid, indexText, ok := strings.Cut(in, ":")
		index, err := strconv.ParseUint(indexText, 10, 32)
		if !ok || err != nil {
			return ""
		}
		out, err := GamingChainOutpoint(ctx, txid, uint32(index), true)
		if err != nil {
			return ""
		}
		if out.Found {
			continue
		}
		hash, err := chainhash.NewHashFromStr(txid)
		if err != nil {
			return ""
		}
		node := hostNode()
		if node == nil {
			return ""
		}
		parent, err := node.GetRawTransactionVerbose(ctx, hash)
		if err != nil || parent.Confirmations <= 0 {
			return ""
		}
		if h := parent.BlockHeight - 1; start < 0 || h < start {
			start = h
		}
		missing[in] = true
	}
	if len(missing) == 0 {
		return ""
	}
	spends, err := scanGamingWalletSpendsFrom(ctx, start, missing)
	if err != nil {
		return ""
	}
	return fundingSpender(op.ID, op.Inputs, spends)
}

// abandonDeadFunding ends a funding the chain shows can never confirm and
// frees its deposit, so the game can ask for it again.
func abandonDeadFunding(ctx context.Context, store *gamingfunds.Store, op gamingfunds.Operation) {
	spender := fundingSpentElsewhere(ctx, op)
	if spender == "" {
		return
	}
	if err := store.AbandonFunding(op.ID, spender); err != nil {
		gameLog.Errorf("abandon dead funding %s: %v", op.ID, err)
		return
	}
	gameLog.Infof("funding %s can never confirm: its coins were spent by %s; the deposit can be requested again", op.ID, spender)
	failAbandonedFundingSpend(op, spender)
}

// failAbandonedFundingSpend closes the request a dead funding answered.
func failAbandonedFundingSpend(op gamingfunds.Operation, spender string) {
	spendMu.Lock()
	defer spendMu.Unlock()
	log, err := readSpendLog()
	if err != nil {
		return
	}
	changed := false
	for i := range log.Spends {
		sp := &log.Spends[i]
		if !slices.Contains(op.DepositIDs, sp.DepositID) {
			continue
		}
		if sp.State == GamingSpendPublishing || sp.State == GamingSpendPending || (sp.State == GamingSpendApproved && sp.TxID == op.ID) {
			sp.State = GamingSpendFailed
			sp.Error = "its coins were spent by " + spender + "; the game can request this deposit again"
			sp.DecidedAt = time.Now().Unix()
			changed = true
		}
	}
	if changed {
		if err = writeSpendLog(log, time.Now().Unix()); err != nil {
			gameLog.Errorf("record dead funding: %v", err)
		}
	}
}

func reconcileGamingDeposits(ctx context.Context, store *gamingfunds.Store) {
	deposits, err := store.AllDeposits()
	if err != nil {
		return
	}
	targets := map[string]bool{}
	for _, dep := range deposits {
		if dep.Outpoint != "" {
			targets[dep.Outpoint] = true
		}
	}
	if len(targets) == 0 || hostNode() == nil {
		return
	}
	mempool, mempoolErr := scanGamingMempoolSpends(ctx, targets)
	confirmed, walletErr := scanGamingWalletSpends(ctx, deposits, targets)
	for _, dep := range deposits {
		if !targets[dep.Outpoint] || recoveryWalletMatches(ctx, dep.Scope) != nil {
			continue
		}
		txid, indexText, ok := strings.Cut(dep.Outpoint, ":")
		index, parseErr := strconv.ParseUint(indexText, 10, 32)
		if !ok || parseErr != nil {
			continue
		}
		out, err := GamingChainOutpoint(ctx, txid, uint32(index), true)
		if err != nil {
			continue
		}
		if out.Found {
			if out.ScriptVersion != 0 || out.ValueAtoms != dep.Terms.Atoms || out.PkScriptHex != dep.PkScript {
				_ = store.ObserveDeposit(dep.Scope, dep.ID, gamingfunds.DepositObservation{})
				continue
			}
			operation, known, err := observeGamingOperation(ctx, dep.FundingTx)
			if err != nil || !known {
				continue
			}
			_ = store.ObserveDeposit(dep.Scope, dep.ID, gamingfunds.DepositObservation{OutputFound: true, Confirmations: operation.Confirmations, FundingBlock: operation.BlockHash, FundingHeight: operation.Height})
			continue
		}
		spend := confirmed[dep.Outpoint]
		if spend.txid == "" && !spend.conflict {
			spend = mempool[dep.Outpoint]
		}
		if spend.conflict || (walletErr != nil && mempoolErr != nil) {
			_ = store.ObserveDeposit(dep.Scope, dep.ID, gamingfunds.DepositObservation{})
			continue
		}
		if spend.txid != "" {
			_ = store.ObserveDeposit(dep.Scope, dep.ID, gamingfunds.DepositObservation{SpendingTx: spend.txid, SpendConfirmations: spend.confirmations})
			continue
		}
		if walletErr == nil && mempoolErr == nil {
			_ = store.ObserveDeposit(dep.Scope, dep.ID, gamingfunds.DepositObservation{})
		}
	}
}

// StartGamingFinancialWorker resumes durable financial work without a running
// game or browser. Call once at dashboard startup and cancel at shutdown.
func StartGamingFinancialWorker(ctx context.Context) {
	gamingFinancialWorker.Lock()
	if gamingFinancialWorker.running {
		gamingFinancialWorker.Unlock()
		return
	}
	gamingFinancialWorker.running = true
	gamingFinancialWorker.Unlock()
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		replay := time.NewTicker(30 * time.Second)
		defer replay.Stop()
		process := func(event GamingFrameEvent) { processFinancialFrame(ctx, event) }
		for _, event := range Gaming().financialReplay() {
			process(event)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-gamingFinancialInbox:
				process(event)
			case <-replay.C:
				for _, event := range Gaming().financialReplay() {
					process(event)
				}
			}
		}
	}()
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var pruned time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				work, cancel := context.WithTimeout(ctx, 25*time.Second)
				reconcileGamingFinance(work)
				cancel()
				if time.Since(pruned) >= time.Hour {
					pruned = time.Now()
					pruneSettledGaming(ctx)
				}
			}
		}
	}()
	go func() {
		workers.Wait()
		gamingFinancialWorker.Lock()
		gamingFinancialWorker.running = false
		gamingFinancialWorker.Unlock()
	}()
}

func reconcileGamingFinance(ctx context.Context) {
	store, err := gamingFundsStore()
	if err != nil {
		gameLog.Errorf("financial ledger unavailable: %v", err)
		return
	}
	operations, err := store.Operations()
	if err != nil {
		gameLog.Errorf("financial operations unavailable: %v", err)
		return
	}
	for _, op := range operations {
		if ctx.Err() != nil {
			return
		}
		if !op.Approved || op.State == "awaiting_signatures" || op.State == gamingfunds.OperationAbandoned {
			continue
		}
		if err = recoveryWalletMatches(ctx, op.Scope); err != nil {
			continue
		}
		if hostNode() == nil {
			continue
		}
		observation, known, err := observeGamingOperation(ctx, op.ID)
		if err != nil {
			noteGamingIndexTrouble(ctx, err)
			continue
		}
		noteGamingLookupWorks()
		if known {
			if err = store.ObserveOperation(op.ID, observation); err != nil {
				gameLog.Errorf("record financial chain observation: %v", err)
			}
			if op.Kind == "funding" {
				reconcileGamingFundingHistory(op.ID)
			}
			continue
		}
		if err = store.ObserveOperation(op.ID, gamingfunds.ChainObservation{}); err != nil {
			continue
		}
		if op.Kind == "settlement" {
			p, e := store.Settlement(op.Scope, op.ID)
			if e != nil {
				continue
			}
			if _, e = store.AuthorizedTable(op.Scope, p.Table); e != nil {
				continue
			}
		}
		if op.Kind == "funding" && !fundingStillWanted(store, op) {
			continue
		}
		raw, err := hex.DecodeString(op.Raw)
		if err != nil {
			continue
		}
		tx, err := finance.DecodeTransaction(raw)
		if err != nil || tx.TxHash().String() != op.ID {
			continue
		}
		// These exact signatures were authorized and persisted before any send.
		if _, err = hostWallet().Broadcast(ctx, raw); err != nil {
			gameLog.Warnf("financial transaction %s remains pending: %v", op.ID, err)
			if op.Kind == "funding" && !transientBroadcastError(err) {
				abandonDeadFunding(ctx, store, op)
			}
		}
	}
	reconcileGamingDeposits(ctx, store)
}

// fundingStillWanted reports whether a funding's deposit and table are still
// open. A closed one is observed but never broadcast again.
func fundingStillWanted(store *gamingfunds.Store, op gamingfunds.Operation) bool {
	deposits, err := store.AllDeposits()
	if err != nil {
		return false
	}
	for _, dep := range deposits {
		if dep.FundingTx != op.ID {
			continue
		}
		if dep.Closed {
			return false
		}
		if dep.Terms.Table == "" {
			continue
		}
		if _, err = store.AuthorizedTable(dep.Scope, dep.Terms.Table); err != nil {
			return false
		}
	}
	return true
}

// receiveFinancial applies one financial frame. Settable for tests.
var receiveFinancial = receiveFinancialFrame

// processFinancialFrame applies one stored financial frame and keeps it out of
// later replays once applied.
func processFinancialFrame(ctx context.Context, event GamingFrameEvent) {
	work, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := receiveFinancial(work, event)
	cancel()
	if err != nil {
		// Invalid records stay harmless; dependency failures are retried
		// from the durable local inbox without peer traffic.
		gameLog.Debugf("financial message rejected: %v", err)
		return
	}
	Gaming().markFinancialApplied(event)
}

// gamingPruneDepth is how deep every spend of a group's deposits must be
// before its protocol history goes.
const gamingPruneDepth = 144

// pruneSettledGaming drops the protocol history of paid-out tables. A spend
// dcrd cannot find counts as not deep, so nothing goes on doubt.
func pruneSettledGaming(ctx context.Context) {
	store, err := gamingFundsStore()
	if err != nil {
		return
	}
	work, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	groups, err := store.PrunableGroups(time.Now().Unix(), gamingPruneDepth, func(txid string) (int64, error) {
		observation, known, err := observeGamingOperation(work, txid)
		if err != nil || !known {
			return 0, err
		}
		return observation.Confirmations, nil
	})
	if err != nil {
		gameLog.Debugf("gaming history prune skipped: %v", err)
		return
	}
	Gaming().pruneSettledGamingHistory(work, groups)
}

func reconcileGamingFundingHistory(txid string) {
	spendMu.Lock()
	defer spendMu.Unlock()
	log, err := readSpendLog()
	if err != nil {
		return
	}
	changed := false
	store, err := gamingFundsStore()
	if err != nil {
		return
	}
	deposits, err := store.AllDeposits()
	if err != nil {
		return
	}
	for _, dep := range deposits {
		if dep.FundingTx != txid {
			continue
		}
		for i := range log.Spends {
			sp := &log.Spends[i]
			if sp.DepositID == dep.ID && (sp.State == GamingSpendPublishing || sp.State == GamingSpendPending) {
				sp.State = GamingSpendApproved
				sp.TxID = txid
				sp.Error = ""
				sp.DecidedAt = time.Now().Unix()
				changed = true
			}
		}
	}
	if changed {
		if err = writeSpendLog(log, time.Now().Unix()); err != nil {
			gameLog.Errorf("reconcile funding history: %v", err)
		}
	}
}
