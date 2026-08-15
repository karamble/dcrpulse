// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"

	"dcrpulse/internal/gamingbridge"
	"dcrpulse/internal/rpc"
)

// Relaying a transaction a game signed itself.
//
// Everything else a game does with money goes through /spend, where the host
// builds the transaction and the person approves it with a passphrase the
// dashboard never holds. That covers paying money in. It cannot cover taking it
// back out, because the coin sits behind a script only the game holds the key
// for - a fidelity bond, or a stake in escrow behind its timelock. The game
// signs those itself and needs somebody to put the bytes on the network.
//
// So the host relays, and the question is what it should refuse. It cannot
// judge intent: it has no idea what a table is and is not going to be taught,
// because the moment the bridge understands poker it stops being a tunnel. What
// it can do is bound the shape of what passes:
//
//   - every input must spend a P2SH output that the wallet does not own, which
//     is what game money looks like and what wallet money does not, and
//   - every output must pay an address the wallet does own.
//
// The second rule is the one that matters and the one easy to leave out. The
// game is the thing holding the signing key, so validating only inputs leaves
// it free to sign a perfectly well-formed refund that pays itself - with coin
// that came out of the user's wallet in the first place. With both rules the
// relay can only move game money back to the person who funded it, which is
// exactly what a refund and a sweep are and nothing else.
//
// Settlement will need the output rule relaxed, since it pays a winner who may
// be another player. That is not a flaw here: settlement is an n-of-n
// transaction every member signs and checks, so it earns a rule of its own
// rather than inheriting one written for unilateral recovery.

// sigLen is the length of a signature as it appears in a signature script:
// 64 bytes of schnorr over secp256k1, and one byte of sighash type after it.
//
// Sixty-five, not sixty-four, and the difference cost two live tables. A schnorr
// signature is 64 bytes and is never pushed on its own - txscript wants the hash
// type with it, and pkg/escrow appends it (escrow.SigLen is 65 for the same
// reason). Counting 64 here found no signatures in anything, so every co-signed
// spend looked unilateral, so a table could neither pay its winner nor hand a
// bond back to anybody but the person running this wallet.
//
// The tests below now measure real escrow bytes rather than bytes built from
// this constant. A fixture derived from the number being tested cannot disagree
// with it, which is why nothing caught this.
const sigLen = 65

// signaturesIn counts how many parties had to agree to a spend.
//
// A pay-to-script-hash signature script ends with the redeem script it
// satisfies, and everything pushed before it is what that script consumes -
// signatures, and whatever selects a branch. So the count is the pushes before
// the last one that are the size of a signature.
//
// Counting signatures rather than recognising an escrow, deliberately. The
// moment this file learns what a poker table looks like the bridge stops being
// a tunnel, and it would have to learn again for the next game.
func signaturesIn(sigScript []byte) int {
	var pushes [][]byte
	tok := txscript.MakeScriptTokenizer(scriptVersion, sigScript)
	for tok.Next() {
		if d := tok.Data(); d != nil {
			pushes = append(pushes, d)
		}
	}
	if tok.Err() != nil || len(pushes) < 2 {
		return 0
	}
	// The last push is the redeem script, not a signature.
	var n int
	for _, p := range pushes[:len(pushes)-1] {
		if len(p) == sigLen {
			n++
		}
	}
	return n
}

// outputsMayPayAnyone reports whether this transaction is allowed to pay
// somebody other than the person running this wallet.
//
// It may when every input needed more than one signature, and not otherwise.
//
// The distinction is the whole of the rule. Coin this box could move on its own
// - a stake past its timelock, a bond past its lock - must come home, because
// the game holds that key and nothing else stands between it and paying itself.
// Coin that took every member of a table to move is different: the thing
// protecting it is the other signers, and they have already signed. Refusing it
// here would protect nobody, because any of those co-signers can relay the same
// transaction from their own machine. It would only decide which of them does.
//
// Every input, not any: a transaction that mixes the two would otherwise carry
// a unilateral spend out of the wallet on the back of a co-signed one.
func outputsMayPayAnyone(tx *wire.MsgTx) bool {
	if len(tx.TxIn) == 0 {
		return false
	}
	for _, in := range tx.TxIn {
		if signaturesIn(in.SignatureScript) < 2 {
			return false
		}
	}
	return true
}

// scriptVersion is the only script version these games use.
const scriptVersion = 0

// maxGamingTxBytes bounds what a game may hand over. A refund or a sweep is one
// input and one output - a few hundred bytes. This is room to be wrong in
// without being room to abuse.
const maxGamingTxBytes = 100 << 10

// GamingPrevout is what the validator needs to know about an input's previous
// output: what kind of script pays it, and to whom.
type GamingPrevout struct {
	Found     bool
	Type      string
	Addresses []string

	// ValueAtoms is what the input is worth. It costs nothing to carry - the
	// lookup already asks for it - and it is what lets the validator check
	// that a transaction moving coin nobody here owns is not quietly moving
	// some of it somewhere else.
	ValueAtoms int64
}

func lookupGamingPrevout(ctx context.Context, op wire.OutPoint) (GamingPrevout, error) {
	if rpc.DcrdClient == nil {
		return GamingPrevout{}, ErrGamingChainUnavailable
	}
	// Mempool included: a refund may well spend a deposit whose funding is
	// still unconfirmed, and refusing on that basis would be refusing on a
	// timing accident rather than on anything about the transaction.
	out, err := rpc.DcrdClient.GetTxOut(ctx, &op.Hash, op.Index, 0, true)
	if err != nil {
		return GamingPrevout{}, fmt.Errorf("read %s:%d: %w", op.Hash, op.Index, err)
	}
	if out == nil {
		return GamingPrevout{}, nil
	}
	value, err := dcrutil.NewAmount(out.Value)
	if err != nil {
		return GamingPrevout{}, fmt.Errorf("read the value of %s:%d: %w", op.Hash, op.Index, err)
	}
	return GamingPrevout{
		Found:      true,
		Type:       out.ScriptPubKey.Type,
		Addresses:  out.ScriptPubKey.Addresses,
		ValueAtoms: int64(value),
	}, nil
}

// walletOwns reports whether any of these addresses is the wallet's own.
func walletOwns(ctx context.Context, addresses []string) bool {
	for _, a := range addresses {
		if strings.TrimSpace(a) == "" {
			continue
		}
		if v, err := ValidateAddress(ctx, a); err == nil && v.IsMine {
			return true
		}
	}
	return false
}

// GamingBroadcast checks a game's signed transaction and relays it.
func GamingBroadcast(ctx context.Context, game, rawTxHex string) (string, error) {
	game = strings.ToLower(strings.TrimSpace(game))
	raw, err := hex.DecodeString(strings.TrimSpace(rawTxHex))
	if err != nil {
		return "", gamingbridge.GameSafe(fmt.Errorf("that is not hex"))
	}
	switch {
	case len(raw) == 0:
		return "", gamingbridge.GameSafe(fmt.Errorf("nothing to broadcast"))
	case len(raw) > maxGamingTxBytes:
		return "", gamingbridge.GameSafe(fmt.Errorf("transaction is %d bytes, and the limit is %d", len(raw), maxGamingTxBytes))
	}

	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return "", gamingbridge.GameSafe(fmt.Errorf("that is not a transaction: %w", err))
	}
	if len(tx.TxIn) == 0 || len(tx.TxOut) == 0 {
		return "", gamingbridge.GameSafe(fmt.Errorf("a transaction needs at least one input and one output"))
	}

	// Inputs: game money only, and never the wallet's own coin.
	var inputAtoms int64
	for i, in := range tx.TxIn {
		facts, err := spendPrevout(ctx, in.PreviousOutPoint)
		if err != nil {
			return "", err
		}
		switch {
		case !facts.Found:
			return "", gamingbridge.GameSafe(fmt.Errorf("input %d spends %s:%d, which holds no coin anyone can see",
				i, in.PreviousOutPoint.Hash, in.PreviousOutPoint.Index))
		case facts.Type != "scripthash":
			return "", gamingbridge.GameSafe(fmt.Errorf("input %d spends a %s output, and a game may only spend script hashes",
				i, facts.Type))
		case walletOwns(ctx, facts.Addresses):
			return "", gamingbridge.GameSafe(fmt.Errorf("input %d spends coin belonging to this wallet", i))
		}
		inputAtoms += facts.ValueAtoms
	}

	// Outputs: back to the person, and nowhere else.
	decoded, err := DecodeRawTransaction(ctx, raw)
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	// Three shapes are allowed out, and no others.
	//
	// Co-signed: every input needed a signature from every seat, so the
	// others already agreed to wherever it goes.
	//
	// Coming home: coin the game could move on its own, going back to this
	// wallet.
	//
	// Passing through: coin that was never this wallet's, staying not this
	// wallet's, with nothing taken on the way. That last one is a real
	// transaction and not a theoretical one - a game answering an accusation
	// spends its own bond straight into a fresh one, alone, and being unable
	// to send it costs the player that bond within minutes.
	coSigned := outputsMayPayAnyone(&tx)
	if !coSigned && passesThrough(ctx, &tx, decoded, inputAtoms) {
		coSigned = true
	}
	if !coSigned {
		for _, out := range decoded.Outputs {
			if !walletOwns(ctx, out.Addresses) {
				where := "an address this wallet does not hold"
				if len(out.Addresses) > 0 {
					where = out.Addresses[0]
				}
				return "", gamingbridge.GameSafe(fmt.Errorf("output %d pays %s; coin this game could move on its own "+
					"may only come back to this wallet", out.Index, where))
			}
		}
	}

	txid, err := BroadcastSignedTransaction(ctx, raw)
	if err != nil {
		return "", gamingbridge.GameSafe(fmt.Errorf("broadcast: %w", err))
	}

	// Said out loud, because this is the one path where coin moves without
	// anybody being asked. It is bounded rather than approved, and a bound
	// nobody can see afterwards is not much of a bound.
	ins := make([]string, 0, len(tx.TxIn))
	for _, in := range tx.TxIn {
		ins = append(ins, fmt.Sprintf("%s:%d", in.PreviousOutPoint.Hash, in.PreviousOutPoint.Index))
	}
	how := "returning coin to this wallet"
	if coSigned {
		how = "co-signed, so its outputs were not checked against this wallet"
	}
	gameLog.Infof("%s broadcast %s, spending %s (%s)", game, txid, strings.Join(ins, " "), how)
	return txid, nil
}

// maxPassThroughFee bounds what a transaction going straight back out may leave
// behind.
//
// Generous next to what any of these actually pay - the game's own fees are a
// fraction of this - and far below anything worth the trouble of stealing. It
// is a ceiling on carelessness, not a fee estimate.
const maxPassThroughFee = 100_000

// passesThrough reports whether this is coin that was never the wallet's,
// staying not the wallet's, with nothing taken on the way.
//
// All three parts are load-bearing. Without the input rule this would let a
// game move the wallet's own money; without the output rule it says nothing at
// all; and without conserving the value it would let a game spend a bond into a
// dust output and hand the rest to a miner, which is theft with extra steps.
//
// The bridge learns nothing about any game's escrow to do this. It is a
// statement about where coin came from and where it is going, which is the only
// kind of statement this can honestly make.
func passesThrough(ctx context.Context, tx *wire.MsgTx, decoded *pb.DecodedTransaction, inputAtoms int64) bool {
	if len(tx.TxIn) == 0 || len(decoded.Outputs) == 0 || inputAtoms <= 0 {
		return false
	}
	var outputAtoms int64
	for _, out := range tx.TxOut {
		outputAtoms += out.Value
	}
	// Every output has to be a script hash the wallet does not hold. A game
	// sending its coin to a plain address it chose is the case this must
	// not admit.
	for _, out := range decoded.Outputs {
		if out.GetScriptClass() != pb.DecodedTransaction_Output_SCRIPT_HASH || walletOwns(ctx, out.Addresses) {
			return false
		}
	}
	fee := inputAtoms - outputAtoms
	return fee >= 0 && fee <= maxPassThroughFee
}
