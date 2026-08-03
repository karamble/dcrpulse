// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

// inboundSpend routes a spend-protocol frame to the shared wallet it
// names. Frames for wallets this node does not hold are dropped without
// journaling so a later restore can still process them.
func inboundSpend(ctx context.Context, m *Manager, msg *Message, frame *Frame, fromUID, fromNick string, now time.Time) {
	store, rec := m.Route(msg.WalletID, func(r *WalletRecord) bool {
		return r.Address == msg.WalletID
	})
	if rec == nil {
		log.Printf("msig: %s frame for unknown shared wallet from %s, left for catch-up", msg.Type, fromNick)
		return
	}
	// Only members speak the spend protocol.
	if rec.peerByUID(fromUID) == nil {
		log.Printf("msig: %s frame from a non-member for %q", msg.Type, rec.Label)
		return
	}
	fresh, err := store.MarkProcessed(frame.MID, now)
	if err != nil {
		log.Printf("msig: journal: %v", err)
		return
	}
	if !fresh {
		return
	}
	switch msg.Type {
	case TypeSignReq:
		inboundSignReq(ctx, store, rec, msg, fromUID, fromNick, now)
	case TypeSig:
		inboundSig(store, rec, msg, fromUID)
	case TypeSigDecline:
		inboundSigDecline(store, rec, msg, fromUID, fromNick)
	case TypeBroadcast:
		inboundBroadcastNotice(ctx, store, rec, msg, fromUID, fromNick)
	}
}

// inboundSignReq records an incoming payment request after verifying it
// end to end against local state. Anything that fails verification is
// auto-declined with the reason, so the proposer's relay advances at once
// instead of waiting out the deadline.
func inboundSignReq(ctx context.Context, store *Store, rec *WalletRecord, msg *Message, fromUID, fromNick string, now time.Time) {
	autoDecline := func(reason string) {
		log.Printf("msig: declining payment request %s for %q: %s", msg.TxID[:12], rec.Label, reason)
		if err := sendFrame(store, fromUID, &Message{
			Type: TypeSigDecline, WalletID: rec.Address, TxID: msg.TxID, Reason: reason,
		}, ""); err != nil {
			log.Printf("msig: %v", err)
		}
	}
	if rec.Status != StatusActive {
		autoDecline("the shared wallet is not active on this node")
		return
	}
	tx, err := DecodeTxHex(msg.RawTx)
	if err != nil {
		autoDecline("the transaction could not be decoded")
		return
	}
	if tx.TxHash().String() != msg.TxID {
		autoDecline("the transaction does not match its stated id")
		return
	}
	if _, prop, ok := store.Proposal(rec.TempID, msg.TxID); ok && prop.Terminal() {
		return
	}
	// Refuse anything spending outputs a local proposal already claims.
	for _, in := range tx.TxIn {
		key := fmt.Sprintf("%s:%d", in.PreviousOutPoint.Hash.String(), in.PreviousOutPoint.Index)
		if lockedInputs(rec, msg.TxID)[key] {
			autoDecline("those funds are already committed to another payment here")
			return
		}
	}

	inputs, outputs, fee, err := describeTx(ctx, store, rec, tx)
	if err != nil {
		// The request may spend indices this node has not imported or
		// scanned yet (a peer ran ahead within the gap window). Top the
		// window up, schedule the bounded rescan and retry once before
		// burning the hop with a decline.
		if ierr := ensureWindowImported(ctx, store, rec.TempID); ierr == nil {
			from := rec.CreatedHeight - 1
			if from < 0 {
				from = 0
			}
			rescanSeam(from)
			if fresh, ok := store.Wallet(rec.TempID); ok {
				rec = fresh
			}
			inputs, outputs, fee, err = describeTx(ctx, store, rec, tx)
		}
	}
	if err != nil {
		autoDecline(err.Error())
		return
	}
	resolve, err := resolverForInputs(rec, inputs)
	if err != nil {
		autoDecline("this payment spends addresses outside the wallet's ladder")
		return
	}
	signers, err := VerifyProposalUpdateHD(tx, msg.TxID, resolve)
	if err != nil {
		autoDecline("the transaction carries signatures this wallet cannot verify")
		return
	}
	prop := &Proposal{
		TxID: msg.TxID, Role: RoleCosigner, Status: ProposalIncoming,
		RawTx: msg.RawTx, SigCount: len(signers), SignedBy: signers,
		Inputs: inputs, Outputs: outputs, FeeAtoms: fee,
		Note: msg.Note, FromUID: fromUID, FromNick: fromNick,
	}
	if err := verifyAgainstWallet(ctx, store, rec, tx, prop); err != nil {
		autoDecline(err.Error())
		return
	}
	err = store.UpdateProposal(rec.TempID, msg.TxID, true, func(_ *WalletRecord, p *Proposal) error {
		if p.Terminal() {
			return fmt.Errorf("already resolved")
		}
		*p = *prop
		p.CreatedAt = now.Unix()
		return nil
	})
	if err != nil {
		return
	}
	log.Printf("msig: payment request %s received for %q from %s", msg.TxID[:12], rec.Label, fromNick)
}

// describeTx renders a transaction into the reviewable shape, resolving
// input values from this node's own UTXO view across the ladder window.
// An output is labeled change only when it pays a derived internal-branch
// address this node can verify; the proposer's word is never taken.
func describeTx(ctx context.Context, store *Store, rec *WalletRecord, tx *wire.MsgTx) ([]ProposalInput, []ProposalOutput, int64, error) {
	utxos, err := listWindowUTXOs(ctx, store, rec)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("the wallet could not list the shared funds")
	}
	byOutpoint := make(map[string]UTXO, len(utxos))
	for _, u := range utxos {
		byOutpoint[fmt.Sprintf("%s:%d", u.TxID, u.Vout)] = u
	}
	var totalIn int64
	inputs := make([]ProposalInput, 0, len(tx.TxIn))
	for _, in := range tx.TxIn {
		key := fmt.Sprintf("%s:%d", in.PreviousOutPoint.Hash.String(), in.PreviousOutPoint.Index)
		u, ok := byOutpoint[key]
		if !ok {
			return nil, nil, 0, fmt.Errorf("this payment spends funds that are no longer available")
		}
		inputs = append(inputs, ProposalInput{TxID: u.TxID, Vout: u.Vout, Tree: u.Tree, Atoms: u.Atoms, Address: u.Address})
		totalIn += u.Atoms
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return nil, nil, 0, err
	}
	view, err := ladderFor(rec)
	if err != nil {
		return nil, nil, 0, err
	}
	var totalOut int64
	outputs := make([]ProposalOutput, 0, len(tx.TxOut))
	for _, out := range tx.TxOut {
		totalOut += out.Value
		entry := ProposalOutput{Atoms: out.Value}
		_, addrs := stdscript.ExtractAddrs(out.Version, out.PkScript, params)
		if len(addrs) > 0 {
			entry.Address = addrs[0].String()
			if e, ok := view.byAddr[entry.Address]; ok && e.Branch == BranchInternal {
				entry.IsChange = true
			}
		}
		outputs = append(outputs, entry)
	}
	return inputs, outputs, totalIn - totalOut, nil
}

func inboundSig(store *Store, rec *WalletRecord, msg *Message, fromUID string) {
	_, prop, ok := store.Proposal(rec.TempID, msg.TxID)
	if !ok || prop.Role != RoleInitiator || prop.Terminal() {
		return
	}
	tx, err := DecodeTxHex(msg.RawTx)
	if err != nil {
		log.Printf("msig: signature frame for %s could not be decoded", msg.TxID[:12])
		return
	}
	resolve, err := resolverForInputs(rec, prop.Inputs)
	if err != nil {
		log.Printf("msig: %v", err)
		return
	}
	signers, err := VerifyProposalUpdateHD(tx, msg.TxID, resolve)
	if err != nil {
		log.Printf("msig: rejecting returned transaction for %s: %v", msg.TxID[:12], err)
		return
	}
	if len(signers) <= prop.SigCount {
		log.Printf("msig: returned transaction for %s adds no signature", msg.TxID[:12])
		return
	}
	err = store.UpdateProposal(rec.TempID, msg.TxID, false, func(_ *WalletRecord, p *Proposal) error {
		if p.Terminal() || len(signers) <= p.SigCount {
			return fmt.Errorf("stale signature")
		}
		p.RawTx = msg.RawTx
		p.SigCount = len(signers)
		p.SignedBy = signers
		for _, h := range p.Queue {
			if h.UID == fromUID && h.State == HopSent {
				h.State = HopSigned
			}
		}
		return nil
	})
	if err != nil {
		return
	}
	advanceProposal(store, rec.TempID, msg.TxID)
}

func inboundSigDecline(store *Store, rec *WalletRecord, msg *Message, fromUID, fromNick string) {
	_, prop, ok := store.Proposal(rec.TempID, msg.TxID)
	if !ok || prop.Role != RoleInitiator || prop.Terminal() {
		return
	}
	err := store.UpdateProposal(rec.TempID, msg.TxID, false, func(_ *WalletRecord, p *Proposal) error {
		for _, h := range p.Queue {
			if h.UID == fromUID && h.State == HopSent {
				h.State = HopDeclined
				h.Reason = msg.Reason
			}
		}
		return nil
	})
	if err != nil {
		return
	}
	log.Printf("msig: %s declined payment %s", fromNick, msg.TxID[:12])
	advanceProposal(store, rec.TempID, msg.TxID)
}

// inboundBroadcastNotice records that a payment went out. Members who
// were never asked to sign (the threshold was met without them) learn of
// the spend only here, so an unknown txid creates an informational entry
// carrying no transaction of its own. The notice itself is not trusted:
// the record only turns broadcast once this wallet has seen the
// transaction; until then the report is kept with its provenance and the
// sweep's own lookup converges the state.
func inboundBroadcastNotice(ctx context.Context, store *Store, rec *WalletRecord, msg *Message, fromUID, fromNick string) {
	_, existing, known := store.Proposal(rec.TempID, msg.TxID)
	if known && existing.Status == ProposalConfirmed {
		return
	}
	var seen bool
	if _, s, err := txLookupSeam(ctx, msg.TxID); err == nil {
		seen = s
	}
	err := store.UpdateProposal(rec.TempID, msg.TxID, true, func(_ *WalletRecord, p *Proposal) error {
		if p.Status == ProposalConfirmed {
			return fmt.Errorf("already confirmed")
		}
		if p.Role == "" {
			p.Role = RoleCosigner
			p.FromUID = fromUID
			p.FromNick = fromNick
		}
		if seen {
			p.Status = ProposalBroadcast
			p.Reason = ""
		} else {
			if p.Status == "" {
				p.Status = ProposalBroadcast
			}
			p.Reason = fmt.Sprintf("%s reported this payment broadcast; not yet seen by this wallet", fromNick)
		}
		return nil
	})
	if err != nil {
		return
	}
	log.Printf("msig: %s reported payment %s broadcast (seen locally: %t)", fromNick, msg.TxID[:12], seen)
}

// sweepProposals expires hops whose deadline passed and retires
// proposals whose inputs were spent elsewhere. Runs for every store on
// the periodic sweep; chain checks need the active wallet.
func sweepProposals(ctx context.Context, store *Store) {
	now := time.Now().Unix()
	for _, rec := range store.Wallets() {
		for txid, prop := range rec.Proposals {
			if prop.Role == RoleInitiator && prop.Status == ProposalCollecting {
				timedOut := false
				if err := store.UpdateProposal(rec.TempID, txid, false, func(_ *WalletRecord, p *Proposal) error {
					for _, h := range p.Queue {
						if h.State == HopSent && h.Deadline > 0 && h.Deadline < now {
							h.State = HopTimeout
							timedOut = true
						}
					}
					return nil
				}); err != nil {
					continue
				}
				if timedOut {
					log.Printf("msig: payment %s timed out at a cosigner; moving on", txid[:12])
					advanceProposal(store, rec.TempID, txid)
				}
			}
			if prop.Status == ProposalReady && store.WalletName() == activeWalletSeam() {
				broadcastProposal(store, rec.TempID, txid)
			}
		}
		if store.WalletName() == activeWalletSeam() && rec.Status == StatusActive && rec.HD {
			utxos, err := listWindowUTXOs(ctx, store, rec)
			if err != nil {
				continue
			}
			// Chain observation drives the cursors: usage a peer caused
			// advances the windows and, when it outran this node's
			// imports, schedules the bounded rescan.
			observeUsage(ctx, store, rec, utxos)
			reconcileSpent(ctx, store, rec, utxos)
		}
	}
}

// reconcileSpent marks proposals whose inputs have left the UTXO set:
// the wallet's own broadcast becomes confirmed, anything else superseded.
func reconcileSpent(ctx context.Context, store *Store, rec *WalletRecord, utxos []UTXO) {
	var live bool
	for _, p := range rec.Proposals {
		if p.Live() {
			live = true
		}
	}
	if !live {
		return
	}
	unspent := make(map[string]bool, len(utxos))
	for _, u := range utxos {
		unspent[fmt.Sprintf("%s:%d", u.TxID, u.Vout)] = true
	}
	setStatus := func(txid, status, reason string) {
		if err := store.UpdateProposal(rec.TempID, txid, false, func(_ *WalletRecord, p *Proposal) error {
			p.Status = status
			p.Reason = reason
			return nil
		}); err != nil {
			return
		}
		log.Printf("msig: payment %s is now %s", txid[:12], status)
	}
	for txid, prop := range rec.Proposals {
		if !prop.Live() {
			continue
		}
		// The wallet's own view of the proposal's transaction decides the
		// outcome. This also settles records that carry no inputs of
		// their own (broadcast notices for payments this member was never
		// asked to sign) and cosigner records whose notice never arrived.
		conf, seen, lookErr := txLookupSeam(ctx, txid)
		if lookErr == nil && seen && conf > 0 {
			setStatus(txid, ProposalConfirmed, "")
			continue
		}
		gone := false
		for _, in := range prop.Inputs {
			if !unspent[fmt.Sprintf("%s:%d", in.TxID, in.Vout)] {
				gone = true
			}
		}
		if !gone || lookErr != nil {
			// Nothing spent yet, or the spender cannot be attributed
			// right now; the next sweep retries.
			continue
		}
		if seen {
			// The proposal's own transaction spent the inputs and is
			// still unmined. Seeing it locally also retires any
			// "reported but not yet seen" caveat a broadcast notice
			// left behind.
			if prop.Status != ProposalBroadcast || prop.Reason != "" {
				setStatus(txid, ProposalBroadcast, "")
			}
			continue
		}
		setStatus(txid, ProposalSuperseded, "another payment spent these funds first")
	}
}
