// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package oggopus

import (
	"bytes"
	"errors"
	"math/rand"
)

const (
	// SampleRate and FrameSamples mirror bruig's recorder: 48 kHz audio in
	// 20 ms Opus frames, so every packet carries 960 PCM samples.
	SampleRate   = 48000
	FrameSamples = 960

	// MaxPacketBytes is the largest Opus packet a 20 ms frame can produce.
	MaxPacketBytes = 1275
)

var errNoPackets = errors.New("no packets to encode")

// OpusFile wraps the given 20 ms Opus packets in an Ogg container the way
// bruig's NoteRecorder.OpusFile does, one page per packet.
func OpusFile(packets [][]byte) ([]byte, error) {
	return opusFileWithSerial(packets, rand.Uint32())
}

func opusFileWithSerial(packets [][]byte, serial uint32) ([]byte, error) {
	if len(packets) == 0 {
		return nil, errNoPackets
	}
	buf := bytes.NewBuffer(nil)
	w, err := newOpusWriterSerial(buf, serial)
	if err != nil {
		return nil, err
	}
	for i, p := range packets {
		if err := w.WritePacket(p, FrameSamples, i == len(packets)-1); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
