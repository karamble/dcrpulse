// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"github.com/decred/dcrd/wire"
)

// TxCountsFit walks a fully serialized transaction's prefix with wire's own
// readers before wire.MsgTx allocates for its input and output counts: a count
// the bytes cannot back ends at EOF after a few reads instead of an allocation
// for millions of elements. The layout mirrors wire's readTxInPrefix and
// readTxOut; a full transaction's witness count must match its inputs.
func TxCountsFit(raw []byte) error {
	r := bytes.NewReader(raw)
	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return err
	}
	if wire.TxSerializeType(version>>16) != wire.TxSerializeFull {
		return errors.New("not a fully serialized transaction")
	}
	inputs, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return err
	}
	var op wire.OutPoint
	var sequence uint32
	for i := uint64(0); i < inputs; i++ {
		if err := wire.ReadOutPoint(r, 0, uint16(version), &op); err != nil {
			return err
		}
		if err := binary.Read(r, binary.LittleEndian, &sequence); err != nil {
			return err
		}
	}
	outputs, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return err
	}
	var value int64
	var scriptVersion uint16
	for i := uint64(0); i < outputs; i++ {
		if err := binary.Read(r, binary.LittleEndian, &value); err != nil {
			return err
		}
		if err := binary.Read(r, binary.LittleEndian, &scriptVersion); err != nil {
			return err
		}
		n, err := wire.ReadVarInt(r, 0)
		if err != nil {
			return err
		}
		if n > uint64(r.Len()) {
			return io.ErrUnexpectedEOF
		}
		if _, err := r.Seek(int64(n), io.SeekCurrent); err != nil {
			return err
		}
	}
	return nil
}
