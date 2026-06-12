// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/decred/go-socks/socks"
)

// externalTransport routes the dashboard's own outbound internet requests.
// Each request re-reads the Tor toggle, so flipping it applies immediately
// without a restart. The clearnet and Tor paths keep separate connection
// pools so an idle connection from one path is never reused on the other.
type externalTransport struct {
	clear *http.Transport
	tor   *http.Transport
}

var (
	extTransportOnce sync.Once
	extTransport     *externalTransport
)

// ExternalTransport returns the shared RoundTripper for dashboard-origin
// external HTTP calls (rate oracle, Politeia, VSP, BR seeder, invite bot).
func ExternalTransport() http.RoundTripper {
	extTransportOnce.Do(func() {
		extTransport = &externalTransport{
			clear: http.DefaultTransport.(*http.Transport).Clone(),
			tor: &http.Transport{
				DialContext: dialTorSOCKS,
			},
		}
	})
	return extTransport
}

// externalHTTPClient replaces http.DefaultClient at the external call sites;
// per-request timeouts keep coming from the request contexts.
var externalHTTPClient = &http.Client{Transport: ExternalTransport()}

func (t *externalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if ReadTorSettings().Enabled {
		return t.tor.RoundTrip(req)
	}
	return t.clear.RoundTrip(req)
}

// dialTorSOCKS dials through the Tor SOCKS proxy, passing the hostname to the
// proxy so name resolution happens inside Tor. There is no clearnet fallback:
// with the toggle on and the proxy down, the request fails.
func dialTorSOCKS(ctx context.Context, network, addr string) (net.Conn, error) {
	endpoint, ok := torProxyEndpoint()
	if !ok {
		return nil, errors.New("tor routing enabled but no proxy is configured")
	}
	p := &socks.Proxy{Addr: endpoint, TorIsolation: ReadTorSettings().Isolation}
	return p.DialContext(ctx, network, addr)
}
