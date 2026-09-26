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

// GamingPayoutView is a payout proposal as the operator reviews it, with their
// own share picked out.
type GamingPayoutView struct {
	gamingfunds.Settlement
	// Mine is nil when the operator has no stake among the inputs.
	Mine *GamingPayoutShare `json:"mine"`
	// SignaturesSent says whether our signatures reached Bison Relay: "sent",
	// "uncertain" or "unsent"; empty when we have none waiting on peers.
	SignaturesSent string `json:"signaturesSent,omitempty"`
}

// GamingPayoutShare is what the operator put into a payout and gets out of it.
type GamingPayoutShare struct {
	Key          string `json:"key"`
	Address      string `json:"address"`
	StakeAtoms   int64  `json:"stakeAtoms"`
	ReceiveAtoms int64  `json:"receiveAtoms"`
}

// payoutShare finds the operator's input and payment by their table key. A
// payout's owner key is the input's recovery key, and payments name it.
func payoutShare(p gamingfunds.Settlement, key gamingfunds.WalletKey) (GamingPayoutShare, bool) {
	share := GamingPayoutShare{Key: key.Public, Address: key.Address}
	staked := false
	for _, in := range p.Inputs {
		if in.Terms.Recovery == key.Public {
			share.StakeAtoms += in.Terms.Atoms
			staked = true
		}
	}
	for _, pay := range p.Payments {
		if pay.Key == key.Public {
			share.ReceiveAtoms += pay.Atoms
		}
	}
	return share, staked
}

func GamingPayouts(ctx context.Context) ([]GamingPayoutView, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	all, err := store.AllSettlements()
	if err != nil {
		return nil, err
	}
	out := make([]GamingPayoutView, 0, len(all))
	for _, p := range all {
		view := GamingPayoutView{Settlement: p}
		if key, err := store.WalletKey(p.Scope, p.Table); err == nil {
			if share, ok := payoutShare(p, key); ok {
				view.Mine = &share
			}
			view.SignaturesSent = payoutSignaturesSent(store, p, key)
		}
		out = append(out, view)
	}
	return out, nil
}

// payoutSignaturesSent reads, never sends, the state of our signature message
// for a payout still collecting signatures.
func payoutSignaturesSent(store *gamingfunds.Store, p gamingfunds.Settlement, key gamingfunds.WalletKey) string {
	sigs := p.Signatures[key.Public]
	if p.State != "awaiting_signatures" || len(sigs) == 0 {
		return ""
	}
	table, err := store.AuthorizedTable(p.Scope, p.Table)
	if err != nil {
		return ""
	}
	parsed, frame, err := financialFrame(p.Scope.Game, p.Table, financialMessage{Settlement: p.ID, Signatures: sigs})
	if err != nil {
		return ""
	}
	state, err := gamingFrameSendState(p.Scope.Game, table.Group, parsed, frame)
	switch {
	case err != nil:
		return ""
	case state == "sent":
		return "sent"
	case state == "claimed":
		return "uncertain"
	default:
		return "unsent"
	}
}

// SendGamingPayoutSignatures is the operator's one send of signatures that
// Bison Relay never accepted. It signs nothing; the stored signatures go out
// through the same claim, so a message that was sent is never sent again.
func SendGamingPayoutSignatures(ctx context.Context, id string) (*gamingpb.PayoutStatusReply, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	p, err := gamingPayoutByID(store, id)
	if err != nil {
		return nil, err
	}
	key, err := store.WalletKey(p.Scope, p.Table)
	if err != nil {
		return nil, err
	}
	sigs := p.Signatures[key.Public]
	if p.State != "awaiting_signatures" || len(sigs) == 0 {
		return nil, fmt.Errorf("payout has no signatures of ours waiting to be sent")
	}
	table, err := store.AuthorizedTable(p.Scope, p.Table)
	if err != nil {
		return nil, err
	}
	if err = sendFinancialMessage(ctx, p.Scope.Game, table.Group, p.Table, financialMessage{Settlement: id, Signatures: sigs}); err != nil {
		return nil, err
	}
	return payoutStatus(p), nil
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
	// Approval is a state transition. Publish its signatures once; BR group
	// history supplies them to peers that connect later.
	if err = sendFinancialMessage(ctx, p.Scope.Game, table.Group, p.Table, financialMessage{Settlement: id, Signatures: sigs}); err != nil {
		return nil, err
	}
	refreshed, err := store.Settlement(p.Scope, id)
	if err != nil {
		return nil, err
	}
	GamingPresenceChanged(p.Scope.Game)
	return payoutStatus(refreshed), nil
}
