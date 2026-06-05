// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrlnd/lnrpc"
	lpclient "github.com/decred/dcrlnlpd/client"
)

// liquidityDefault is the built-in liquidity provider per network, mirroring
// bisonrelay's client_onboard.go onboardOpenInboundChan defaults.
type liquidityDefault struct {
	server  string
	certPEM string
}

const hub0MainnetCertPEM = `-----BEGIN CERTIFICATE-----
MIIBwzCCAWigAwIBAgIQJNKWfgRSQnnMdBwKsVshhTAKBggqhkjOPQQDAjAxMREw
DwYDVQQKEwhkY3JsbmxwZDEcMBoGA1UEAxMTaHViMC5iaXNvbnJlbGF5Lm9yZzAe
Fw0yNDA5MTIxNTMyNTVaFw0zNDA5MTExNTMyNTVaMDExETAPBgNVBAoTCGRjcmxu
bHBkMRwwGgYDVQQDExNodWIwLmJpc29ucmVsYXkub3JnMFkwEwYHKoZIzj0CAQYI
KoZIzj0DAQcDQgAE8BvBcDlzJs+DLRHa08bLVx1ya9S+PX+b7obfhq45VdkenSNt
xk9OJZUGnpTkDbt1CBLjQg6RRqYkADYviCuDfaNiMGAwDgYDVR0PAQH/BAQDAgKE
MA8GA1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFBkc97rEXLNm3S/166Q7OqOoBuwd
MB4GA1UdEQQXMBWCE2h1YjAuYmlzb25yZWxheS5vcmcwCgYIKoZIzj0EAwIDSQAw
RgIhAKW0WpOpb0HyXofI1ML0Yu29NqU+WNwyOVzD9IlOluerAiEA84ltFlil8D1i
L6izsBzTqk6GKYSfl095BKOGyIrT+1c=
-----END CERTIFICATE-----`

var liquidityDefaults = map[string]liquidityDefault{
	"mainnet": {server: "https://hub0.bisonrelay.org:9130", certPEM: hub0MainnetCertPEM},
	"simnet":  {server: "https://127.0.0.1:29130"},
}

// liquidityNetwork resolves the active network from dcrlnd itself so the
// default LP always matches the node the payment will flow through.
func liquidityNetwork(ctx context.Context) (string, error) {
	if rpc.LightningClient == nil {
		return "", fmt.Errorf("dcrlnd not available")
	}
	info, err := rpc.LightningClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return "", fmt.Errorf("GetInfo: %w", err)
	}
	if len(info.Chains) == 0 || info.Chains[0].Chain != "decred" {
		return "", fmt.Errorf("not connected to a decred lightning network")
	}
	return info.Chains[0].Network, nil
}

// GetLiquidityDefaults returns the built-in LP server + cert for the active
// network so the request wizard can pre-fill them. Server is empty when the
// network has no default (e.g. testnet).
func GetLiquidityDefaults(ctx context.Context) (*types.LiquidityDefaults, error) {
	network, err := liquidityNetwork(ctx)
	if err != nil {
		return nil, err
	}
	d := liquidityDefaults[network]
	return &types.LiquidityDefaults{
		Network: network,
		Server:  d.server,
		CertPEM: d.certPEM,
	}, nil
}

// resolveServerCert applies the network defaults for fields the caller left
// blank, mirroring how bruig pre-fills from its built-in table.
func resolveServerCert(ctx context.Context, server, certPEM string) (string, []byte, error) {
	server = strings.TrimSpace(server)
	certPEM = strings.TrimSpace(certPEM)
	if server == "" || certPEM == "" {
		network, err := liquidityNetwork(ctx)
		if err != nil {
			return "", nil, err
		}
		d := liquidityDefaults[network]
		if server == "" {
			server = d.server
		}
		if certPEM == "" {
			certPEM = d.certPEM
		}
	}
	if server == "" {
		return "", nil, fmt.Errorf("no liquidity provider server configured for this network")
	}
	var certBytes []byte
	if certPEM != "" {
		certBytes = []byte(certPEM)
	}
	return server, certBytes, nil
}

// errEstimateAbort is returned from the PolicyFetched callback during the
// estimate phase to abort RequestChannel before any side effects; the
// dcrlnlpd client only connects to the LP peer and requests the invoice
// after PolicyFetched returns nil.
var errEstimateAbort = errors.New("liquidity estimate complete")

// EstimateLiquidityChannel fetches the LP policy for the requested channel
// size and returns the estimated fee plus the policy limits without paying
// anything. The dcrlnlpd client validates size bounds, the per-node channel
// limit, outbound capacity and the payment route BEFORE PolicyFetched, so
// those failures surface here as the raw library errors.
func EstimateLiquidityChannel(ctx context.Context, req *types.RequestLiquidityEstimateRequest) (*types.LiquidityEstimateResponse, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	server, certBytes, err := resolveServerCert(ctx, req.Server, req.CertPEM)
	if err != nil {
		return nil, err
	}
	var out types.LiquidityEstimateResponse
	c, err := lpclient.New(lpclient.Config{
		LC:           rpc.LightningClient,
		Address:      server,
		Certificates: certBytes,
		PolicyFetched: func(p lpclient.ServerPolicy) error {
			out = types.LiquidityEstimateResponse{
				ChanSizeAtoms:          req.ChanSizeAtoms,
				EstimatedFeeAtoms:      int64(lpclient.EstimatedInvoiceAmount(uint64(req.ChanSizeAtoms), p.ChanInvoiceFeeRate)),
				MinChanSizeAtoms:       int64(p.MinChanSize),
				MaxChanSizeAtoms:       int64(p.MaxChanSize),
				MaxNbChannels:          uint32(p.MaxNbChannels),
				MinChanLifetimeSeconds: int64(p.MinChanLifetime / time.Second),
				Node:                   p.Node.String(),
				Addresses:              p.NodeAddresses,
			}
			return errEstimateAbort
		},
	})
	if err != nil {
		return nil, err
	}
	err = c.RequestChannel(ctx, uint64(req.ChanSizeAtoms))
	if errors.Is(err, errEstimateAbort) {
		return &out, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("liquidity estimate did not abort as expected")
}

// RequestLiquidityChannel pays the LP and returns once its channel to us is
// seen pending. dcrlnlpd's RequestChannel only returns when the channel is
// fully OPEN (several confirmations), so it runs detached on a background
// context and the HTTP caller is answered at the PendingChannel callback;
// cancelling the watcher at that point would not stop the LP-driven open
// anyway, but letting it run keeps the event stream observed.
func RequestLiquidityChannel(ctx context.Context, req *types.RequestLiquidityRequest) (*types.RequestLiquidityResponse, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	server, certBytes, err := resolveServerCert(ctx, req.Server, req.CertPEM)
	if err != nil {
		return nil, err
	}
	type pendingChan struct {
		chanPoint string
		capacity  uint64
	}
	pendingCh := make(chan pendingChan, 1)
	doneCh := make(chan error, 1)
	c, err := lpclient.New(lpclient.Config{
		LC:           rpc.LightningClient,
		Address:      server,
		Certificates: certBytes,
		PolicyFetched: func(p lpclient.ServerPolicy) error {
			// Guard against the LP changing its fee rate between the
			// estimate the user approved and this request.
			fee := int64(lpclient.EstimatedInvoiceAmount(uint64(req.ChanSizeAtoms), p.ChanInvoiceFeeRate))
			if req.ApprovedFeeAtoms > 0 && fee > req.ApprovedFeeAtoms {
				return fmt.Errorf("liquidity provider now quotes a fee of %d atoms, above the approved %d atoms; request a new estimate", fee, req.ApprovedFeeAtoms)
			}
			return nil
		},
		PendingChannel: func(chanID string, capacity uint64) {
			select {
			case pendingCh <- pendingChan{chanPoint: chanID, capacity: capacity}:
			default:
			}
		},
	})
	if err != nil {
		return nil, err
	}
	// Bounded so an LP that never broadcasts cannot leak the channel-events
	// stream forever; normal opens complete well within this.
	runCtx, runCancel := context.WithTimeout(context.Background(), 2*time.Hour)
	go func() {
		defer runCancel()
		doneCh <- c.RequestChannel(runCtx, uint64(req.ChanSizeAtoms))
	}()
	select {
	case p := <-pendingCh:
		return &types.RequestLiquidityResponse{
			ChannelPoint:  p.chanPoint,
			CapacityAtoms: int64(p.capacity),
		}, nil
	case err := <-doneCh:
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("channel opened without an observed pending event")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
