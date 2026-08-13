// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"dcrpulse/internal/gamingpb"
)

// parseCert reads back a credential the bridge issued.
func parseCert(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("the credential did not parse as PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse the credential: %v", err)
	}
	return cert
}

// peerCtx is the context a handshake leaves behind, as gRPC builds it.
//
// The certificate goes in VerifiedChains because that is where the interceptor
// reads from - the chain the TLS stack accepted, rather than the certificates a
// peer merely offered.
func peerCtx(cert *x509.Certificate) context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{
			VerifiedChains: [][]*x509.Certificate{{cert}},
		}},
	})
}

// runUnary drives the production interceptor and reports who the call was.
func runUnary(t *testing.T, s *Server, ctx context.Context) (string, error) {
	t.Helper()
	var seen string
	_, err := s.unaryIdentity(ctx, nil,
		&grpc.UnaryServerInfo{FullMethod: "/gamingpb.BridgeService/ChainTip"},
		func(ctx context.Context, _ any) (any, error) {
			seen = callerGame(ctx)
			return nil, nil
		})
	return seen, err
}

// The certificate says who a caller is. Nothing the caller says does.
//
// This is the whole of the identity model, and it is easy to get wrong in a way
// that looks fine: a bridge that trusted a name in the certificate's subject, or
// a game id in the request, would let one game spend against another's caps by
// asking to. Here the subject deliberately disagrees with the registration -
// certgen writes the machine's hostname into the common name, so every
// credential this bridge issues claims to be the same thing - and the answer
// must come from the fingerprint regardless.
func TestTheCertificateOutranksWhatACallerClaims(t *testing.T) {
	r := newBridgeRig(t)
	creds := r.register(t, "poker")
	cert := parseCert(t, creds.CertPEM)

	if cert.Subject.CommonName == "poker" {
		t.Fatal("this test needs a certificate whose subject disagrees with the registration, and it does not")
	}

	game, err := runUnary(t, r.srv, peerCtx(cert))
	if err != nil {
		t.Fatalf("a registered credential was refused: %v", err)
	}
	if game != "poker" {
		t.Fatalf("the call was attributed to %q, but the certificate is registered to poker: "+
			"identity is being taken from somewhere other than the fingerprint", game)
	}
}

// Re-registering a game retires the credential it replaces.
//
// Regenerating is what an operator does when a machine is lost. If the old
// credential kept working there would be two ways in and only one of them known
// about, which is the opposite of what regenerating is for.
func TestRegeneratingRetiresTheOldCredential(t *testing.T) {
	r := newBridgeRig(t)
	old := parseCert(t, r.register(t, "poker").CertPEM)
	fresh := parseCert(t, r.register(t, "poker").CertPEM)

	if _, err := runUnary(t, r.srv, peerCtx(fresh)); err != nil {
		t.Fatalf("the credential just issued was refused: %v", err)
	}
	if _, err := runUnary(t, r.srv, peerCtx(old)); err == nil {
		t.Fatal("the credential that was replaced still works, so regenerating left two ways in")
	}
}

// Revoking withdraws a credential from the next thing it tries.
//
// Not at the next reconnect: an operator revoking has decided that machine is
// not to spend any more, and a credential that stayed good until the game chose
// to disconnect would leave the timing of that in the game's hands.
func TestARevokedCredentialStopsResolving(t *testing.T) {
	r := newBridgeRig(t)
	cert := parseCert(t, r.register(t, "poker").CertPEM)

	if _, err := runUnary(t, r.srv, peerCtx(cert)); err != nil {
		t.Fatalf("a registered credential was refused: %v", err)
	}

	r.revoke("poker")

	if _, err := runUnary(t, r.srv, peerCtx(cert)); err == nil {
		t.Fatal("a revoked credential still identifies its game, so revoking withdrew nothing")
	}
}

// A revoked credential cannot open a fresh connection either.
//
// The roots a handshake verifies against are rebuilt per handshake for exactly
// this reason. A pool fixed when the listener started would go on admitting a
// withdrawn credential until something restarted the appliance, which is not a
// revocation an operator could rely on.
func TestARevokedCredentialCannotDialBackIn(t *testing.T) {
	r := newBridgeRig(t)
	creds := r.register(t, "poker")

	r.revoke("poker")

	if err := refusedConnection(t, r.Addr, clientTLS(t, creds)); err == nil {
		t.Fatal("a revoked credential opened a new connection, so revoking waits on a restart")
	}
}

// A refusal describes nothing.
//
// Whoever was just turned away is, by definition, not someone this appliance
// wants to talk to, so the refusal must not distinguish a bridge that is off
// from one that was never built from a credential that used to work. It is the
// posture the browser tunnel already takes with 404.
func TestARefusalDescribesNothing(t *testing.T) {
	r := newBridgeRig(t)
	cert := parseCert(t, r.register(t, "poker").CertPEM)
	r.revoke("poker")

	_, err := runUnary(t, r.srv, peerCtx(cert))
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("the refusal was not a gRPC status: %v", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Errorf("a refused caller was told %v, which distinguishes this bridge from one that has no gaming in it",
			st.Code())
	}
	if st.Message() != "" {
		t.Errorf("the refusal said %q, which tells a stranger something about this host", st.Message())
	}
}

// The bridge stops answering the moment the App Password does.
//
// The caps and the approval a spend waits on are worth exactly as much as the
// certainty that whoever is approving is the operator. If the gate came down and
// the port kept answering until the next restart, the money routes would be
// reachable in the window nobody was watching.
func TestTheBridgeStopsWhenTheAppPasswordDoes(t *testing.T) {
	r := newBridgeRig(t)
	cert := parseCert(t, r.register(t, "poker").CertPEM)

	if _, err := runUnary(t, r.srv, peerCtx(cert)); err != nil {
		t.Fatalf("a registered credential was refused while the gate was up: %v", err)
	}

	r.appPassword.Store(false)

	if _, err := runUnary(t, r.srv, peerCtx(cert)); err == nil {
		t.Fatal("the bridge went on answering after the App Password was removed")
	}
}

// Switching the bridge off stops it answering, without a restart.
func TestSwitchingTheBridgeOffStopsIt(t *testing.T) {
	r := newBridgeRig(t)
	cert := parseCert(t, r.register(t, "poker").CertPEM)

	r.enabled.Store(false)

	if _, err := runUnary(t, r.srv, peerCtx(cert)); err == nil {
		t.Fatal("the bridge answered a call after the operator switched it off")
	}
}

// A connection that never proved anything is nobody.
//
// The interceptor is the last thing between a peer and a handler, and it must
// not fall back to admitting a caller it could not identify - which is what an
// unverified connection or a missing peer looks like.
func TestAnUnverifiedConnectionIsNobody(t *testing.T) {
	r := newBridgeRig(t)
	r.register(t, "poker")

	if _, err := runUnary(t, r.srv, context.Background()); err == nil {
		t.Fatal("a call carrying no connection at all was let through")
	}

	empty := peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{}},
	})
	if _, err := runUnary(t, r.srv, empty); err == nil {
		t.Fatal("a connection with no verified chain was let through")
	}
}

// A stream is identified the same way a call is.
//
// The subscription is the one long-lived thing here and the one that carries
// every frame a game receives. An identity check that covered the unary calls
// and not the stream would leave the busiest route unguarded.
func TestTheStreamIsIdentifiedTheSameWay(t *testing.T) {
	r := newBridgeRig(t)
	client := r.dial(t, r.register(t, "poker"))

	ctx, cancel := callCtx(t)
	defer cancel()
	stream, err := client.Subscribe(ctx, &gamingpb.SubscribeRequest{})
	if err != nil {
		t.Fatalf("open a subscription: %v", err)
	}
	_, err = stream.Recv()
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("the subscription failed with something that is not a status: %v", err)
	}
	// Unimplemented either way while Subscribe has no body. The message is
	// what separates them: named means a handler was reached, empty means the
	// caller was refused before one was.
	if st.Code() == codes.Unimplemented && !strings.Contains(st.Message(), "Subscribe") {
		t.Fatalf("a registered game's subscription was refused as a stranger's (%q), "+
			"so the stream is not resolving identity the way calls do", st.Message())
	}
}
