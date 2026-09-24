// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package oggopus

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type page struct {
	typ     byte
	granule uint64
	serial  uint32
	seq     uint32
	lacing  []byte
	payload []byte
}

// parsePages walks the container the way a decoder would, so the assertions
// below are about the bytes on the wire rather than the writer's bookkeeping.
func parsePages(t *testing.T, b []byte) []page {
	t.Helper()
	var pages []page
	for len(b) > 0 {
		if len(b) < 27 || string(b[:4]) != "OggS" || b[4] != 0 {
			t.Fatalf("bad page header at offset %d", len(b))
		}
		n := int(b[26])
		lacing := b[27 : 27+n]
		size := 0
		for _, l := range lacing {
			size += int(l)
		}
		end := 27 + n + size
		pages = append(pages, page{
			typ:     b[5],
			granule: binary.LittleEndian.Uint64(b[6:]),
			serial:  binary.LittleEndian.Uint32(b[14:]),
			seq:     binary.LittleEndian.Uint32(b[18:]),
			lacing:  lacing,
			payload: b[27+n : end],
		})
		b = b[end:]
	}
	return pages
}

func TestOpusFileWritesTheContainerBruigWrites(t *testing.T) {
	packets := [][]byte{
		bytes.Repeat([]byte{0xA1}, 100),
		bytes.Repeat([]byte{0xB2}, 255),
		bytes.Repeat([]byte{0xC3}, 300),
	}
	out, err := opusFileWithSerial(packets, 0x01020304)
	if err != nil {
		t.Fatal(err)
	}
	pages := parsePages(t, out)
	if len(pages) != 2+len(packets) {
		t.Fatalf("pages = %d, want two headers plus one per packet", len(pages))
	}

	head := pages[0]
	if head.typ != 0x02 || head.granule != 0 || head.seq != 0 {
		t.Errorf("OpusHead page flags/granule/seq = %#x/%d/%d, want BOS/0/0", head.typ, head.granule, head.seq)
	}
	if len(head.payload) != 19 || string(head.payload[:8]) != "OpusHead" {
		t.Fatalf("OpusHead payload = %q", head.payload)
	}
	// bruig declares two channels although the audio is mono, and a note
	// must match it byte for byte.
	if head.payload[8] != 1 || head.payload[9] != 2 {
		t.Errorf("OpusHead version/channels = %d/%d, want 1/2", head.payload[8], head.payload[9])
	}
	if binary.LittleEndian.Uint16(head.payload[10:]) != 0 || binary.LittleEndian.Uint32(head.payload[12:]) != SampleRate {
		t.Errorf("OpusHead pre-skip/rate = %d/%d", binary.LittleEndian.Uint16(head.payload[10:]), binary.LittleEndian.Uint32(head.payload[12:]))
	}

	tags := pages[1]
	if tags.typ != 0 || tags.seq != 1 || string(tags.payload[:8]) != "OpusTags" {
		t.Errorf("OpusTags page = %#x/%d/%q", tags.typ, tags.seq, tags.payload[:8])
	}
	if got := string(tags.payload[12:21]); got != "skynetbot" || binary.LittleEndian.Uint32(tags.payload[8:]) != 9 {
		t.Errorf("OpusTags vendor = %q", got)
	}

	for i, p := range pages[2:] {
		last := i == len(packets)-1
		if want := uint64(FrameSamples * (i + 1)); p.granule != want {
			t.Errorf("packet %d granule = %d, want %d (running total including this packet)", i, p.granule, want)
		}
		if p.seq != uint32(i+2) {
			t.Errorf("packet %d seq = %d, want %d", i, p.seq, i+2)
		}
		if (p.typ == 0x04) != last {
			t.Errorf("packet %d EOS flag = %v, want %v", i, p.typ == 0x04, last)
		}
		if p.serial != 0x01020304 {
			t.Errorf("packet %d serial = %#x", i, p.serial)
		}
		if !bytes.Equal(p.payload, packets[i]) {
			t.Errorf("packet %d payload changed", i)
		}
	}
	// An exact multiple of 255 is terminated by a zero lacing value.
	if l := pages[3].lacing; len(l) != 2 || l[0] != 255 || l[1] != 0 {
		t.Errorf("255-byte packet lacing = %v, want [255 0]", l)
	}
	if l := pages[4].lacing; len(l) != 2 || l[0] != 255 || l[1] != 45 {
		t.Errorf("300-byte packet lacing = %v, want [255 45]", l)
	}
}

func TestOpusFileIsDeterministicForAPinnedSerial(t *testing.T) {
	packets := [][]byte{{1, 2, 3}, {4, 5}}
	a, _ := opusFileWithSerial(packets, 7)
	b, _ := opusFileWithSerial(packets, 7)
	if !bytes.Equal(a, b) {
		t.Fatal("same packets and serial produced different bytes")
	}
	c, _ := opusFileWithSerial(packets, 8)
	if bytes.Equal(a, c) {
		t.Fatal("serial is not reaching the page header")
	}
}

func TestOpusFileRefusesEmptyAndOversizedInput(t *testing.T) {
	if _, err := OpusFile(nil); err == nil {
		t.Fatal("no packets should not produce a file")
	}
	if _, err := OpusFile([][]byte{make([]byte, 255*255+1)}); err == nil {
		t.Fatal("a packet needing page splitting must be refused")
	}
}
