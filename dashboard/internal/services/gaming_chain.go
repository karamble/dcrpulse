// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/rpc"
)

// The sandbox has no route to a chain, and it needs one. Bonds are coin at an
// outpoint, so a player can only be asked to post one if the others can check
// it; an admission window is only agreed if everyone reads the same height; and
// a dealer chosen from a block hash is only unpredictable if nobody supplies
// the hash themselves.
//
// So the host answers, and the split is worth being exact about. Everywhere
// else the bridge is a tunnel and not an inspector: it carries frames whole and
// forms no opinion, because a host that judged a fold would be a party to the
// game nobody agreed to trust. This is not a departure from that. What a game
// gets here are facts about a public ledger, not judgements about a table - the
// host says what the chain contains, and the game decides what it means.
//
// Trusting the host for that is not a concession either. It is the user's own
// dcrd, on their own machine. The parties a player must not have to trust are
// the other players, and none of them are on this side of the tunnel.

// ErrGamingChainUnavailable says the node is not there to ask.
var ErrGamingChainUnavailable = errors.New("chain is not available")

// GamingChainTip is where the chain is now.
type GamingChainTip struct {
	Height int64  `json:"height"`
	Hash   string `json:"hash"`
}

// GamingOutpoint is what the chain says about one transaction output.
//
// Spent and missing are the same answer here, because dcrd reports both by
// having nothing to say about the outpoint - and for the question a game is
// asking, "no coin at that outpoint" is the answer either way.
type GamingOutpoint struct {
	Found         bool   `json:"found"`
	ValueAtoms    int64  `json:"valueAtoms"`
	PkScriptHex   string `json:"pkScriptHex"`
	Confirmations int64  `json:"confirmations"`
	Coinbase      bool   `json:"coinbase"`
}

// GamingChainTipNow reports the best block.
func GamingChainTipNow(ctx context.Context) (GamingChainTip, error) {
	if rpc.DcrdClient == nil {
		return GamingChainTip{}, ErrGamingChainUnavailable
	}
	hash, height, err := rpc.DcrdClient.GetBestBlock(ctx)
	if err != nil {
		return GamingChainTip{}, fmt.Errorf("best block: %w", err)
	}
	return GamingChainTip{Height: height, Hash: hash.String()}, nil
}

// GamingChainOutpoint reports what is at an outpoint, if anything.
//
// It looks up unspent outputs only, which is the whole question: a bond that
// has been spent is not a bond any more, and an escrow whose deposit is gone
// cannot be settled against.
func GamingChainOutpoint(ctx context.Context, txid string, vout uint32) (GamingOutpoint, error) {
	if rpc.DcrdClient == nil {
		return GamingOutpoint{}, ErrGamingChainUnavailable
	}
	hash, err := chainhash.NewHashFromStr(strings.TrimSpace(txid))
	if err != nil {
		return GamingOutpoint{}, fmt.Errorf("txid: %w", err)
	}

	// Mempool is excluded. A game is deciding whether to stake money against
	// somebody else's deposit, and an unconfirmed one can still be replaced.
	out, err := rpc.DcrdClient.GetTxOut(ctx, hash, vout, 0, false)
	if err != nil {
		return GamingOutpoint{}, fmt.Errorf("outpoint: %w", err)
	}
	if out == nil {
		// Nothing there: never existed, already spent, or unconfirmed.
		return GamingOutpoint{}, nil
	}

	// dcrd reports the value as DCR in a float. Multiplying it out by hand
	// would put rounding error into an amount a bond is later judged
	// against, so the conversion is the one dcrutil does.
	amt, err := dcrutil.NewAmount(out.Value)
	if err != nil {
		return GamingOutpoint{}, fmt.Errorf("outpoint value: %w", err)
	}

	return GamingOutpoint{
		Found:         true,
		ValueAtoms:    int64(amt),
		PkScriptHex:   out.ScriptPubKey.Hex,
		Confirmations: out.Confirmations,
		Coinbase:      out.Coinbase,
	}, nil
}
