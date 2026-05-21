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
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrd/chaincfg/chainhash"
	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

const (
	vspRegistryURL    = "https://api.decred.org/?c=vsp"
	vspInfoPathV3     = "/api/v3/vspinfo"
	vspCacheTTL       = 24 * time.Hour
	vspProbeTimeout   = 5 * time.Second
	vspRegistryTimout = 8 * time.Second
)

var (
	vspCacheMu   sync.RWMutex
	vspCache     []types.VSPInfo
	vspCacheTime time.Time
)

// VSPListingEnabled reports whether the user has the global VSP-registry
// toggle on. Absent or true defaults to enabled (backward compatible).
func VSPListingEnabled() bool {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return true
	}
	allowed, _ := gc.AllowedExternalRequests()
	if allowed == nil {
		return true
	}
	v, ok := allowed[config.ExternalRequestVSPListing]
	if !ok {
		return true
	}
	return v
}

// ListVSPs returns the public registry of VSPs. Honors the global
// stakepool_listing toggle - when disabled, returns (nil, nil) without
// any outbound HTTP. Otherwise fetches and caches the slice for
// vspCacheTTL to avoid hammering api.decred.org.
func ListVSPs(ctx context.Context) ([]types.VSPInfo, error) {
	if !VSPListingEnabled() {
		return nil, nil
	}

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

// GetUsedVSPs returns the per-wallet used_vsps history sorted by
// LastUsed descending. Returns (nil, nil) if no entries exist.
func GetUsedVSPs(ctx context.Context) ([]types.VSPInfo, error) {
	network, err := CurrentNetwork(ctx)
	if err != nil || network == "" {
		return nil, nil
	}
	wc, err := config.LoadWalletCfg(network, CurrentWalletName())
	if err != nil {
		return nil, err
	}
	m, err := wc.UsedVSPs()
	if err != nil || len(m) == 0 {
		return nil, err
	}
	out := make([]types.VSPInfo, 0, len(m))
	for _, v := range m {
		out = append(out, types.VSPInfo{
			Host:   v.Host,
			PubKey: v.Pubkey,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return m[out[i].Host].LastUsed > m[out[j].Host].LastUsed
	})
	return out, nil
}

// rememberVSPUsed upserts a VSP into the per-wallet used_vsps map.
// Idempotent. Errors are logged and swallowed so the calling action
// (ticket purchase, autobuyer start, etc.) is not blocked by a
// best-effort persistence write.
func rememberVSPUsed(ctx context.Context, host, pubkey string) {
	if host == "" {
		return
	}
	network, err := CurrentNetwork(ctx)
	if err != nil || network == "" {
		log.Printf("rememberVSPUsed: skipping, network not resolved: %v", err)
		return
	}
	wc, err := config.LoadWalletCfg(network, CurrentWalletName())
	if err != nil {
		log.Printf("rememberVSPUsed: load wallet cfg: %v", err)
		return
	}
	if err := wc.UpsertUsedVSP(config.VSPMetadata{
		Host:     host,
		Pubkey:   pubkey,
		LastUsed: time.Now().Unix(),
	}); err != nil {
		log.Printf("rememberVSPUsed: upsert: %v", err)
		return
	}
	if err := wc.Save(); err != nil {
		log.Printf("rememberVSPUsed: save: %v", err)
	}
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

	// Successful purchase - remember the VSP we used for next time the
	// picker is opened, even when the registry toggle is off. Mirrors
	// Decrediton's dispatch(updateUsedVSPs(vsp)) in ControlActions.js:379.
	rememberVSPUsed(ctx, vspHost, vspPubkey)

	return out, nil
}

// ListTickets streams every wallet ticket and joins each with its VSP fee
// state. Errors from the VSP-fee-status calls are non-fatal: the records are
// still returned, just without a FeeStatus value.
func ListTickets(ctx context.Context) ([]types.TicketRecord, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC client not initialized")
	}

	stream, err := rpc.WalletGrpcClient.GetTickets(ctx, &pb.GetTicketsRequest{})
	if err != nil {
		return nil, fmt.Errorf("GetTickets RPC: %w", err)
	}

	records := make([]types.TicketRecord, 0, 32)
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("GetTickets stream: %w", err)
		}
		records = append(records, ticketRecordFromResponse(resp))
	}

	feeStatusByHash := fetchFeeStatusMap(ctx)
	for i := range records {
		if fs, ok := feeStatusByHash[records[i].Hash]; ok {
			records[i].FeeStatus = fs
		}
	}
	return records, nil
}

// ticketStatusNames maps the dcrwallet enum to the canonical short names we
// surface to the frontend.
var ticketStatusNames = map[pb.GetTicketsResponse_TicketDetails_TicketStatus]string{
	pb.GetTicketsResponse_TicketDetails_UNKNOWN:  "UNKNOWN",
	pb.GetTicketsResponse_TicketDetails_UNMINED:  "UNMINED",
	pb.GetTicketsResponse_TicketDetails_IMMATURE: "IMMATURE",
	pb.GetTicketsResponse_TicketDetails_LIVE:     "LIVE",
	pb.GetTicketsResponse_TicketDetails_VOTED:    "VOTED",
	pb.GetTicketsResponse_TicketDetails_MISSED:   "MISSED",
	pb.GetTicketsResponse_TicketDetails_EXPIRED:  "EXPIRED",
	pb.GetTicketsResponse_TicketDetails_REVOKED:  "REVOKED",
}

func ticketRecordFromResponse(r *pb.GetTicketsResponse) types.TicketRecord {
	out := types.TicketRecord{
		VSPHost: r.GetVspHost(),
	}
	if b := r.GetBlock(); b != nil {
		out.BlockHeight = b.GetHeight()
		out.BlockTime = b.GetTimestamp()
	}
	td := r.GetTicket()
	if td == nil {
		return out
	}
	out.Status = ticketStatusNames[td.GetTicketStatus()]
	if t := td.GetTicket(); t != nil {
		if h, herr := chainhash.NewHash(t.GetHash()); herr == nil {
			out.Hash = h.String()
		} else {
			out.Hash = hex.EncodeToString(t.GetHash())
		}
		var debitSum, creditSum int64
		for _, in := range t.GetDebits() {
			debitSum += in.GetPreviousAmount()
		}
		for _, c := range t.GetCredits() {
			creditSum += c.GetAmount()
		}
		// ticket commit = funds spent into the stake submission output.
		commit := debitSum - creditSum - t.GetFee()
		if commit < 0 {
			commit = 0
		}
		out.TicketPrice = float64(commit) / 1e8
	}
	if s := td.GetSpender(); s != nil {
		if h, herr := chainhash.NewHash(s.GetHash()); herr == nil {
			out.SpenderHash = h.String()
		} else {
			out.SpenderHash = hex.EncodeToString(s.GetHash())
		}
		out.SpenderTime = s.GetTimestamp()
		if td.GetTicketStatus() == pb.GetTicketsResponse_TicketDetails_VOTED {
			var spenderCredit int64
			for _, c := range s.GetCredits() {
				spenderCredit += c.GetAmount()
			}
			reward := float64(spenderCredit)/1e8 - out.TicketPrice
			if reward < 0 {
				reward = 0
			}
			out.Reward = reward
		}
	}
	return out
}

// fetchFeeStatusMap returns ticket-hash -> short fee-status name.
func fetchFeeStatusMap(ctx context.Context) map[string]string {
	out := make(map[string]string)
	feeStatuses := []struct {
		enum pb.GetVSPTicketsByFeeStatusRequest_FeeStatus
		name string
	}{
		{pb.GetVSPTicketsByFeeStatusRequest_VSP_FEE_PROCESS_STARTED, "UNPAID"},
		{pb.GetVSPTicketsByFeeStatusRequest_VSP_FEE_PROCESS_PAID, "PAID"},
		{pb.GetVSPTicketsByFeeStatusRequest_VSP_FEE_PROCESS_ERRORED, "ERRORED"},
		{pb.GetVSPTicketsByFeeStatusRequest_VSP_FEE_PROCESS_CONFIRMED, "CONFIRMED"},
	}
	for _, fs := range feeStatuses {
		resp, err := rpc.WalletGrpcClient.GetVSPTicketsByFeeStatus(ctx, &pb.GetVSPTicketsByFeeStatusRequest{
			FeeStatus: fs.enum,
		})
		if err != nil {
			log.Printf("GetVSPTicketsByFeeStatus(%s): %v", fs.name, err)
			continue
		}
		for _, raw := range resp.GetTicketsHashes() {
			if h, herr := chainhash.NewHash(raw); herr == nil {
				out[h.String()] = fs.name
			} else {
				out[hex.EncodeToString(raw)] = fs.name
			}
		}
	}
	return out
}

// SyncFailedVSPTickets retries fee payment for tickets with VSP fee errors
// against the given VSP. Uses the lazy per-account-encryption migration
// pattern from PurchaseTickets.
func SyncFailedVSPTickets(ctx context.Context, vspHost, vspPubkey string, account, changeAccount uint32, passphrase []byte) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC client not initialized")
	}
	if vspHost == "" || vspPubkey == "" {
		return fmt.Errorf("vspHost and vspPubkey are required")
	}

	if _, err := rpc.WalletGrpcClient.UnlockAccount(ctx, &pb.UnlockAccountRequest{
		Passphrase:    passphrase,
		AccountNumber: account,
	}); err != nil {
		if strings.Contains(err.Error(), "account is not encrypted with a unique passphrase") {
			if mErr := ensureAccountEncrypted(ctx, account, passphrase); mErr != nil {
				return fmt.Errorf("migrate account to per-account encryption: %w", mErr)
			}
			if _, err := rpc.WalletGrpcClient.UnlockAccount(ctx, &pb.UnlockAccountRequest{
				Passphrase:    passphrase,
				AccountNumber: account,
			}); err != nil {
				return fmt.Errorf("unlock account: %w", err)
			}
		} else {
			return fmt.Errorf("unlock account: %w", err)
		}
	}
	defer func() {
		relockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = rpc.WalletGrpcClient.LockAccount(relockCtx, &pb.LockAccountRequest{AccountNumber: account})
	}()

	if _, err := rpc.WalletGrpcClient.SyncVSPFailedTickets(ctx, &pb.SyncVSPTicketsRequest{
		VspHost:       "https://" + strings.TrimPrefix(strings.TrimPrefix(vspHost, "https://"), "http://"),
		VspPubkey:     vspPubkey,
		Account:       account,
		ChangeAccount: changeAccount,
	}); err != nil {
		return fmt.Errorf("SyncVSPFailedTickets RPC: %w", err)
	}
	rememberVSPUsed(ctx, vspHost, vspPubkey)
	return nil
}
