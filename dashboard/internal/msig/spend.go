// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"decred.org/dcrwallet/v5/wallet/txrules"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"

	"dcrpulse/internal/services"
)

// Spend seams, mirroring the handshake ones so the relay is testable
// without a wallet.
var (
	signSeam = func(ctx context.Context, rawTxHex string, prevInputs []services.MsigPrevInput, account uint32, passphrase []byte) (string, error) {
		return services.SignMsigTransaction(ctx, rawTxHex, prevInputs, account, passphrase)
	}
	broadcastSeam = func(ctx context.Context, signedTx []byte) (string, error) {
		return services.BroadcastSignedTransaction(ctx, signedTx)
	}
	txLookupSeam = func(ctx context.Context, txid string) (int64, bool, error) {
		return services.MsigTxConfirmations(ctx, txid)
	}
)

const (
	// DefaultHopTTL bounds how long one cosigner may sit on a request
	// before the relay moves to the next.
	DefaultHopTTL = 24 * time.Hour
	minHopTTL     = time.Hour
	maxHopTTL     = 7 * 24 * time.Hour

	// maxFeeRateMultiple caps how far above the relay floor a proposal's
	// fee may sit before a cosigner refuses to sign it.
	maxFeeRateMultiple = 10
)

// prevInputsFor builds the explicit prevout list handed to
// signrawtransaction, one pkScript per input from its own ladder
// address. The redeem script is deliberately absent: dcrwallet ignores
// it unless raw private keys are passed and always resolves scripts from
// its own store, which is why every ladder script must be imported.
func prevInputsFor(rec *WalletRecord, inputs []ProposalInput) ([]services.MsigPrevInput, error) {
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return nil, err
	}
	out := make([]services.MsigPrevInput, 0, len(inputs))
	for _, in := range inputs {
		addr, err := stdaddr.DecodeAddress(in.Address, params)
		if err != nil {
			return nil, fmt.Errorf("input %s:%d address: %v", in.TxID, in.Vout, err)
		}
		_, pkScript := addr.PaymentScript()
		out = append(out, services.MsigPrevInput{
			TxID: in.TxID, Vout: in.Vout, Tree: in.Tree, ScriptPubKey: hex.EncodeToString(pkScript),
		})
	}
	return out, nil
}

// lockedInputs returns the outpoints claimed by live proposals, so two
// local proposals can never fight over the same UTXO.
func lockedInputs(rec *WalletRecord, exceptTxID string) map[string]bool {
	locked := make(map[string]bool)
	for txid, p := range rec.Proposals {
		if txid == exceptTxID || !p.Live() {
			continue
		}
		for _, in := range p.Inputs {
			locked[fmt.Sprintf("%s:%d", in.TxID, in.Vout)] = true
		}
	}
	return locked
}

func requireActive(store *Store, rec *WalletRecord) error {
	if rec.Status != StatusActive {
		return fmt.Errorf("shared wallet %q is not active yet", rec.Label)
	}
	if store.WalletName() != activeWalletSeam() {
		return fmt.Errorf("this shared wallet belongs to wallet %q; switch to it first", store.WalletName())
	}
	return nil
}

// ProposeSpend builds, self-signs and dispatches a spend from a shared
// wallet. The queue lists the cosigners asked to sign, in order; it must
// hold at least m-1 entries. With sendAll the whole spendable balance
// sweeps to a single recipient: the amount is computed here as the input
// sum minus the fee and any amount in the request is ignored. Change
// pays a fresh internal ladder index; the dedicated account signs.
func ProposeSpend(ctx context.Context, walletID string, recipients []Recipient, sendAll bool, queueUIDs []string, note string, hopTTL time.Duration, passphrase []byte) (*Proposal, error) {
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()
	network, err := networkSeam(ctx)
	if err != nil {
		return nil, err
	}
	store, rec := manager(network).Route(walletID, nil)
	if rec == nil {
		return nil, fmt.Errorf("unknown shared wallet %s", walletID)
	}
	if err := requireActive(store, rec); err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("no recipients")
	}
	if sendAll && len(recipients) != 1 {
		return nil, fmt.Errorf("send all pays a single recipient")
	}
	if len(note) > MaxNoteLen {
		return nil, fmt.Errorf("note exceeds %d characters", MaxNoteLen)
	}
	if rec.ManualTransport() {
		// The human ferries the request; deadlines would only re-route
		// behind their back.
		hopTTL = 0
	} else {
		if hopTTL == 0 {
			hopTTL = DefaultHopTTL
		}
		if hopTTL < minHopTTL || hopTTL > maxHopTTL {
			return nil, fmt.Errorf("signing window must be between 1 hour and 7 days")
		}
	}
	if len(queueUIDs) < rec.M-1 {
		return nil, fmt.Errorf("pick at least %d cosigner(s) to reach %d signatures", rec.M-1, rec.M)
	}
	queue := make([]*QueueHop, 0, len(queueUIDs))
	seen := make(map[string]bool)
	for _, uid := range queueUIDs {
		p := rec.peerByUID(uid)
		if p == nil {
			return nil, fmt.Errorf("unknown cosigner in the signing queue")
		}
		if seen[uid] {
			return nil, fmt.Errorf("duplicate cosigner in the signing queue")
		}
		seen[uid] = true
		queue = append(queue, &QueueHop{UID: uid, Nick: p.Nick, State: HopPending})
	}

	if !rec.HD || rec.OwnHD == nil {
		return nil, fmt.Errorf("this shared wallet predates the HD ladder; recreate it")
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return nil, err
	}
	all, err := listWindowUTXOs(ctx, store, rec)
	if err != nil {
		return nil, err
	}
	locked := lockedInputs(rec, "")
	avail := make([]UTXO, 0, len(all))
	for _, u := range all {
		if !locked[fmt.Sprintf("%s:%d", u.TxID, u.Vout)] {
			avail = append(avail, u)
		}
	}
	if len(avail) == 0 {
		return nil, fmt.Errorf("no spendable funds: every output is claimed by another proposal")
	}
	var selected []UTXO
	if sendAll {
		if len(avail) > MaxInputs {
			return nil, fmt.Errorf("send all would require more than %d inputs", MaxInputs)
		}
		var sum int64
		for _, u := range avail {
			sum += u.Atoms
		}
		fee := int64(txrules.FeeForSerializeSize(txrules.DefaultRelayFeePerKb,
			EstimateFullSize(len(avail), 1, rec.M, rec.N)))
		if sum <= fee {
			return nil, fmt.Errorf("spendable balance %v does not cover the %v fee",
				dcrutil.Amount(sum), dcrutil.Amount(fee))
		}
		recipients[0].Atoms = sum - fee
		selected = avail
	} else {
		var target int64
		for _, r := range recipients {
			target += r.Atoms
		}
		selected, err = SelectUTXOs(avail, target, len(recipients), rec.M, rec.N, 0)
		if err != nil {
			return nil, err
		}
	}
	// Change pays a fresh internal index, allocated (and its window
	// imported) before the transaction exists. A sendAll sweep produces
	// no change, so it skips the allocation instead of burning an index.
	changeAddr := rec.Address
	if !sendAll {
		if _, changeAddr, err = allocateChangeIndex(ctx, store, rec); err != nil {
			return nil, err
		}
	}
	sizeScript, _, err := ScriptAt(rec.M, rec.Xpubs, BranchExternal, 0, params)
	if err != nil {
		return nil, err
	}
	tx, fee, change, err := BuildSpend(BuildSpendParams{
		UTXOs: selected, Recipients: recipients, ChangeAddress: changeAddr,
		RedeemScript: sizeScript, ChainParams: params,
	})
	if err != nil {
		return nil, err
	}
	txid := tx.TxHash().String()
	rawBytes, err := tx.Bytes()
	if err != nil {
		return nil, err
	}

	inputs := make([]ProposalInput, 0, len(selected))
	for _, u := range selected {
		inputs = append(inputs, ProposalInput{TxID: u.TxID, Vout: u.Vout, Tree: u.Tree, Atoms: u.Atoms, Address: u.Address})
	}
	outputs := make([]ProposalOutput, 0, len(recipients)+1)
	for _, r := range recipients {
		outputs = append(outputs, ProposalOutput{Address: r.Address, Atoms: r.Atoms})
	}
	if change > 0 {
		outputs = append(outputs, ProposalOutput{Address: changeAddr, Atoms: change, IsChange: true})
	}

	prevs, err := prevInputsFor(rec, inputs)
	if err != nil {
		return nil, err
	}
	signedHex, err := signSeam(ctx, hex.EncodeToString(rawBytes), prevs, rec.OwnHD.Account, passphrase)
	if err != nil {
		return nil, err
	}
	signedTx, err := DecodeTxHex(signedHex)
	if err != nil {
		return nil, err
	}
	resolve, err := resolverForInputs(rec, inputs)
	if err != nil {
		return nil, err
	}
	signers, err := VerifyProposalUpdateHD(signedTx, txid, resolve)
	if err != nil {
		return nil, fmt.Errorf("local signing produced an unusable transaction: %v", err)
	}
	if len(signers) == 0 {
		return nil, fmt.Errorf("the wallet added no signature; is this account's key part of the roster?")
	}

	err = store.UpdateProposal(rec.TempID, txid, true, func(_ *WalletRecord, p *Proposal) error {
		p.Role = RoleInitiator
		p.Status = ProposalCollecting
		p.RawTx = signedHex
		p.SigCount = len(signers)
		p.SignedBy = signers
		p.Inputs = inputs
		p.Outputs = outputs
		p.FeeAtoms = fee
		p.Note = note
		p.Queue = queue
		p.HopTTLSecs = int64(hopTTL / time.Second)
		return nil
	})
	if err != nil {
		return nil, err
	}
	advanceProposal(store, rec.TempID, txid)
	_, prop, _ := store.Proposal(rec.TempID, txid)
	return prop, nil
}

// advanceProposal sends the request to the next pending cosigner, or
// broadcasts once the threshold is met. Called after every signature.
func advanceProposal(store *Store, walletID, txid string) {
	rec, prop, ok := store.Proposal(walletID, txid)
	if !ok || prop.Role != RoleInitiator || prop.Terminal() {
		return
	}
	if prop.SigCount >= rec.M {
		broadcastProposal(store, walletID, txid)
		return
	}
	var next *QueueHop
	for _, h := range prop.Queue {
		if h.State == HopPending {
			next = h
			break
		}
	}
	if next == nil {
		if err := store.UpdateProposal(walletID, txid, false, func(_ *WalletRecord, p *Proposal) error {
			p.Status = ProposalFailed
			p.Reason = "no cosigner left to ask"
			return nil
		}); err != nil {
			msigLog.Error(err)
		}
		msigLog.Warnf("proposal %s ran out of cosigners", txid[:12])
		return
	}
	mid, err := NewID()
	if err != nil {
		msigLog.Error(err)
		return
	}
	// A manual request carries the long hand-carried lifetime and no hop
	// deadline: the human is the relay, nothing re-routes behind them.
	manual := rec.ManualTransport()
	expiry := time.Now().Add(ManualTTL)
	var hopDeadline int64
	if !manual {
		ttl := time.Duration(prop.HopTTLSecs) * time.Second
		if ttl == 0 {
			ttl = DefaultHopTTL
		}
		expiry = time.Now().Add(ttl)
		hopDeadline = expiry.Unix()
	}
	msg := &Message{
		Type: TypeSignReq, WalletID: rec.Address, TxID: txid,
		RawTx: prop.RawTx, Note: prop.Note, SigsHave: prop.SigCount,
	}
	payload, err := EncodeMessage(msg)
	if err != nil {
		msigLog.Error(err)
		return
	}
	body, err := Encode(payload, mid, expiry)
	if err != nil {
		msigLog.Error(err)
		return
	}
	err = store.UpdateProposal(walletID, txid, false, func(_ *WalletRecord, p *Proposal) error {
		for _, h := range p.Queue {
			if h.UID == next.UID && h.State == HopPending {
				h.State = HopSent
				h.SentMID = mid
				h.SentAt = time.Now().Unix()
				h.Deadline = hopDeadline
				return nil
			}
		}
		return fmt.Errorf("hop already advanced")
	})
	if err != nil {
		return
	}
	if err := store.AppendOutbox(&OutboxItem{
		MID: mid, ToUID: next.UID, Body: body, State: OutboxSending, Ts: time.Now().Unix(),
		Manual: manual, Type: TypeSignReq, RecID: rec.TempID, TxID: txid,
	}); err != nil {
		msigLog.Error(err)
		return
	}
	if manual {
		if fresh, ok := store.Wallet(rec.TempID); ok {
			notifyRecordChanged(fresh)
		}
		msigLog.Infof("proposal %s waiting for hand-over to %s (%d/%d signatures)", txid[:12], next.Nick, prop.SigCount, rec.M)
		return
	}
	deliverOutbox(store, mid, next.UID, body)
	msigLog.Infof("proposal %s sent to %s (%d/%d signatures)", txid[:12], next.Nick, prop.SigCount, rec.M)
}

func broadcastProposal(store *Store, walletID, txid string) {
	rec, prop, ok := store.Proposal(walletID, txid)
	if !ok || prop.Terminal() {
		return
	}
	if err := store.UpdateProposal(walletID, txid, false, func(_ *WalletRecord, p *Proposal) error {
		p.Status = ProposalReady
		return nil
	}); err != nil {
		msigLog.Error(err)
		return
	}
	if store.WalletName() != activeWalletSeam() {
		msigLog.Infof("proposal %s ready; switch to wallet %q to broadcast", txid[:12], store.WalletName())
		return
	}
	raw, err := hex.DecodeString(prop.RawTx)
	if err != nil {
		msigLog.Error(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), walletCallTimeout)
	defer cancel()
	sent, err := broadcastSeam(ctx, raw)
	if err != nil {
		msigLog.Warnf("broadcast of %s deferred: %v", txid[:12], err)
		return
	}
	if err := store.UpdateProposal(walletID, txid, false, func(_ *WalletRecord, p *Proposal) error {
		p.Status = ProposalBroadcast
		return nil
	}); err != nil {
		msigLog.Error(err)
	}
	notice := &Message{Type: TypeBroadcast, WalletID: rec.Address, TxID: txid}
	for _, p := range rec.Peers {
		if err := sendFrame(store, p.UID, notice, ""); err != nil {
			msigLog.Warnf("broadcast notice: %v", err)
		}
	}
	msigLog.Infof("proposal %s broadcast as %s", txid[:12], sent)
}

// RebroadcastProposal retries a fully signed proposal whose broadcast
// failed or whose wallet was inactive at the time.
func RebroadcastProposal(ctx context.Context, walletID, txid string) error {
	network, err := networkSeam(ctx)
	if err != nil {
		return err
	}
	store, rec := manager(network).Route(walletID, nil)
	if rec == nil {
		return fmt.Errorf("unknown shared wallet %s", walletID)
	}
	_, prop, ok := store.Proposal(rec.TempID, txid)
	if !ok {
		return fmt.Errorf("unknown proposal")
	}
	// A click can race the automatic broadcast; converging on an already
	// sent payment is success, not an error.
	if prop.Status == ProposalBroadcast || prop.Status == ProposalConfirmed {
		return nil
	}
	if prop.Status != ProposalReady {
		return fmt.Errorf("this payment is not waiting to be broadcast")
	}
	broadcastProposal(store, rec.TempID, txid)
	return nil
}

// AbortProposal cancels a proposal this wallet is collecting signatures
// for, releasing its inputs. Cosigners that already hold the request see
// it expire on its own deadline.
func AbortProposal(ctx context.Context, walletID, txid string) error {
	network, err := networkSeam(ctx)
	if err != nil {
		return err
	}
	store, rec := manager(network).Route(walletID, nil)
	if rec == nil {
		return fmt.Errorf("unknown shared wallet %s", walletID)
	}
	return store.UpdateProposal(rec.TempID, txid, false, func(_ *WalletRecord, p *Proposal) error {
		if p.Status != ProposalCollecting && p.Status != ProposalReady {
			return fmt.Errorf("this payment can no longer be cancelled")
		}
		p.Status = ProposalAborted
		p.Reason = "cancelled locally"
		return nil
	})
}

// SignIncomingProposal verifies an incoming request against local state,
// adds this wallet's signature and returns it to the proposer. The
// dedicated account signs after its branch indices are synced through
// the imported window.
func SignIncomingProposal(ctx context.Context, walletID, txid string, passphrase []byte) error {
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()
	network, err := networkSeam(ctx)
	if err != nil {
		return err
	}
	store, rec := manager(network).Route(walletID, nil)
	if rec == nil {
		return fmt.Errorf("unknown shared wallet %s", walletID)
	}
	if err := requireActive(store, rec); err != nil {
		return err
	}
	if !rec.HD || rec.OwnHD == nil {
		return fmt.Errorf("this shared wallet predates the HD ladder; recreate it")
	}
	_, prop, ok := store.Proposal(rec.TempID, txid)
	if !ok {
		return fmt.Errorf("unknown payment request")
	}
	if prop.Status != ProposalIncoming {
		return fmt.Errorf("this payment request is no longer open")
	}
	tx, err := DecodeTxHex(prop.RawTx)
	if err != nil {
		return err
	}
	// Re-verify against live wallet state at signing time, not just at
	// arrival: funds may have moved while the request sat in the inbox.
	if err := verifyAgainstWallet(ctx, store, rec, tx, prop); err != nil {
		return err
	}
	// The wallet only signs for child keys its address manager has seen;
	// make sure the window (and so every input's index) is synced.
	if err := ensureWindowImported(ctx, store, rec.TempID); err != nil {
		return err
	}
	prevs, err := prevInputsFor(rec, prop.Inputs)
	if err != nil {
		return err
	}
	signedHex, err := signSeam(ctx, prop.RawTx, prevs, rec.OwnHD.Account, passphrase)
	if err != nil {
		return err
	}
	signedTx, err := DecodeTxHex(signedHex)
	if err != nil {
		return err
	}
	resolve, err := resolverForInputs(rec, prop.Inputs)
	if err != nil {
		return err
	}
	signers, err := VerifyProposalUpdateHD(signedTx, txid, resolve)
	if err != nil {
		return err
	}
	if len(signers) <= prop.SigCount {
		return fmt.Errorf("the wallet added no signature; is this account's key part of the roster?")
	}
	err = store.UpdateProposal(rec.TempID, txid, false, func(_ *WalletRecord, p *Proposal) error {
		p.RawTx = signedHex
		p.SigCount = len(signers)
		p.SignedBy = signers
		p.Status = ProposalSigned
		return nil
	})
	if err != nil {
		return err
	}
	return sendFrame(store, prop.FromUID, &Message{
		Type: TypeSig, WalletID: rec.Address, TxID: txid, RawTx: signedHex,
	}, "")
}

// RejectIncomingProposal declines a payment request.
func RejectIncomingProposal(ctx context.Context, walletID, txid, reason string) error {
	network, err := networkSeam(ctx)
	if err != nil {
		return err
	}
	store, rec := manager(network).Route(walletID, nil)
	if rec == nil {
		return fmt.Errorf("unknown shared wallet %s", walletID)
	}
	_, prop, ok := store.Proposal(rec.TempID, txid)
	if !ok {
		return fmt.Errorf("unknown payment request")
	}
	if prop.Status != ProposalIncoming {
		return fmt.Errorf("this payment request is no longer open")
	}
	reason = clampReason(reason)
	err = store.UpdateProposal(rec.TempID, txid, false, func(_ *WalletRecord, p *Proposal) error {
		p.Status = ProposalDeclined
		p.Reason = reason
		return nil
	})
	if err != nil {
		return err
	}
	return sendFrame(store, prop.FromUID, &Message{
		Type: TypeSigDecline, WalletID: rec.Address, TxID: txid, Reason: reason,
	}, "")
}

// verifyAgainstWallet checks a proposal against this node's own view of
// the shared wallet: every input must be a current UTXO of the ladder
// window, and the fee must be sane. Outputs are CLASSIFIED, not vetoed:
// an output to a derived internal address is machine-verified change,
// everything else is a recipient the human reviews in the approval UI.
// A cosigner never trusts the proposer's summary.
func verifyAgainstWallet(ctx context.Context, store *Store, rec *WalletRecord, tx *wire.MsgTx, prop *Proposal) error {
	utxos, err := listWindowUTXOs(ctx, store, rec)
	if err != nil {
		return err
	}
	byOutpoint := make(map[string]int64, len(utxos))
	for _, u := range utxos {
		byOutpoint[fmt.Sprintf("%s:%d", u.TxID, u.Vout)] = u.Atoms
	}
	var totalIn int64
	for _, in := range tx.TxIn {
		key := fmt.Sprintf("%s:%d", in.PreviousOutPoint.Hash.String(), in.PreviousOutPoint.Index)
		atoms, ok := byOutpoint[key]
		if !ok {
			return fmt.Errorf("this payment spends funds that are no longer available (%s)", key)
		}
		totalIn += atoms
	}
	var totalOut int64
	for _, out := range tx.TxOut {
		totalOut += out.Value
	}
	fee := totalIn - totalOut
	if fee <= 0 {
		return fmt.Errorf("this payment has no fee and would be rejected by the network")
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return err
	}
	sizeScript, _, err := ScriptAt(rec.M, rec.Xpubs, BranchExternal, 0, params)
	if err != nil {
		return err
	}
	size := EstimateSize(tx, rec.M, len(sizeScript))
	ceiling := int64(txrules.FeeForSerializeSize(maxFeeRateMultiple*txrules.DefaultRelayFeePerKb, size))
	if fee > ceiling {
		return fmt.Errorf("this payment pays an unusually high fee (%v); refusing to sign",
			dcrutil.Amount(fee))
	}
	return nil
}
