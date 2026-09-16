package services

import (
	"context"
	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/gamingpb"
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
		"--gaming[v=1,game=poker,authority=2,authority=1]--QUJD",
		"--gaming[v=1,game=poker, game=stakewars]--QUJD",
	} {
		if _, ok := parseGamingFrame(frame); ok {
			t.Fatal("ambiguous routing accepted")
		}
	}
}
func TestFinancialMessageRejectsTrailingAndUnknownData(t *testing.T) {
	for _, raw := range []string{
		`{"version":2} {"version":2}`,
		`{"version":2} null`,
		`{"version":2,"privateKey":"no"}`,
		`{"version":1}`,
	} {
		if _, err := decodeFinancialMessage([]byte(raw)); err == nil {
			t.Fatal("ambiguous financial message accepted")
		}
	}
	if _, err := decodeFinancialMessage([]byte(`{"version":2,"want":true}`)); err != nil {
		t.Fatal(err)
	}
}

func TestFinancialRosterRetriesAtMostOncePerBlock(t *testing.T) {
	scope := gamingfunds.Scope{Game: "stakewars", Network: "simnet", Wallet: t.Name(), Account: 1}
	if !claimFinancialRosterRetry(scope, "table", 100) {
		t.Fatal("first retry was suppressed")
	}
	if claimFinancialRosterRetry(scope, "table", 100) {
		t.Fatal("unchanged block retried the financial roster")
	}
	if !claimFinancialRosterRetry(scope, "table", 101) {
		t.Fatal("new block did not permit async healing")
	}
	releaseFinancialRosterRetry(scope, "table", 101)
	if !claimFinancialRosterRetry(scope, "table", 101) {
		t.Fatal("failed delivery was not released for retry")
	}
}
