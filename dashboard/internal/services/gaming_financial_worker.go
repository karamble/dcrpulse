package services

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/rpc"
	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
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
	if rpc.DcrdClient == nil {
		return gamingfunds.ChainObservation{}, false, ErrGamingChainUnavailable
	}
	hash, err := chainhash.NewHashFromStr(id)
	if err != nil {
		return gamingfunds.ChainObservation{}, false, err
	}
	facts, err := rpc.DcrdClient.GetRawTransactionVerbose(ctx, hash)
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
	if rpc.WalletGrpcClient == nil {
		return gamingfunds.ChainObservation{}, false, nil
	}
	walletTx, err := rpc.WalletGrpcClient.GetTransaction(ctx, &pb.GetTransactionRequest{TransactionHash: hash[:]})
	if status.Code(err) == codes.NotFound {
		return gamingfunds.ChainObservation{}, false, nil
	}
	if err != nil {
		return gamingfunds.ChainObservation{}, false, err
	}
	if walletTx.GetTransaction() == nil {
		return gamingfunds.ChainObservation{}, false, nil
	}
	known, err := finance.DecodeTransaction(walletTx.GetTransaction().GetTransaction())
	if err != nil || known.TxHash() != *hash {
		return gamingfunds.ChainObservation{}, false, fmt.Errorf("wallet returned inconsistent financial transaction")
	}
	confirmations := int64(walletTx.GetConfirmations())
	if confirmations <= 0 {
		// dcrd did not find it in the mempool. The wallet merely remembering an
		// unmined transaction is not chain presence, so the exact journaled
		// bytes remain eligible for rebroadcast.
		return gamingfunds.ChainObservation{}, false, nil
	}
	blockBytes := walletTx.GetBlockHash()
	if len(blockBytes) != chainhash.HashSize {
		return gamingfunds.ChainObservation{}, false, fmt.Errorf("confirmed wallet transaction has no block hash")
	}
	var blockHash chainhash.Hash
	copy(blockHash[:], blockBytes)
	header, err := rpc.DcrdClient.GetBlockHeaderVerbose(ctx, &blockHash)
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
	found := map[string]observedGamingSpend{}
	hashes, err := rpc.DcrdClient.GetRawMempool(ctx, chainjson.GRMRegular)
	if err != nil {
		return nil, err
	}
	for _, hash := range hashes {
		tx, err := rpc.DcrdClient.GetRawTransaction(ctx, hash)
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
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet unavailable")
	}
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
	if start > int64(^uint32(0)>>1) {
		return nil, fmt.Errorf("gaming scan height is out of range")
	}
	tip, err := rpc.DcrdClient.GetBlockCount(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := rpc.WalletGrpcClient.GetTransactions(ctx, &pb.GetTransactionsRequest{StartingBlockHeight: int32(start), EndingBlockHeight: -1})
	if err != nil {
		return nil, err
	}
	found := map[string]observedGamingSpend{}
	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		block := response.GetMinedTransactions()
		if block == nil || int64(block.GetHeight()) > tip {
			continue
		}
		confirmations := tip - int64(block.GetHeight()) + 1
		for _, details := range block.GetTransactions() {
			tx, err := finance.DecodeTransaction(details.GetTransaction())
			if err != nil {
				return nil, err
			}
			for _, in := range tx.TxIn {
				outpoint := in.PreviousOutPoint.String()
				if targets[outpoint] {
					noteGamingSpend(found, outpoint, tx.TxHash().String(), confirmations)
				}
			}
		}
	}
	return found, nil
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
	if len(targets) == 0 || rpc.DcrdClient == nil {
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
		process := func(event GamingFrameEvent) {
			work, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := receiveFinancialFrame(work, event)
			cancel()
			if err != nil {
				// Invalid records stay harmless; dependency failures are retried
				// from the durable local inbox without peer traffic.
				gameLog.Debugf("financial message rejected: %v", err)
			}
		}
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
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				work, cancel := context.WithTimeout(ctx, 25*time.Second)
				reconcileGamingFinance(work)
				cancel()
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
		if !op.Approved || op.State == "awaiting_signatures" {
			continue
		}
		if err = recoveryWalletMatches(ctx, op.Scope); err != nil {
			continue
		}
		if rpc.DcrdClient == nil {
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
		raw, err := hex.DecodeString(op.Raw)
		if err != nil {
			continue
		}
		tx, err := finance.DecodeTransaction(raw)
		if err != nil || tx.TxHash().String() != op.ID {
			continue
		}
		// These exact signatures were authorized and persisted before any send.
		if _, err = BroadcastSignedTransaction(ctx, raw); err != nil {
			gameLog.Debugf("financial transaction %s remains pending: %v", op.ID, err)
		}
	}
	reconcileGamingDeposits(ctx, store)
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
