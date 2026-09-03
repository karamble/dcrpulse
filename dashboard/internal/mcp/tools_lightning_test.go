// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrlnd/lnrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"

	"dcrpulse/internal/rpc"
)

type stubLightning struct {
	lnrpc.LightningClient
	connectErr  error
	openErr     error
	decodeAtoms int64
}

func (s stubLightning) ConnectPeer(context.Context, *lnrpc.ConnectPeerRequest, ...grpc.CallOption) (*lnrpc.ConnectPeerResponse, error) {
	return &lnrpc.ConnectPeerResponse{}, s.connectErr
}

func (s stubLightning) OpenChannelSync(context.Context, *lnrpc.OpenChannelRequest, ...grpc.CallOption) (*lnrpc.ChannelPoint, error) {
	return nil, s.openErr
}

// AddInvoice lets a test drive invoice creation without a daemon.
func (s stubLightning) AddInvoice(_ context.Context, in *lnrpc.Invoice, _ ...grpc.CallOption) (*lnrpc.AddInvoiceResponse, error) {
	return &lnrpc.AddInvoiceResponse{PaymentRequest: "lnbogus", RHash: []byte{0xab, 0xcd}}, nil
}

// GetInfo is reached when a tool resolves the configured liquidity provider for
// the audit trail. Failing it exercises the best-effort fallback, which leaves
// the provider unnamed rather than failing the call.
func (s stubLightning) GetInfo(context.Context, *lnrpc.GetInfoRequest, ...grpc.CallOption) (*lnrpc.GetInfoResponse, error) {
	return nil, errors.New("no node info in this test")
}

// LookupInvoice is the second roundtrip AddLightningInvoice makes; failing it
// exercises the documented fallback to a minimal record rather than needing a
// full invoice fixture.
func (s stubLightning) LookupInvoice(context.Context, *lnrpc.PaymentHash, ...grpc.CallOption) (*lnrpc.Invoice, error) {
	return nil, errors.New("no invoice store in this test")
}

// decodeAtoms lets a test drive a tool that decodes an invoice before it reaches
// the cap check, without a daemon.
func (s stubLightning) DecodePayReq(context.Context, *lnrpc.PayReqString, ...grpc.CallOption) (*lnrpc.PayReq, error) {
	return &lnrpc.PayReq{NumAtoms: s.decodeAtoms, Destination: "02deadbeef"}, nil
}

const lnTestPeer = "03aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899@127.0.0.1:9735"

// TestOpenChannelKeepsReservationOnCommittedFailure is the regression guard for
// the refund defect: dcrlnd detaches the funding workflow from the caller's
// context, so releasing the cap on a failed - or cancelled - open would let an
// agent fund channels indefinitely inside a fixed daily cap.
func TestOpenChannelKeepsReservationOnCommittedFailure(t *testing.T) {
	spentAfter := func(t *testing.T, client lnrpc.LightningClient) int64 {
		t.Helper()
		prev := rpc.SwapDcrlndClients(rpc.DcrlndClients{Lightning: client})
		t.Cleanup(func() { rpc.SwapDcrlndClients(prev) })

		const agentID = "ln-refund"
		grants.set(agentID, GrantSpec{
			WriteScopes: []string{scopeLightning},
			PerTxAtoms:  5e8,
			DailyAtoms:  5e8,
		}, time.Now())
		t.Cleanup(func() { grants.revoke(agentID) })

		cs := connectTo(t, testAgent(agentID, "ln", map[string]bool{"lightning": true}))
		if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "ln_open_channel",
			Arguments: map[string]any{"peerUri": lnTestPeer, "localDcr": 1.0},
		}); err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		info, ok := SpendGrantInfo(agentID)
		if !ok {
			t.Fatal("grant vanished")
		}
		return info.SpentAtoms
	}

	t.Run("a funding failure keeps the reservation", func(t *testing.T) {
		if got := spentAfter(t, stubLightning{openErr: errors.New("boom")}); got == 0 {
			t.Fatal("reservation was refunded after dcrlnd had been asked to fund")
		}
	})

	t.Run("a failure before funding refunds", func(t *testing.T) {
		if got := spentAfter(t, stubLightning{connectErr: errors.New("no route to host")}); got != 0 {
			t.Fatalf("spent %d atoms after a pre-funding failure, want the reservation released", got)
		}
	})
}

// TestAddInvoiceRecordsNoAmount pins the direction of an invoice in the trail.
// notifySpend renders any successful entry with a positive amount as money the
// agent sent, so recording the requested figure as an amount would tell the
// operator over Bison Relay that an incoming request was an outgoing payment.
func TestAddInvoiceRecordsNoAmount(t *testing.T) {
	prev := rpc.SwapDcrlndClients(rpc.DcrlndClients{Lightning: stubLightning{}})
	t.Cleanup(func() { rpc.SwapDcrlndClients(prev) })

	const agentID = "ln-invoice-direction"
	grants.set(agentID, GrantSpec{WriteScopes: []string{scopeLightning}}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "ln", map[string]bool{"lightning": true}))
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "ln_add_invoice",
		Arguments: map[string]any{"amountDcr": 50.0, "memo": "x"},
	}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	got := AuditLog(1)
	if len(got) != 1 {
		t.Fatalf("AuditLog returned %d entries, want 1", len(got))
	}
	if got[0].Tool != "ln_add_invoice" {
		t.Fatalf("newest audit entry is %q, want ln_add_invoice", got[0].Tool)
	}
	// notifySpend's outbound case is exactly Result=="ok" && AmountDCR>0, so a
	// zero amount is what keeps an invoice out of it. Sending the notification
	// itself needs brclientd, so this asserts the recorded entry rather than the
	// message.
	if got[0].Result == "ok" && got[0].AmountDCR > 0 {
		t.Errorf("an invoice was recorded as %.8f DCR sent; it asks for money in", got[0].AmountDCR)
	}
	if !strings.Contains(got[0].Detail, "requested") {
		t.Errorf("the requested amount was dropped from the trail: %q", got[0].Detail)
	}
}

// TestLiquidityToolsRejectACallerProvider pins that the liquidity provider and
// its certificate are the dashboard's to choose. The request tool pays that
// provider a fee, so a caller naming it would pick who gets paid, and a caller
// supplying its certificate would vouch for them too.
func TestLiquidityToolsRejectACallerProvider(t *testing.T) {
	const agentID = "ln-provider"
	grants.set(agentID, GrantSpec{
		WriteScopes: []string{scopeLightning}, PerTxAtoms: 1e8, DailyAtoms: 1e8,
	}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })
	cs := connectTo(t, testAgent(agentID, "ln", map[string]bool{"lightning": true}))

	for _, tool := range []string{"ln_liquidity_estimate", "ln_liquidity_request"} {
		for _, field := range []string{"server", "certPem"} {
			args := map[string]any{"chanSizeDcr": 1.0, "approvedFeeDcr": 0.5, field: "x"}
			out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
			// A removed field is rejected by schema validation rather than
			// silently dropped, so it surfaces either as a call error or an
			// error result; both mean the caller cannot set it.
			if err == nil && !out.IsError {
				t.Errorf("%s accepted a caller-supplied %q", tool, field)
			}
		}
	}

	// And the catalogue must not invite it either.
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range res.Tools {
		if tl.Name != "ln_liquidity_estimate" && tl.Name != "ln_liquidity_request" {
			continue
		}
		schema, _ := json.Marshal(tl.InputSchema)
		for _, field := range []string{"server", "certPem"} {
			if strings.Contains(string(schema), field) {
				t.Errorf("%s still advertises %q in its input schema", tl.Name, field)
			}
		}
	}
}
