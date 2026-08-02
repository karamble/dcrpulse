// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWireRoundtrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	payload := []byte(`{"type":"ready","tempId":"aabbccdd00112233","walletId":"DcXyz"}`)
	mid, err := NewID()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if len(mid) != 16 || !midRE.MatchString(mid) {
		t.Fatalf("mid shape: %q", mid)
	}
	body, err := Encode(payload, mid, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !IsEnvelope(body) {
		t.Fatalf("encoded frame is not classified as an envelope")
	}
	frame, err := Parse(body, now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if frame.MID != mid || !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestWireExpiry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	payload := []byte(`{"type":"x"}`)
	body, err := Encode(payload, "00aa", now)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Inside the skew grace: accepted.
	if _, err := Parse(body, now.Add(ClockSkewGrace-time.Second)); err != nil {
		t.Fatalf("parse within grace: %v", err)
	}
	// Past the grace: expired.
	if _, err := Parse(body, now.Add(ClockSkewGrace+time.Second)); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestWireParseRejections(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	if _, err := Parse("just a chat message", now); !errors.Is(err, ErrNotEnvelope) {
		t.Fatalf("chat text: %v", err)
	}
	if _, err := Parse("--mcp[v=1,sid=aa,mid=bb,seq=1/1,exp=1]--QQ==", now); !errors.Is(err, ErrNotEnvelope) {
		t.Fatalf("mcp frame: %v", err)
	}
	if _, err := Parse("--msig[v=2,mid=aa,exp=9999999999]--QQ==", now); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("future version: %v", err)
	}
	if _, err := Parse("--msig[mid=aa,exp=9999999999]--QQ==", now); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("missing version: %v", err)
	}
	if _, err := Parse("--msig[v=1,exp=9999999999]--QQ==", now); err == nil {
		t.Fatalf("missing mid accepted")
	}
	if _, err := Parse("--msig[v=1,mid=aa]--QQ==", now); err == nil {
		t.Fatalf("missing exp accepted")
	}
	// The coarse classifier lets base64-alphabet words through; the
	// strict decode must reject them.
	if _, err := Parse("--msig[v=1,mid=aa,exp=9999999999]--QQ== trailing", now); err == nil {
		t.Fatalf("junk payload accepted")
	}
}

func TestWireForwardCompatAndWrapping(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	// Unknown header keys are tolerated.
	frame, err := Parse("--msig[v=1,mid=aa,exp=9999999999,zz=later]--aGVsbG8=", now)
	if err != nil {
		t.Fatalf("unknown key: %v", err)
	}
	if string(frame.Payload) != "hello" {
		t.Fatalf("payload: %q", frame.Payload)
	}
	// Whitespace-wrapped base64 decodes.
	frame, err = Parse("--msig[v=1,mid=aa,exp=9999999999]--aGVs\nbG8=", now)
	if err != nil {
		t.Fatalf("wrapped payload: %v", err)
	}
	if string(frame.Payload) != "hello" {
		t.Fatalf("wrapped payload: %q", frame.Payload)
	}
}

func TestWireCaps(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	big := bytes.Repeat([]byte{0x41}, MaxPayload+1)
	if _, err := Encode(big, "aa", now.Add(time.Hour)); err == nil {
		t.Fatalf("oversize payload encoded")
	}
	if _, err := Encode(nil, "aa", now.Add(time.Hour)); err == nil {
		t.Fatalf("empty payload encoded")
	}
	if _, err := Encode([]byte("x"), "not-hex", now.Add(time.Hour)); err == nil {
		t.Fatalf("bad mid encoded")
	}
	huge := "--msig[v=1,mid=aa,exp=9999999999]--" + strings.Repeat("A", maxBody)
	if _, err := Parse(huge, now); !errors.Is(err, ErrNotEnvelope) {
		t.Fatalf("oversize body: %v", err)
	}
}

func TestSampleEnvelopeParses(t *testing.T) {
	before := time.Unix(1_600_000_000, 0)
	frame, err := Parse(SampleEnvelope, before)
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if string(frame.Payload) != "hello" {
		t.Fatalf("sample payload: %q", frame.Payload)
	}
	if !IsEnvelope(SampleEnvelope) {
		t.Fatalf("sample not classified")
	}
}
