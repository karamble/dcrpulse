package services

import (
	"context"
	"encoding/hex"
	"fmt"

	"dcrpulse/internal/rpc"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
)

const maxGamingTxBytes = 100 << 10

// GamingPrevout is the bridge's independently obtained view of an input.
type GamingPrevout struct {
	PkScript      []byte
	ScriptVersion uint16
	Found         bool
	Type          string
	Addresses     []string
	ValueAtoms    int64
}

func lookupGamingPrevout(ctx context.Context, op wire.OutPoint) (GamingPrevout, error) {
	if rpc.DcrdClient == nil {
		return GamingPrevout{}, ErrGamingChainUnavailable
	}
	out, err := rpc.DcrdClient.GetTxOut(ctx, &op.Hash, op.Index, op.Tree, true)
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
	pk, err := hex.DecodeString(out.ScriptPubKey.Hex)
	if err != nil {
		return GamingPrevout{}, err
	}
	return GamingPrevout{
		PkScript: pk, ScriptVersion: out.ScriptPubKey.Version,
		Found: true, Type: out.ScriptPubKey.Type,
		Addresses: out.ScriptPubKey.Addresses, ValueAtoms: int64(value),
	}, nil
}
