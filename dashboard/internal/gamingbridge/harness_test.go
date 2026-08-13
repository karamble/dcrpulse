// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"context"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"sync/atomic"
	"testing"
	"time"

	"github.com/decred/dcrd/certgen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"dcrpulse/internal/gamingpb"
)

// The rig is one whole bridge: the real service, on a real loopback socket,
// behind real mutual TLS, with credentials minted by the same code the
// dashboard's button calls.
//
// None of that is scenery. A harness that faked the transport would assert
// nothing about the transport, and the promises being made here are almost all
// transport promises: that a stranger's certificate is refused, that a revoked
// one stops working, that a game is told what it is rather than asked. An
// in-memory pipe would hand the client a dialer and quietly assume away the
// part under test.
//
// What a game is given is an address and a credential, and the rig hands over
// exactly that and nothing else - which is the whole of the claim that a game
// needs no inbound port and no route back to it.

// dialDeadline bounds anything that could hang. It is never the assertion: a
// test that passes because something took less than a second is a test that
// will fail on a slow machine for no reason.
const dialDeadline = 10 * time.Second

// gameCreds is everything a game is ever handed.
type gameCreds struct {
	Game       string
	CertPEM    []byte
	KeyPEM     []byte
	BridgeCert []byte
}

type bridgeRig struct {
	Addr       string
	BridgeCert []byte

	srv   *Server
	allow *Allowlist

	// appPassword and enabled are the two halves of whether the bridge is
	// answering, flippable mid-test because that is exactly what an operator
	// does.
	appPassword atomic.Bool
	enabled     atomic.Bool
}

// newBridgeRig stands the bridge up and tears it down again.
func newBridgeRig(t *testing.T) *bridgeRig {
	t.Helper()

	certPEM, keyPEM, err := certgen.NewTLSCertPair(
		elliptic.P256(), "dcrpulse gaming bridge test", time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatalf("mint the bridge's own certificate: %v", err)
	}

	r := &bridgeRig{BridgeCert: certPEM, allow: NewAllowlist()}
	r.appPassword.Store(true)
	r.enabled.Store(true)

	srv, err := New(Config{
		Addr:              "127.0.0.1:0",
		ServerCert:        certPEM,
		ServerKey:         keyPEM,
		AppPasswordActive: r.appPassword.Load,
		Enabled:           r.enabled.Load,
		Allow:             r.allow,
	})
	if err != nil {
		t.Fatalf("prepare the bridge: %v", err)
	}
	r.srv = srv

	serving := make(chan error, 1)
	go func() { serving <- srv.Serve() }()

	// Serve binds before it blocks, but not before it returns, so wait for
	// the address rather than guessing at a delay.
	deadline := time.Now().Add(dialDeadline)
	for r.srv.Addr() == "" {
		select {
		case err := <-serving:
			t.Fatalf("the bridge stopped before it listened: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the bridge never started listening")
		}
		time.Sleep(time.Millisecond)
	}
	r.Addr = r.srv.Addr()

	t.Cleanup(func() { srv.Stop() })
	return r
}

// register issues a game its credential the way the dashboard does.
//
// Through the production path on purpose: a credential minted specially for a
// test would let cert-as-identity pass against a certificate shape no operator
// will ever hold.
func (r *bridgeRig) register(t *testing.T, game string) gameCreds {
	t.Helper()
	c, err := GenerateGameCredential(game)
	if err != nil {
		t.Fatalf("issue a credential for %q: %v", game, err)
	}
	if err := r.allow.Add(c); err != nil {
		t.Fatalf("admit %q: %v", game, err)
	}
	return gameCreds{Game: game, CertPEM: c.CertPEM, KeyPEM: c.KeyPEM, BridgeCert: r.BridgeCert}
}

func (r *bridgeRig) revoke(game string) { r.allow.Revoke(game) }

// deliver stands in for Bison Relay: it is the bridge learning something and
// handing it to the game it is addressed to.
func (r *bridgeRig) Deliver(game string, req *gamingpb.BridgeRequest) {
	r.srv.Deliver(game, req)
}

// clientTLS is what a game configures itself with: its own credential, and the
// bridge's certificate to pin.
func clientTLS(t *testing.T, c gameCreds) *tls.Config {
	t.Helper()
	pair, err := tls.X509KeyPair(c.CertPEM, c.KeyPEM)
	if err != nil {
		t.Fatalf("load the credential: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(c.BridgeCert) {
		t.Fatal("the bridge's certificate did not parse")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{pair},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
		// The bridge is pinned by its certificate, and an operator reaches
		// it by address rather than by a name anything issued.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			leaf, err := x509.ParseCertificate(raw[0])
			if err != nil {
				return err
			}
			_, err = leaf.Verify(x509.VerifyOptions{Roots: pool})
			return err
		},
	}
}

// dial connects as a game would.
func (r *bridgeRig) dial(t *testing.T, c gameCreds) gamingpb.BridgeServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(r.Addr,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS(t, c))))
	if err != nil {
		t.Fatalf("dial the bridge: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return gamingpb.NewBridgeServiceClient(conn)
}

// callCtx bounds one call. The deadline is a hang guard; what is asserted is
// always what came back.
func callCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), dialDeadline)
}
