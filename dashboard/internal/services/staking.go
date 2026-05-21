// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrd/chaincfg/chainhash"
	pb "decred.org/dcrwallet/v4/rpc/walletrpc"
)

const (
	vspRegistryURL    = "https://api.decred.org/?c=vsp"
	vspInfoPathV3     = "/api/v3/vspinfo"
	vspCacheTTL       = 5 * time.Minute
	vspProbeTimeout   = 5 * time.Second
	vspRegistryTimout = 8 * time.Second
)

var (
	vspCacheMu   sync.RWMutex
	vspCache     []types.VSPInfo
	vspCacheTime time.Time
)

// ListVSPs returns the public registry of VSPs. Result cached in-process
// for vspCacheTTL to avoid hammering api.decred.org on every page mount.
func ListVSPs(ctx context.Context) ([]types.VSPInfo, error) {
	vspCacheMu.RLock()
	if time.Since(vspCacheTime) < vspCacheTTL && vspCache != nil {
		cached := vspCache
		vspCacheMu.RUnlock()
		return cached, nil
	}
	vspCacheMu.RUnlock()

	fetched, err := fetchVSPRegistry(ctx)
	if err != nil {
		return nil, err
	}

	vspCacheMu.Lock()
	vspCache = fetched
	vspCacheTime = time.Now()
	vspCacheMu.Unlock()
	return fetched, nil
}

func fetchVSPRegistry(ctx context.Context) ([]types.VSPInfo, error) {
	rctx, cancel := context.WithTimeout(ctx, vspRegistryTimout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, vspRegistryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("vsp registry request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vsp registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vsp registry: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read vsp registry body: %w", err)
	}
	// Response shape: { "host1": { "feepercentage": ..., ... }, "host2": {...} }
	var raw map[string]struct {
		PubKey            string  `json:"pubkey"`
		Network           string  `json:"network"`
		APIVersions       []int   `json:"apiversions"`
		FeePercentage     float64 `json:"feepercentage"`
		VspdVersion       string  `json:"vspdversion"`
		BlockHeight       uint32  `json:"blockheight"`
		NetworkProportion float64 `json:"estimatednetworkproportion"`
		Voting            uint32  `json:"voting"`
		Voted             uint32  `json:"voted"`
		Expired           uint32  `json:"expired"`
		Missed            uint32  `json:"missed"`
		Closed            bool    `json:"closed"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode vsp registry: %w", err)
	}
	out := make([]types.VSPInfo, 0, len(raw))
	for host, v := range raw {
		if v.Closed {
			continue
		}
		out = append(out, types.VSPInfo{
			Host:              host,
			PubKey:            v.PubKey,
			Network:           v.Network,
			APIVersions:       v.APIVersions,
			FeePercentage:     v.FeePercentage,
			VspdVersion:       v.VspdVersion,
			BlockHeight:       v.BlockHeight,
			NetworkProportion: v.NetworkProportion,
			Voting:            v.Voting,
			Voted:             v.Voted,
			Expired:           v.Expired,
			Missed:            v.Missed,
		})
	}
	return out, nil
}

// GetVSPInfo probes a single VSP host's /api/v3/vspinfo for its pubkey + fee.
func GetVSPInfo(ctx context.Context, host string) (*types.VSPInfo, error) {
	host = strings.TrimRight(host, "/")
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	rctx, cancel := context.WithTimeout(ctx, vspProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, host+vspInfoPathV3, nil)
	if err != nil {
		return nil, fmt.Errorf("vsp info request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vsp info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vsp info: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	var raw struct {
		PubKey        string  `json:"pubkey"`
		Network       string  `json:"network"`
		APIVersions   []int   `json:"apiversions"`
		FeePercentage float64 `json:"feepercentage"`
		VspdVersion   string  `json:"vspdversion"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode vsp info: %w", err)
	}
	return &types.VSPInfo{
		Host:          strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://"),
		PubKey:        raw.PubKey,
		Network:       raw.Network,
		APIVersions:   raw.APIVersions,
		FeePercentage: raw.FeePercentage,
		VspdVersion:   raw.VspdVersion,
	}, nil
}

// PurchaseTickets calls dcrwallet's PurchaseTickets gRPC with the modern VSP
// fields. Uses lazy per-account-encryption migration on the source account.
func PurchaseTickets(ctx context.Context, account, numTickets uint32, vspHost, vspPubkey string, changeAccount uint32, passphrase []byte) (*types.PurchaseTicketsResponse, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC client not initialized")
	}
	if numTickets == 0 {
		return nil, fmt.Errorf("numTickets must be > 0")
	}
	if vspHost == "" || vspPubkey == "" {
		return nil, fmt.Errorf("vspHost and vspPubkey are required")
	}

	// Unlock the source account; migrate to per-account encryption on the fly
	// if it isn't yet (matches SignAndPublishTransaction's pattern).
	if _, err := rpc.WalletGrpcClient.UnlockAccount(ctx, &pb.UnlockAccountRequest{
		Passphrase:    passphrase,
		AccountNumber: account,
	}); err != nil {
		if strings.Contains(err.Error(), "account is not encrypted with a unique passphrase") {
			if mErr := ensureAccountEncrypted(ctx, account, passphrase); mErr != nil {
				return nil, fmt.Errorf("migrate account to per-account encryption: %w", mErr)
			}
			if _, err := rpc.WalletGrpcClient.UnlockAccount(ctx, &pb.UnlockAccountRequest{
				Passphrase:    passphrase,
				AccountNumber: account,
			}); err != nil {
				return nil, fmt.Errorf("unlock source account: %w", err)
			}
		} else {
			return nil, fmt.Errorf("unlock source account: %w", err)
		}
	}
	defer func() {
		relockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = rpc.WalletGrpcClient.LockAccount(relockCtx, &pb.LockAccountRequest{AccountNumber: account})
	}()

	resp, err := rpc.WalletGrpcClient.PurchaseTickets(ctx, &pb.PurchaseTicketsRequest{
		Passphrase:    passphrase,
		Account:       account,
		NumTickets:    numTickets,
		VspHost:       "https://" + strings.TrimPrefix(strings.TrimPrefix(vspHost, "https://"), "http://"),
		VspPubkey:     vspPubkey,
		ChangeAccount: changeAccount,
	})
	if err != nil {
		return nil, fmt.Errorf("PurchaseTickets RPC: %w", err)
	}

	out := &types.PurchaseTicketsResponse{
		TicketHashes: make([]string, 0, len(resp.TicketHashes)),
	}
	for _, h := range resp.TicketHashes {
		if hh, herr := chainhash.NewHash(h); herr == nil {
			out.TicketHashes = append(out.TicketHashes, hh.String())
		} else {
			out.TicketHashes = append(out.TicketHashes, hex.EncodeToString(h))
		}
	}
	if len(resp.SplitTx) > 0 {
		out.SplitTxHash = hex.EncodeToString(resp.SplitTx)
	}
	return out, nil
}
