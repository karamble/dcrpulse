// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"dcrpulse/pkg/oggopus"
)

const (
	// AudioNoteMaxSeconds caps a voice note; at 20 ms per packet that is
	// AudioNoteMaxPackets packets.
	AudioNoteMaxSeconds = 60
	AudioNoteMaxPackets = AudioNoteMaxSeconds * 1000 / 20
)

var (
	ErrAudioNoteEmpty     = errors.New("voice note has no audio")
	ErrAudioNoteTooLong   = fmt.Errorf("voice note exceeds %d seconds", AudioNoteMaxSeconds)
	ErrAudioNoteBadPacket = errors.New("voice note packet is malformed")
	ErrAudioNoteTooLarge  = errors.New("voice note exceeds the inline embed size cap")
)

// SplitAudioNotePackets unframes the browser's packet blob: each Opus packet
// is preceded by its big-endian uint16 length.
func SplitAudioNotePackets(blob []byte) ([][]byte, error) {
	var packets [][]byte
	for len(blob) > 0 {
		if len(blob) < 2 {
			return nil, ErrAudioNoteBadPacket
		}
		n := int(binary.BigEndian.Uint16(blob))
		blob = blob[2:]
		if n == 0 || n > oggopus.MaxPacketBytes || n > len(blob) {
			return nil, ErrAudioNoteBadPacket
		}
		if len(packets) == AudioNoteMaxPackets {
			return nil, ErrAudioNoteTooLong
		}
		packets = append(packets, blob[:n])
		blob = blob[n:]
	}
	if len(packets) == 0 {
		return nil, ErrAudioNoteEmpty
	}
	return packets, nil
}

// AudioNoteFilename names a note the way bruig does, from the local time it
// was sent.
func AudioNoteFilename(now time.Time) string {
	return now.Format("2006-01-02-15_04_05") + "-audionote.opus"
}

// BuildAudioNoteBody muxes the packets into bruig's Ogg container and wraps
// it in the embed tag that is the whole PM body.
func BuildAudioNoteBody(blob []byte, now time.Time) (string, error) {
	packets, err := SplitAudioNotePackets(blob)
	if err != nil {
		return "", err
	}
	ogg, err := oggopus.OpusFile(packets)
	if err != nil {
		return "", err
	}
	if len(ogg) > MaxInlineEmbedBytes {
		return "", ErrAudioNoteTooLarge
	}
	return BuildAudioNoteEmbedTag(AudioNoteFilename(now), base64.StdEncoding.EncodeToString(ogg)), nil
}
