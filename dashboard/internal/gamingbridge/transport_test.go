// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"crypto/elliptic"
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/certgen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"dcrpulse/internal/gamingpb"
)

// These assert the transport itself, and they pass from the moment the
// listener exists - which is right, because the transport is the one thing the
// harness cannot fake and therefore the one thing built for real first. They
// are here to stay true, not to go green later.

// A game's credential must never cross a wire in the clear.
//
// The port is the one thing about this bridge that may face a network, and the
// credential travelling it is an identity that spends money. A listener that
// answered plaintext at all would make every other promise here conditional on
// nobody having tried.
func TestThePlainClientGetsNothing(t *testing.T) {
	r := newBridgeRig(t)

	conn, err := grpc.NewClient(r.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("build a plaintext client: %v", err)
	}
	defer conn.Close()

	ctx, cancel := callCtx(t)
	defer cancel()
	_, err = gamingpb.NewBridgeServiceClient(conn).ChainTip(ctx, &gamingpb.ChainTipRequest{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("a caller that brought no encryption at all reached the service and was answered %v: "+
			"a credential that spends money would cross this wire in the clear", status.Code(err))
	}
}

// A raw socket learns nothing by connecting.
func TestARawSocketLearnsNothing(t *testing.T) {
	r := newBridgeRig(t)

	c, err := net.DialTimeout("tcp", r.Addr, dialDeadline)
	if err != nil {
		t.Fatalf("open a socket: %v", err)
	}
	defer c.Close()

	// The HTTP/2 preface, which is what a plaintext gRPC client would open
	// with. A TLS listener must not answer it with anything usable.
	if _, err := c.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")); err != nil {
		return // refused outright, which is the same answer
	}
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err == nil && n > 0 && !looksLikeTLSAlert(buf[:n]) {
		t.Fatalf("a plaintext socket got %d bytes of something back: %q", n, buf[:n])
	}
}

// looksLikeTLSAlert reports whether the bytes are a TLS record rather than an
// answer. A record type of 21 is an alert, 22 a handshake.
func looksLikeTLSAlert(b []byte) bool {
	return len(b) > 0 && (b[0] == 21 || b[0] == 22)
}

// refusedConnection reports whether a caller with this configuration never
// reached the service, and nil if it did.
//
// It has to make a real call to find out, for two reasons. Under TLS 1.3 the
// client sends its certificate in its last flight and finishes without waiting
// to hear whether the server accepted it, so a rejected credential is not
// visible in the dial. And a refusal has to be told apart from an answer: what a
// method with no body returns is still proof the caller got in.
//
// Unavailable is the line. gRPC reports a connection that could not be
// established that way, while anything the service itself produced - including
// Unimplemented - means the caller was admitted.
func refusedConnection(t *testing.T, addr string, cfg *tls.Config) error {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := callCtx(t)
	defer cancel()
	_, err = gamingpb.NewBridgeServiceClient(conn).ChainTip(ctx, &gamingpb.ChainTipRequest{})
	if status.Code(err) == codes.Unavailable {
		return err
	}
	return nil
}

// Turning up without a credential is not a way in.
//
// The certificate is the identity, so a connection that carries none has
// nobody to be. Accepting it and sorting the caller out later would mean an
// unauthenticated peer had already reached the service.
func TestAConnectionWithoutACredentialIsRefused(t *testing.T) {
	r := newBridgeRig(t)

	err := refusedConnection(t, r.Addr, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err == nil {
		t.Fatal("the bridge held a usable connection to a peer that presented no certificate")
	}
}

// A well-formed credential nobody issued is a stranger.
//
// This is the case that matters: not a malformed certificate, but a perfectly
// valid one minted somewhere else. Membership of the allowlist is the whole of
// the trust decision, so anything outside it must fail at the handshake rather
// than reaching a handler that would then have to decide.
func TestAStrangersCredentialIsRefused(t *testing.T) {
	r := newBridgeRig(t)

	strangerCert, strangerKey, err := certgen.NewTLSCertPair(
		elliptic.P256(), "somebody else entirely", time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatalf("mint a stranger's certificate: %v", err)
	}

	stranger := gameCreds{
		Game: "stranger", CertPEM: strangerCert, KeyPEM: strangerKey, BridgeCert: r.BridgeCert,
	}
	if err := refusedConnection(t, r.Addr, clientTLS(t, stranger)); err == nil {
		t.Fatal("the bridge admitted a credential it never issued")
	}
}

// A registered credential does reach the service.
//
// The counterweight to everything above: a harness whose transport refused
// everybody would pass those tests and prove nothing at all. The assertion is a
// completed round trip, not a completed dial - under TLS 1.3 a dial finishes
// before the server has said anything, so only an answer proves arrival.
//
// What comes back is Unimplemented, because no method has a body yet. That it
// names the method is the tell: the empty-message Unimplemented is what a caller
// the bridge would not identify is given, so a named one means this call was
// identified and reached a handler.
func TestARegisteredCredentialReachesTheService(t *testing.T) {
	r := newBridgeRig(t)
	client := r.dial(t, r.register(t, "poker"))

	ctx, cancel := callCtx(t)
	defer cancel()
	_, err := client.ChainTip(ctx, &gamingpb.ChainTipRequest{})
	if err == nil {
		return // a real answer is arrival too, once ChainTip has a body
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("a credential this bridge issued got no answer at all: %v", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Fatalf("a credential this bridge issued was turned away with %v: %v", st.Code(), st.Message())
	}
	if !strings.Contains(st.Message(), "ChainTip") {
		t.Fatalf("a registered game was answered the way a stranger is (%q), so its certificate did not identify it",
			st.Message())
	}
}

// The connection a game holds is pinned and modern.
func TestTheConnectionIsPinnedAndModern(t *testing.T) {
	r := newBridgeRig(t)
	creds := r.register(t, "poker")

	conn, err := tls.Dial("tcp", r.Addr, clientTLS(t, creds))
	if err != nil {
		t.Fatalf("a credential this bridge issued was refused: %v", err)
	}
	defer conn.Close()

	if v := conn.ConnectionState().Version; v < tls.VersionTLS12 {
		t.Errorf("the connection settled on TLS version %x, below 1.2", v)
	}
	if len(conn.ConnectionState().PeerCertificates) == 0 {
		t.Error("the bridge presented no certificate of its own, so nothing could pin it")
	}
}
