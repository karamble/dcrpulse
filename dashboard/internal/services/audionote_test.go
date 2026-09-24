// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"
)

func framePackets(sizes ...int) []byte {
	var blob []byte
	for i, n := range sizes {
		blob = binary.BigEndian.AppendUint16(blob, uint16(n))
		for j := 0; j < n; j++ {
			blob = append(blob, byte(i+1))
		}
	}
	return blob
}

func TestSplitAudioNotePacketsReadsLengthPrefixedFrames(t *testing.T) {
	packets, err := SplitAudioNotePackets(framePackets(3, 1275, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 3 || len(packets[0]) != 3 || len(packets[1]) != 1275 || packets[2][0] != 3 {
		t.Fatalf("packets = %v", packets)
	}
}

func onePacketFrames(n int) []byte {
	sizes := make([]int, n)
	for i := range sizes {
		sizes[i] = 1
	}
	return framePackets(sizes...)
}

func TestSplitAudioNotePacketsRefusesWhatBruigCannotPlay(t *testing.T) {
	cases := []struct {
		name string
		blob []byte
		want error
	}{
		{"empty", nil, ErrAudioNoteEmpty},
		{"zero-length packet", framePackets(0), ErrAudioNoteBadPacket},
		{"oversized packet", framePackets(1276), ErrAudioNoteBadPacket},
		{"truncated packet", framePackets(5)[:4], ErrAudioNoteBadPacket},
		{"dangling length byte", []byte{0x00}, ErrAudioNoteBadPacket},
		{"one packet past 60 s", onePacketFrames(AudioNoteMaxPackets + 1), ErrAudioNoteTooLong},
	}
	for _, c := range cases {
		if _, err := SplitAudioNotePackets(c.blob); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := SplitAudioNotePackets(onePacketFrames(AudioNoteMaxPackets)); err != nil {
		t.Errorf("exactly 60 s must be accepted: %v", err)
	}
}

func TestBuildAudioNoteBodyIsBruigsWholeMessage(t *testing.T) {
	at := time.Date(2026, 9, 24, 20, 33, 52, 0, time.UTC)
	body, err := BuildAudioNoteBody(framePackets(100, 100), at)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "--embed[alt=Audio note,type=audio/ogg,filename=2026-09-24-20_33_52-audionote.opus,data="
	if !strings.HasPrefix(body, prefix) || !strings.HasSuffix(body, "]--") {
		t.Fatalf("body = %.120s...", body)
	}
	ogg, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(strings.TrimPrefix(body, prefix), "]--"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ogg[:4]) != "OggS" || string(ogg[28:36]) != "OpusHead" {
		t.Fatalf("data is not an Ogg/Opus file: %q", ogg[:36])
	}
}

func TestBuildAudioNoteBodyHonoursTheInlineCap(t *testing.T) {
	// 3000 packets of the Opus maximum is ~3.8 MB muxed, well past the cap
	// the PM handler applies to every inline embed.
	sizes := make([]int, AudioNoteMaxPackets)
	for i := range sizes {
		sizes[i] = 1275
	}
	if _, err := BuildAudioNoteBody(framePackets(sizes...), time.Now()); !errors.Is(err, ErrAudioNoteTooLarge) {
		t.Fatalf("err = %v, want %v", err, ErrAudioNoteTooLarge)
	}
}
