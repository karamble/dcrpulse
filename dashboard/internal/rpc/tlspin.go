// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"crypto/x509"
	"fmt"
)

// pinnedLeafVerifier returns a tls.Config.VerifyPeerCertificate callback that
// authenticates the peer against a pinned certificate pool without checking the
// certificate hostname/SANs. dcrlnd and brclientd ship self-signed certs whose
// SANs (localhost/127.0.0.1/container hostname) do not match the service name
// the dashboard dials, so Go's default verification cannot be used. Pinning the
// exact cert in the pool is the trust root. This runs even when the tls.Config
// sets InsecureSkipVerify, which only disables Go's built-in chain+hostname
// check; the callback below is what actually authenticates the connection.
func pinnedLeafVerifier(pool *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("peer presented no certificate")
		}
		leaf, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("parse peer certificate: %w", err)
		}
		opts := x509.VerifyOptions{Roots: pool}
		for _, der := range rawCerts[1:] {
			ic, err := x509.ParseCertificate(der)
			if err != nil {
				return fmt.Errorf("parse peer intermediate certificate: %w", err)
			}
			if opts.Intermediates == nil {
				opts.Intermediates = x509.NewCertPool()
			}
			opts.Intermediates.AddCert(ic)
		}
		if _, err := leaf.Verify(opts); err != nil {
			return fmt.Errorf("peer certificate not trusted by pinned pool: %w", err)
		}
		return nil
	}
}
