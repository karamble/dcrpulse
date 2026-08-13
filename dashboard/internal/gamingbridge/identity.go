// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package gamingbridge serves the port standalone games connect in on.
//
// It is deliberately not the browser API. That one is guarded by same-origin
// and a dashboard session, neither of which a separate program has, and
// same-origin defends against a browser being tricked into spending cookies it
// already holds - which has no bearing on a caller presenting a certificate.
// Two audiences, two listeners, two ways of proving who you are.
package gamingbridge

import (
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/decred/dcrd/certgen"
)

// credentialLifetime is how long an issued credential is good for.
//
// Long on purpose. Revocation here is the allowlist, not expiry: an operator
// who can no longer reach a machine still needs the certificate on it to stop
// working the moment they say so, and that is a map delete rather than a wait.
// An expiry short enough to be a control would only be an outage nobody chose.
const credentialLifetime = 10 * 365 * 24 * time.Hour

// ErrGameNotRegistered is a credential that resolves to nobody.
var ErrGameNotRegistered = errors.New("no game is registered under that credential")

// Credential is what an operator carries to a game by hand.
//
// The private key is in it because the bridge mints the pair, and it is the one
// part that must be shown once and never stored: it is handed over through a
// person, which is what makes it impossible for anything on either machine to
// fetch a credential it was not given.
type Credential struct {
	Game        string
	CertPEM     []byte
	KeyPEM      []byte
	Fingerprint string
}

// GenerateGameCredential mints the certificate a game authenticates with.
//
// The certificate IS the identity: everything the bridge enforces hangs off
// which game a connection resolves to, so a game never states who it is - it
// presents this and the bridge decides.
//
// Built with certgen, which is the stack's own helper and names itself for
// server certificates. It works here because it sets no extended key usage at
// all, and x509 skips that check entirely for a certificate that declares none,
// so the result is equally valid presented by a client. Do not "fix" that by
// adding an ExtKeyUsage: server-only would make every credential useless.
func GenerateGameCredential(game string) (Credential, error) {
	// certgen reads the machine's hostname and enumerates its interfaces to
	// fill in subject alternative names, and treats a failure at either as
	// fatal. None of that matters to a client certificate - nothing checks a
	// client's names - but it means minting can fail for reasons that have
	// nothing to do with this game.
	certPEM, keyPEM, err := certgen.NewTLSCertPair(
		elliptic.P256(), "dcrpulse gaming bridge", time.Now().Add(credentialLifetime), nil)
	if err != nil {
		return Credential{}, fmt.Errorf("mint a credential for %q: %w", game, err)
	}
	fp, err := fingerprintPEM(certPEM)
	if err != nil {
		return Credential{}, err
	}
	return Credential{Game: game, CertPEM: certPEM, KeyPEM: keyPEM, Fingerprint: fp}, nil
}

// fingerprintPEM is the identity a certificate resolves by.
//
// Taken over the certificate's own bytes rather than over any field inside it.
// A subject can say anything - certgen writes the machine's hostname into the
// common name, so every credential this bridge issues claims the same one - and
// a name a caller supplies is not an identity.
func fingerprintPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("that is not a certificate")
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:]), nil
}

// fingerprintOf is the same answer for a parsed certificate.
func fingerprintOf(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// Allowlist is which credentials may connect, and what each one is.
//
// Membership is the whole of the trust decision. There is no authority above
// it: a credential is its own root, so revoking is removing an entry rather
// than publishing a revocation nobody fetches.
type Allowlist struct {
	mu sync.RWMutex
	// byFingerprint maps a certificate's fingerprint to the game it is.
	byFingerprint map[string]allowEntry
}

type allowEntry struct {
	game string
	// cert is kept as the parsed certificate, not re-encoded, because the
	// pool below matches on exact bytes.
	cert *x509.Certificate
}

func NewAllowlist() *Allowlist {
	return &Allowlist{byFingerprint: make(map[string]allowEntry)}
}

// Add admits a credential. Adding a game that already has one replaces it, so
// regenerating retires the old credential in the same act rather than leaving
// two ways in.
func (a *Allowlist) Add(c Credential) error {
	block, _ := pem.Decode(c.CertPEM)
	if block == nil {
		return errors.New("that is not a certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse the credential for %q: %w", c.Game, err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for fp, e := range a.byFingerprint {
		if e.game == c.Game {
			delete(a.byFingerprint, fp)
		}
	}
	a.byFingerprint[fingerprintOf(cert)] = allowEntry{game: c.Game, cert: cert}
	return nil
}

// Resolve names the game a fingerprint belongs to.
func (a *Allowlist) Resolve(fingerprint string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	e, ok := a.byFingerprint[fingerprint]
	return e.game, ok
}

// Revoke withdraws a game's credential.
//
// It is removal, not concealment: the certificate stops being anything at all,
// so a game still holding it finds that it no longer resolves. Re-registering
// later mints a fresh one, because a credential nobody restated is one nobody
// reviewed.
func (a *Allowlist) Revoke(game string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for fp, e := range a.byFingerprint {
		if e.game == game {
			delete(a.byFingerprint, fp)
		}
	}
}

// pool builds the roots a handshake verifies against, from whatever is admitted
// right now.
func (a *Allowlist) pool() *x509.CertPool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	p := x509.NewCertPool()
	for _, e := range a.byFingerprint {
		p.AddCert(e.cert)
	}
	return p
}

// serverTLSConfig is what the listener answers with.
//
// Every credential is its own root, so the allowlist is handed over as the pool
// a client certificate is verified against. That is why the roots are rebuilt
// per handshake rather than fixed when the listener starts: revoking has to
// take effect on the next connection, and a pool captured at startup would go
// on admitting a credential the operator has withdrawn until something
// restarted.
//
// RequireAndVerifyClientCert rather than accepting any certificate and checking
// it afterwards, because this way the standard verifier does the work - and it
// enforces the validity window, which a hand-rolled check would have to
// remember to do.
func serverTLSConfig(cert tls.Certificate, allow *Allowlist) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    allow.pool(),
			}, nil
		},
	}
}
