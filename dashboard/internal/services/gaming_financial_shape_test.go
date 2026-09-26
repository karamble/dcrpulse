package services

import (
	"bytes"
	"context"
	"dcrpulse/internal/gamingpb"
	"reflect"
	"testing"
)

// Malformed requests must fail before any wallet, node, or store access.
func TestPayoutRejectsMalformedArraysBeforeWalletAccess(t *testing.T) {
	for _, req := range []*gamingpb.ProposePayoutRequest{
		nil,
		{},
		{Inputs: []*gamingpb.PayoutInput{nil, nil}, Payments: []*gamingpb.PayoutPayment{{}}},
		{Inputs: []*gamingpb.PayoutInput{{}, {}}, Payments: []*gamingpb.PayoutPayment{nil}},
		{Inputs: make([]*gamingpb.PayoutInput, 14), Payments: []*gamingpb.PayoutPayment{{}}},
	} {
		if _, err := ProposeGamingPayout(context.Background(), "stakewars", req); err == nil {
			t.Fatal("malformed payout accepted")
		}
	}
}

func TestGamingEnvelopeRejectsDuplicateRoutingFields(t *testing.T) {
	for _, frame := range []string{
		"--gaming[v=1,game=poker,game=stakewars]--QUJD",
		"--gaming[v=1,game=poker,authority=3,authority=1]--QUJD",
		"--gaming[v=1,game=poker, game=stakewars]--QUJD",
	} {
		if _, ok := parseGamingFrame(frame); ok {
			t.Fatal("ambiguous routing accepted")
		}
	}
}

func TestFinancialParticipantMessageRoundTrip(t *testing.T) {
	key := "0344e0ea14b52801d13e46ce0d4e815ba7633ce2a48c0b4a3cc2cd2d776aa7ea37"
	proof := bytes.Repeat([]byte{7}, 71)
	for _, want := range []financialMessage{
		{Key: key, Proof: proof},
		{Key: key, RosterHash: "ad0c1dd7e0b540afe39b0a5ae9a6095352bad9e07e6ae77cf2592af2d074548f", Proof: proof},
	} {
		raw, err := encodeFinancialMessage(want)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := decodeFinancialMessage(raw); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("round trip = %#v, %v", got, err)
		}
		if len(raw) != 34+1+71 && len(raw) != 66+1+71 {
			t.Fatalf("participant message is %d bytes", len(raw))
		}
	}
	if _, err := encodeFinancialMessage(financialMessage{Key: key}); err == nil {
		t.Fatal("a key announcement without its proof was encoded")
	}
}

func TestFinancialSettlementMessageRoundTrip(t *testing.T) {
	want := financialMessage{
		Settlement: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Signatures: [][]byte{{1, 2, 3}, {4, 5}},
	}
	raw, err := encodeFinancialMessage(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeFinancialMessage(raw)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, %v", got, err)
	}
}

func TestFinancialMessageRejectsMalformedData(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 33)
	for _, raw := range [][]byte{
		nil,
		{0x20}, // retired wire version
		append(append(append([]byte{0x31}, key...), 1), 9), // version 3 announcement
		{0x40}, // no message kind
		{0x43}, // ambiguous message kinds
		append(append(append([]byte{0x45}, key...), 1), 9),   // retired want/reply flag
		append([]byte{0x41}, key[:32]...),                    // short participant key
		append([]byte{0x41}, key...),                         // no proof
		append(append([]byte{0x41}, key...), 0),              // empty proof
		append(append([]byte{0x41}, key...), 2, 9),           // proof shorter than its length
		append(append([]byte{0x41}, key...), 1, 9, 9),        // trailing participant data
		append(bytes.Repeat([]byte{0}, 33), 1),               // invalid version and body
		append([]byte{0x42}, bytes.Repeat([]byte{0}, 32)...), // missing signature count
	} {
		if _, err := decodeFinancialMessage(raw); err == nil {
			t.Fatalf("malformed financial message accepted: %x", raw)
		}
	}
}
