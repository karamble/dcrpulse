package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/rpc"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/wire"
)

type GamingRecoveryView struct {
	ID              string `json:"id"`
	Game            string `json:"game"`
	Table           string `json:"table"`
	Kind            string `json:"kind"`
	Atoms           int64  `json:"atoms"`
	Outpoint        string `json:"outpoint"`
	LockBlocks      uint32 `json:"lockBlocks"`
	Confirmations   int64  `json:"confirmations"`
	RemainingBlocks int64  `json:"remainingBlocks"`
	State           string `json:"state"`
	Reason          string `json:"reason,omitempty"`
	CanRecover      bool   `json:"canRecover"`
	Closed          bool   `json:"closed"`
}

func recoveryWalletMatches(ctx context.Context, scope gamingfunds.Scope) error {
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return err
	}
	if network != scope.Network {
		return fmt.Errorf("connect the original network to recover this deposit")
	}
	xpub, err := GetAccountExtendedPubKey(ctx, scope.Account)
	if err != nil {
		return err
	}
	h := sha256.Sum256([]byte(xpub))
	if xpub == "" || hex.EncodeToString(h[:]) != scope.Wallet {
		return fmt.Errorf("connect the original wallet to recover this deposit")
	}
	return nil
}
func recoveryDeposit(id string) (gamingfunds.Deposit, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return gamingfunds.Deposit{}, err
	}
	rows, err := store.AllDeposits()
	if err != nil {
		return gamingfunds.Deposit{}, err
	}
	for _, dep := range rows {
		if dep.ID == id {
			return dep, nil
		}
	}
	return gamingfunds.Deposit{}, fmt.Errorf("unknown deposit")
}

// recoveryMempoolInputs maps each outpoint a mempool transaction spends to that
// transaction's id.
func recoveryMempoolInputs(ctx context.Context) (map[string]string, error) {
	if rpc.DcrdClient == nil {
		return nil, ErrGamingChainUnavailable
	}
	hashes, err := rpc.DcrdClient.GetRawMempool(ctx, chainjson.GRMRegular)
	if err != nil {
		return nil, err
	}
	inputs := map[string]string{}
	for _, hash := range hashes {
		tx, err := rpc.DcrdClient.GetRawTransaction(ctx, hash)
		if err != nil {
			return nil, err
		}
		for _, in := range tx.MsgTx().TxIn {
			inputs[in.PreviousOutPoint.String()] = hash.String()
		}
	}
	return inputs, nil
}

// recoveryRefundTx is the id of the refund journaled for a deposit, if any.
func recoveryRefundTx(depositID string) string {
	store, err := gamingFundsStore()
	if err != nil {
		return ""
	}
	ops, err := store.Operations()
	if err != nil {
		return ""
	}
	for _, op := range ops {
		if op.Kind != "recovery" {
			continue
		}
		for _, id := range op.DepositIDs {
			if id == depositID {
				return op.ID
			}
		}
	}
	return ""
}

// spendReason describes a recorded spend of a deposit: the journaled refund
// or another transaction, confirmed or still waiting for a block.
func spendReason(spender, refund string, confirmed bool) string {
	switch {
	case confirmed && spender == refund:
		return "Refunded to your wallet in " + spender
	case confirmed:
		return "Spent by transaction " + spender
	case spender == refund:
		return "Refund " + spender + " is in the mempool, waiting for a block"
	default:
		return "Transaction " + spender + " spends this output and is waiting for a block"
	}
}

// pendingReason describes an output a mempool transaction spends, or that a
// journaled refund reserves before it is seen in the mempool.
func pendingReason(spender, refund string) string {
	switch {
	case spender != "" && spender == refund:
		return "Refund " + spender + " is in the mempool, waiting for a block"
	case spender != "":
		return "Another transaction " + spender + " spends this output"
	case refund != "":
		return "Refund " + refund + " is signed; broadcasting, retried automatically"
	default:
		return "A transaction already reserves this output"
	}
}

func recoveryView(ctx context.Context, dep gamingfunds.Deposit, spends map[string]string, poolErr error) GamingRecoveryView {
	v := GamingRecoveryView{ID: dep.ID, Game: dep.Scope.Game, Table: dep.Terms.Table, Kind: dep.Terms.Kind, Atoms: dep.Terms.Atoms, Outpoint: dep.Outpoint, LockBlocks: dep.Terms.LockBlocks, Closed: dep.Closed, State: "needs_attention"}
	if err := recoveryWalletMatches(ctx, dep.Scope); err != nil {
		v.Reason = err.Error()
		return v
	}
	if dep.SpendingTx != "" {
		v.State = "recovery_pending"
		if dep.State == "spent" {
			v.State = "spent"
		}
		v.Reason = spendReason(dep.SpendingTx, recoveryRefundTx(dep.ID), dep.State == "spent")
		return v
	}
	if dep.Outpoint == "" {
		v.State = "awaiting_payment"
		v.Reason = "No funded output has been recorded"
		return v
	}
	tx, index, ok := strings.Cut(dep.Outpoint, ":")
	n, err := strconv.ParseUint(index, 10, 32)
	if !ok || err != nil {
		v.Reason = "Invalid recorded output"
		return v
	}
	out, err := GamingChainOutpoint(ctx, tx, uint32(n), true)
	if err != nil {
		v.Reason = err.Error()
		return v
	}
	if !out.Found {
		v.Reason = "Output unavailable: checking for a confirmed spend or chain reorganization"
		return v
	}
	if out.ScriptVersion != 0 || out.ValueAtoms != dep.Terms.Atoms || out.PkScriptHex != dep.PkScript {
		v.Reason = "Chain output does not match the registered deposit"
		return v
	}
	if poolErr != nil {
		v.Reason = "Cannot verify pending spends"
		return v
	}
	if spender := spends[dep.Outpoint]; spender != "" || dep.State == "recovery_pending" {
		v.State = "recovery_pending"
		v.Reason = pendingReason(spender, recoveryRefundTx(dep.ID))
		return v
	}
	v.Confirmations = out.Confirmations
	v.RemainingBlocks = max(int64(dep.Terms.LockBlocks)-out.Confirmations, 0)
	if v.RemainingBlocks > 0 {
		v.State = "locked"
		return v
	}
	if !dep.Closed {
		v.State = "close_table"
		v.Reason = "Close the table locally before recovering funds"
		return v
	}
	store, err := gamingFundsStore()
	if err != nil {
		v.Reason = err.Error()
		return v
	}
	params, err := chainParams(ctx)
	if err != nil {
		v.Reason = err.Error()
		return v
	}
	if err = store.VerifyRecovery(dep.Scope, dep.ID, params); err != nil {
		v.Reason = err.Error()
		return v
	}
	v.State = "recoverable"
	v.CanRecover = true
	return v
}
func GamingRecoveryList(ctx context.Context) ([]GamingRecoveryView, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	deps, err := store.AllDeposits()
	if err != nil {
		return nil, err
	}
	spends, poolErr := recoveryMempoolInputs(ctx)
	out := make([]GamingRecoveryView, 0, len(deps))
	for _, dep := range deps {
		out = append(out, recoveryView(ctx, dep, spends, poolErr))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func CloseGamingRecoveryTable(ctx context.Context, id string) error {
	dep, err := recoveryDeposit(id)
	if err != nil {
		return err
	}
	if err = recoveryWalletMatches(ctx, dep.Scope); err != nil {
		return err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return err
	}
	return store.CloseTable(dep.Scope, dep.Terms.Table)
}
func QuoteGamingRecovery(ctx context.Context, id string) (gamingfunds.RecoveryQuote, error) {
	var zero gamingfunds.RecoveryQuote
	dep, err := recoveryDeposit(id)
	if err != nil {
		return zero, err
	}
	spends, poolErr := recoveryMempoolInputs(ctx)
	view := recoveryView(ctx, dep, spends, poolErr)
	if !view.CanRecover {
		return zero, fmt.Errorf("%s: %s", view.State, view.Reason)
	}
	dest, err := GetNextAddress(ctx, dep.Scope.Account)
	if err != nil {
		return zero, err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return zero, err
	}
	params, err := chainParams(ctx)
	if err != nil {
		return zero, err
	}
	return store.QuoteRecovery(dep.Scope, id, dest, params)
}
func ConfirmGamingRecovery(ctx context.Context, id, quote string, passphrase []byte) (string, error) {
	dep, err := recoveryDeposit(id)
	if err != nil {
		return "", err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return "", err
	}
	q, quoted, err := store.RecoveryQuote(dep.Scope, quote)
	if err != nil {
		return "", err
	}
	if quoted.ID != dep.ID {
		return "", fmt.Errorf("quote belongs to a different deposit")
	}
	if err = recoveryWalletMatches(ctx, dep.Scope); err != nil {
		return "", err
	}
	owner, err := ValidateAddress(ctx, q.Destination)
	if err != nil {
		return "", err
	}
	if !owner.IsValid || !owner.IsMine || owner.AccountNumber != dep.Scope.Account {
		return "", fmt.Errorf("recovery destination is not in the original account")
	}
	if q.TxID == "" {
		spends, poolErr := recoveryMempoolInputs(ctx)
		view := recoveryView(ctx, dep, spends, poolErr)
		if !view.CanRecover {
			return "", fmt.Errorf("%s: %s", view.State, view.Reason)
		}
	}
	params, err := chainParams(ctx)
	if err != nil {
		return "", err
	}
	raw, err := withGamingWalletSigner(ctx, dep.Scope, passphrase, func(sign gamingfunds.WalletSigner) ([]byte, error) {
		return store.ApproveRecovery(dep.Scope, quote, params, sign)
	})
	if err != nil {
		return "", err
	}
	// Journaled bytes survive both failure and lost responses. Never rebuild an
	// approved refund at a different destination or with a different fee.
	txid, err := BroadcastSignedTransaction(ctx, raw)
	if err != nil {
		var tx wire.MsgTx
		_ = tx.FromBytes(raw)
		return tx.TxHash().String(), fmt.Errorf("recovery is journaled; broadcast outcome needs reconciliation: %w", err)
	}
	GamingPresenceChanged(dep.Scope.Game)
	return txid, nil
}

// GamingLedgerBackup returns the whole financial ledger as a self-checking
// backup: open and settled deposits, their terms and scripts, and every
// funding, payout and refund transaction. It holds no private keys.
func GamingLedgerBackup() ([]byte, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	return store.ExportBackup()
}
