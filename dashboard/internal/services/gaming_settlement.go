package services

import (
	"context"
	"encoding/hex"
	"fmt"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/gamingpb"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
)

func validateGamingPayoutInputs(ctx context.Context, inputs []finance.Input) error {
	params, err := chainParams(ctx)
	if err != nil {
		return err
	}
	for _, input := range inputs {
		facts, err := GamingChainOutpoint(ctx, input.Outpoint.Hash.String(), input.Outpoint.Index, true)
		if err != nil {
			return err
		}
		_, pk, err := input.Terms.Output(params)
		if err != nil {
			return err
		}
		if !facts.Found || facts.ScriptVersion != 0 || facts.Coinbase || facts.Confirmations < 1 || facts.ValueAtoms != input.Terms.Atoms || facts.PkScriptHex != hex.EncodeToString(pk) {
			return fmt.Errorf("payout input is not a confirmed matching deposit")
		}
	}
	return nil
}
func payoutStatus(p gamingfunds.Settlement) *gamingpb.PayoutStatusReply {
	txid := ""
	if p.State == "publishing" || p.State == "confirmed" {
		txid = p.ID
	}
	return &gamingpb.PayoutStatusReply{Id: p.ID, Sid: p.Table, State: p.State, Txid: txid, Signatures: uint32(len(p.Signatures)), Required: uint32(len(p.Destinations))}
}
func ProposeGamingPayout(ctx context.Context, game string, req *gamingpb.ProposePayoutRequest) (*gamingpb.PayoutStatusReply, error) {
	if req == nil || len(req.Inputs) < 2 || len(req.Inputs) > finance.MaxMembers || len(req.Payments) == 0 || len(req.Payments) > finance.MaxMembers {
		return nil, fmt.Errorf("invalid payout proposal shape")
	}
	for _, input := range req.Inputs {
		if input == nil {
			return nil, fmt.Errorf("missing payout input")
		}
	}
	for _, payment := range req.Payments {
		if payment == nil {
			return nil, fmt.Errorf("missing payout payment")
		}
	}
	scope, err := gamingFinancialScope(ctx, game)
	if err != nil {
		return nil, err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	table, err := store.AuthorizedTable(scope, req.GetSid())
	if err != nil {
		return nil, err
	}
	peers, err := store.Participants(scope, table.Table)
	if err != nil {
		return nil, err
	}
	if len(req.Inputs) != int(table.Seats) || len(req.Payments) > int(table.Seats) {
		return nil, fmt.Errorf("invalid payout size")
	}
	members := make([]string, 0, len(peers))
	for _, peer := range peers {
		members = append(members, peer.Key)
	}
	proposal := finance.Payout{Table: table.Table}
	for _, input := range req.Inputs {
		hash, err := chainhash.NewHashFromStr(input.Txid)
		if err != nil {
			return nil, err
		}
		terms := finance.Terms{Version: finance.Version, Game: game, Network: scope.Network, Account: scope.Account, Table: table.Table, Kind: "stake", Atoms: table.StakeAtoms, LockBlocks: table.CSVBlocks, Identity: input.IdentityKey, Recovery: input.OwnerKey, Members: members}
		proposal.Inputs = append(proposal.Inputs, finance.Input{Terms: terms, Outpoint: wire.OutPoint{Hash: *hash, Index: input.Vout, Tree: wire.TxTreeRegular}})
	}
	for _, pay := range req.Payments {
		proposal.Payments = append(proposal.Payments, finance.Payment{Key: pay.OwnerKey, Atoms: pay.AmountAtoms})
	}
	if err = validateGamingPayoutInputs(ctx, proposal.Inputs); err != nil {
		return nil, err
	}
	params, err := chainParams(ctx)
	if err != nil {
		return nil, err
	}
	p, err := store.ProposeSettlement(scope, proposal, params)
	if err != nil {
		return nil, err
	}
	GamingPresenceChanged(game)
	return payoutStatus(p), nil
}
func GamingPayoutStatus(ctx context.Context, game, id string) (*gamingpb.PayoutStatusReply, error) {
	scope, err := gamingFinancialScope(ctx, game)
	if err != nil {
		return nil, err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	p, err := store.Settlement(scope, id)
	if err != nil {
		return nil, err
	}
	return payoutStatus(p), nil
}
func GamingPayouts(ctx context.Context) ([]gamingfunds.Settlement, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	return store.AllSettlements()
}

func gamingPayoutByID(store *gamingfunds.Store, id string) (gamingfunds.Settlement, error) {
	all, err := store.AllSettlements()
	if err != nil {
		return gamingfunds.Settlement{}, err
	}
	for _, p := range all {
		if p.ID == id {
			return p, nil
		}
	}
	return gamingfunds.Settlement{}, fmt.Errorf("unknown payout")
}

// RejectGamingPayout is exclusively a dashboard action. It releases no key or
// signature and makes an identical game retry observe the operator's refusal.
func RejectGamingPayout(ctx context.Context, id string) (*gamingpb.PayoutStatusReply, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	p, err := gamingPayoutByID(store, id)
	if err != nil {
		return nil, err
	}
	rejected, err := store.RejectSettlement(p.Scope, id)
	if err != nil {
		return nil, err
	}
	GamingPresenceChanged(p.Scope.Game)
	return payoutStatus(rejected), nil
}

// ApproveGamingPayout is exclusively a dashboard action. The game receives
// status only; the bridge exchanges signatures on its own reserved BR channel.
func ApproveGamingPayout(ctx context.Context, id string, passphrase []byte) (*gamingpb.PayoutStatusReply, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	p, err := gamingPayoutByID(store, id)
	if err != nil {
		return nil, err
	}
	if err = recoveryWalletMatches(ctx, p.Scope); err != nil {
		return nil, err
	}
	if err = validateGamingPayoutInputs(ctx, p.Inputs); err != nil {
		return nil, err
	}
	var sigs [][]byte
	_, err = withGamingWalletSigner(ctx, p.Scope, passphrase, func(sign gamingfunds.WalletSigner) ([]byte, error) {
		var err error
		sigs, err = store.ApproveSettlement(p.Scope, id, sign)
		return nil, err
	})
	if err != nil {
		return nil, err
	}
	uid, err := localGamingUID(ctx)
	if err != nil {
		return nil, err
	}
	params, err := chainParams(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = store.AddSettlementSignatures(p.Scope, id, uid, sigs, params); err != nil {
		return nil, err
	}
	table, err := store.AuthorizedTable(p.Scope, p.Table)
	if err != nil {
		return nil, err
	}
	// A send failure retains approval and signatures for durable retry.
	if err = sendFinancialMessage(ctx, p.Scope.Game, table.Group, p.Table, financialMessage{Settlement: id, Signatures: sigs, Want: true}); err != nil {
		return nil, err
	}
	refreshed, err := store.Settlement(p.Scope, id)
	if err != nil {
		return nil, err
	}
	GamingPresenceChanged(p.Scope.Game)
	return payoutStatus(refreshed), nil
}
