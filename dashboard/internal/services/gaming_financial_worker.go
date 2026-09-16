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

// Keep healing an incomplete async authority roster through the table's stake
// funding window. After this, an incomplete table is stale recovery state and
// must not keep writing BR chat messages forever.
const gamingFinancialRosterGraceBlocks int64 = 16

var gamingFinancialRosterRetries struct {
	sync.Mutex
	last map[string]int64
}

func financialRosterRetryKey(scope gamingfunds.Scope, table string) string {
	return scope.Game + "\x00" + scope.Network + "\x00" + scope.Wallet + "\x00" + strconv.FormatUint(uint64(scope.Account), 10) + "\x00" + table
}

// claimFinancialRosterRetry limits durable BR healing traffic to one request
// per chain height. A failed send is released so the next worker pass can try
// again; successful unchanged announcements wait for the next block.
func claimFinancialRosterRetry(scope gamingfunds.Scope, table string, height int64) bool {
	key := financialRosterRetryKey(scope, table)
	gamingFinancialRosterRetries.Lock()
	defer gamingFinancialRosterRetries.Unlock()
	if gamingFinancialRosterRetries.last == nil {
		gamingFinancialRosterRetries.last = map[string]int64{}
	}
	if last, ok := gamingFinancialRosterRetries.last[key]; ok && last == height {
		return false
	}
	gamingFinancialRosterRetries.last[key] = height
	return true
}

func releaseFinancialRosterRetry(scope gamingfunds.Scope, table string, height int64) {
	key := financialRosterRetryKey(scope, table)
	gamingFinancialRosterRetries.Lock()
	defer gamingFinancialRosterRetries.Unlock()
	if gamingFinancialRosterRetries.last[key] == height {
		delete(gamingFinancialRosterRetries.last, key)
	}
}

// observeGamingOperation first asks dcrd, which covers the mempool and nodes
// with transaction indexing. Confirmed wallet transactions then fall back to
// dcrwallet plus a block-header lookup, so reconciliation does not require
// dcrd's optional transaction index.
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
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-gamingFinancialInbox:
				work, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := receiveFinancialFrame(work, event)
				cancel()
				if err != nil {
					gameLog.Warnf("financial message rejected: %v", err)
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
			continue
		}
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
	// Durable public announcements and released signatures are retransmitted.
	tables, err := store.Tables()
	if err != nil {
		return
	}
	tip := int64(-1)
	if rpc.DcrdClient != nil {
		if height, tipErr := rpc.DcrdClient.GetBlockCount(ctx); tipErr == nil {
			tip = height
		}
	}
	for _, table := range tables {
		if ctx.Err() != nil {
			return
		}
		if table.Closed {
			continue
		}
		ready, readyErr := store.RosterReady(table.Scope, table.Table)
		if readyErr == nil && ready {
			continue
		}
		if tip >= 0 && tip > int64(table.Until)+gamingFinancialRosterGraceBlocks {
			continue
		}
		retryHeight := tip
		if retryHeight < 0 {
			// No node means no trustworthy table height. Keep async healing
			// bounded to one attempt per target block interval instead.
			retryHeight = time.Now().Unix() / int64((5 * time.Minute).Seconds())
		}
		if !claimFinancialRosterRetry(table.Scope, table.Table, retryHeight) {
			continue
		}
		if _, err = store.WalletKey(table.Scope, table.Table); err != nil {
			releaseFinancialRosterRetry(table.Scope, table.Table, retryHeight)
			continue
		}
		if err = recoveryWalletMatches(ctx, table.Scope); err != nil {
			releaseFinancialRosterRetry(table.Scope, table.Table, retryHeight)
			continue
		}
		if err = announceGamingAuthority(ctx, table.Scope, table.Table, true); err != nil {
			releaseFinancialRosterRetry(table.Scope, table.Table, retryHeight)
			gameLog.Debugf("financial roster announcement pending: %v", err)
		}
	}
	payouts, err := store.AllSettlements()
	if err != nil {
		return
	}
	for _, p := range payouts {
		if ctx.Err() != nil {
			return
		}
		if p.State != "awaiting_signatures" && p.State != "publishing" {
			continue
		}
		table, err := store.AuthorizedTable(p.Scope, p.Table)
		if err != nil {
			continue
		}
		key, err := store.WalletKey(p.Scope, p.Table)
		if err != nil {
			continue
		}
		if sigs := p.Signatures[key.Public]; len(sigs) > 0 {
			if err = sendFinancialMessage(ctx, p.Scope.Game, table.Group, p.Table, financialMessage{Settlement: p.ID, Signatures: sigs}); err != nil {
				gameLog.Debugf("payout signature delivery pending: %v", err)
			}
		}
	}
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
