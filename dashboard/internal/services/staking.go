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
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
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
			Host:          v.Host,
			PubKey:        v.Pubkey,
			FeePercentage: v.FeePercentage,
			VspdVersion:   v.VspdVersion,
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
	meta := config.VSPMetadata{
		Host:     host,
		Pubkey:   pubkey,
		LastUsed: time.Now().Unix(),
	}
	// Best-effort enrich with the live fee% + vspd version so the stored entry
	// matches Decrediton's used_vsps (host + pubkey + fee + version). GetVSPInfo
	// applies its own probe timeout; a failure just leaves them unset.
	if info, perr := GetVSPInfo(ctx, host); perr == nil {
		meta.FeePercentage = info.FeePercentage
		meta.VspdVersion = info.VspdVersion
		if meta.Pubkey == "" {
			meta.Pubkey = info.PubKey
		}
	}
	if err := wc.UpsertUsedVSP(meta); err != nil {
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
	resp, err := externalHTTPClient.Do(req)
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

// disallowedProbeIP reports whether ip is in a range an outbound probe must not
// reach, so a user-supplied VSP host cannot be used to reach the local host, the
// docker network, or a cloud metadata service (SSRF).
func disallowedProbeIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

// vspHTTPClient probes VSP hosts. It refuses redirects and refuses to connect to
// any non-public address, dialing the validated IP directly to avoid a
// DNS-rebinding window between the check and the connect.
var vspHTTPClient = &http.Client{
	Timeout: vspProbeTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return fmt.Errorf("redirects are not allowed")
	},
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		// The dial path below depends on the live Tor toggle; without
		// keep-alives no pooled connection can outlive a flip.
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			// Over Tor the hostname goes to the proxy unresolved: a local
			// lookup would leak DNS, and the connection originates at the
			// exit, away from any address the rebind guard protects. A
			// literal non-public IP is still refused.
			if ReadTorSettings().Enabled {
				if ip := net.ParseIP(host); ip != nil && disallowedProbeIP(ip) {
					return nil, fmt.Errorf("refusing to connect to non-public address %s", ip)
				}
				return dialTorSOCKS(ctx, network, addr)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if disallowedProbeIP(ip.IP) {
					return nil, fmt.Errorf("refusing to connect to non-public address %s", ip.IP)
				}
			}
			d := &net.Dialer{Timeout: 15 * time.Second}
			return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	},
}

// GetVSPInfo probes a single VSP host's /api/v3/vspinfo for its pubkey + fee.
func GetVSPInfo(ctx context.Context, host string) (*types.VSPInfo, error) {
	// VSP communication is https only; force the scheme even if a http:// host
	// was supplied.
	host = strings.TrimRight(host, "/")
	host = "https://" + strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	rctx, cancel := context.WithTimeout(ctx, vspProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, host+vspInfoPathV3, nil)
	if err != nil {
		return nil, fmt.Errorf("vsp info request: %w", err)
	}
	resp, err := vspHTTPClient.Do(req)
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

var (
	ticketPurchaseMu     sync.Mutex
	ticketPurchaseActive bool
)

// IsTicketPurchaseInProgress reports whether a manual ticket purchase is
// currently running. A purchase pauses and then restarts the mixer itself, so
// the mixer must not be started mid-purchase.
func IsTicketPurchaseInProgress() bool {
	ticketPurchaseMu.Lock()
	defer ticketPurchaseMu.Unlock()
	return ticketPurchaseActive
}

// tryBeginTicketPurchase marks a purchase active and returns true, or returns
// false if one is already running. Pair a true return with endTicketPurchase.
// This is the single-flight guard shared by the synchronous (plain) path and
// the background worker (privacy) path so neither can race the other, the
// mixer, or the autobuyer.
func tryBeginTicketPurchase() bool {
	ticketPurchaseMu.Lock()
	defer ticketPurchaseMu.Unlock()
	if ticketPurchaseActive {
		return false
	}
	ticketPurchaseActive = true
	return true
}

func endTicketPurchase() {
	ticketPurchaseMu.Lock()
	ticketPurchaseActive = false
	ticketPurchaseMu.Unlock()
}

const purchaseEventBufferSize = 200

var (
	purchaseEventsMu sync.Mutex
	purchaseEvents   []types.PurchaseEvent

	purchaseSubsMu      sync.Mutex
	purchaseSubscribers []chan types.PurchaseEvent

	purchaseResultMu    sync.Mutex
	purchaseLastErr     string
	purchaseLastHashes  []string
	purchaseLastSplitTx string
)

// setPurchaseResult records the most recent terminal outcome of the background
// purchase worker so a reloaded page can read it via PurchaseStatusSnapshot.
func setPurchaseResult(hashes []string, splitTx, errMsg string) {
	purchaseResultMu.Lock()
	purchaseLastHashes = append([]string(nil), hashes...)
	purchaseLastSplitTx = splitTx
	purchaseLastErr = errMsg
	purchaseResultMu.Unlock()
}

// PurchaseStatusSnapshot reports whether a manual purchase is running plus the
// last terminal result. Mirrors AutobuyerStatusSnapshot.
func PurchaseStatusSnapshot() types.PurchaseStatus {
	purchaseResultMu.Lock()
	defer purchaseResultMu.Unlock()
	return types.PurchaseStatus{
		InProgress:   IsTicketPurchaseInProgress(),
		LastError:    purchaseLastErr,
		TicketHashes: append([]string(nil), purchaseLastHashes...),
		SplitTxHash:  purchaseLastSplitTx,
	}
}

// LastPurchaseEvents returns up to n most-recent purchase events, oldest first.
// A reconnecting WebSocket replays these, so a page reload during a long mixed
// purchase still receives the terminal "done"/"error" event.
func LastPurchaseEvents(n int) []types.PurchaseEvent {
	purchaseEventsMu.Lock()
	defer purchaseEventsMu.Unlock()
	if n <= 0 || n > len(purchaseEvents) {
		n = len(purchaseEvents)
	}
	out := make([]types.PurchaseEvent, n)
	copy(out, purchaseEvents[len(purchaseEvents)-n:])
	return out
}

// SubscribePurchaseEvents returns a channel receiving every future purchase
// event plus a cleanup func to call when the subscriber detaches.
func SubscribePurchaseEvents() (<-chan types.PurchaseEvent, func()) {
	ch := make(chan types.PurchaseEvent, 32)
	purchaseSubsMu.Lock()
	purchaseSubscribers = append(purchaseSubscribers, ch)
	purchaseSubsMu.Unlock()
	return ch, func() {
		purchaseSubsMu.Lock()
		defer purchaseSubsMu.Unlock()
		for i, sub := range purchaseSubscribers {
			if sub == ch {
				purchaseSubscribers = append(purchaseSubscribers[:i], purchaseSubscribers[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

func emitPurchaseEvent(ev types.PurchaseEvent) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	purchaseEventsMu.Lock()
	purchaseEvents = append(purchaseEvents, ev)
	if len(purchaseEvents) > purchaseEventBufferSize {
		purchaseEvents = purchaseEvents[len(purchaseEvents)-purchaseEventBufferSize:]
	}
	purchaseEventsMu.Unlock()

	purchaseSubsMu.Lock()
	for _, sub := range purchaseSubscribers {
		select {
		case sub <- ev:
		default:
		}
	}
	purchaseSubsMu.Unlock()
}

func recordPurchaseEvent(level, msg string) {
	emitPurchaseEvent(types.PurchaseEvent{Level: level, Message: msg, Kind: "progress"})
}

// StartPurchaseWorker dispatches a privacy/mixed ticket purchase to a background
// goroutine and returns immediately. The purchase pauses the mixer, CSPP-mixes
// the split transaction (which only forms every ~10 minutes) and buys the
// ticket, so it cannot fit in a single HTTP round-trip. Progress and the
// terminal result/error are emitted as purchase events and streamed to the
// frontend over the purchase-events WebSocket. The passphrase is copied because
// the goroutine outlives the request and the caller zeroes its own slice.
func StartPurchaseWorker(account, numTickets uint32, vspHost, vspPubkey string, changeAccount uint32, passphrase []byte) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC client not initialized")
	}
	if numTickets == 0 {
		return fmt.Errorf("numTickets must be > 0")
	}
	if vspHost == "" || vspPubkey == "" {
		return fmt.Errorf("vspHost and vspPubkey are required")
	}
	if !tryBeginTicketPurchase() {
		return fmt.Errorf("a ticket purchase is already in progress")
	}

	passCopy := append([]byte(nil), passphrase...)
	setPurchaseResult(nil, "", "") // clear any prior result while this one runs
	recordPurchaseEvent("info", fmt.Sprintf(
		"Purchasing %d mixed ticket(s). Funds are CSPP-mixed before the ticket is bought, which can take up to ~10 minutes.",
		numTickets))

	go func() {
		defer endTicketPurchase()
		defer func() {
			for i := range passCopy {
				passCopy[i] = 0
			}
		}()

		// Detached context: the request context is cancelled once the 202 is
		// written, so the worker owns its own. No deadline is set: PurchaseTickets
		// with mixing pairs the CoinShuffle++ split only on epoch boundaries (10
		// min on mainnet) plus a 20-60s trickle delay per ticket, so any fixed
		// budget can be exceeded. dcrwallet drives the timing; the autobuyer's
		// RunTicketBuyer stream runs deadline-free for the same reason.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		resp, err := purchaseTicketsCore(ctx, account, numTickets, vspHost, vspPubkey, changeAccount, passCopy)
		if err != nil {
			setPurchaseResult(nil, "", err.Error())
			emitPurchaseEvent(types.PurchaseEvent{Level: "error", Kind: "error", Message: "Purchase failed: " + err.Error()})
			return
		}
		setPurchaseResult(resp.TicketHashes, resp.SplitTxHash, "")
		msg := "Tickets purchased"
		if len(resp.TicketHashes) > 0 {
			msg = "Tickets purchased: " + strings.Join(resp.TicketHashes, ", ")
		}
		emitPurchaseEvent(types.PurchaseEvent{
			Level:        "info",
			Kind:         "done",
			Message:      msg,
			TicketHashes: resp.TicketHashes,
			SplitTxHash:  resp.SplitTxHash,
		})
	}()
	return nil
}

// PurchaseTickets runs a ticket purchase synchronously under the single-flight
// guard. Used for plain (non-privacy) purchases, which complete quickly.
func PurchaseTickets(ctx context.Context, account, numTickets uint32, vspHost, vspPubkey string, changeAccount uint32, passphrase []byte) (*types.PurchaseTicketsResponse, error) {
	if !tryBeginTicketPurchase() {
		return nil, fmt.Errorf("a ticket purchase is already in progress")
	}
	defer endTicketPurchase()
	return purchaseTicketsCore(ctx, account, numTickets, vspHost, vspPubkey, changeAccount, passphrase)
}

// purchaseTicketsCore calls dcrwallet's PurchaseTickets gRPC with the modern VSP
// fields. Uses lazy per-account-encryption migration on the source account. The
// caller owns the single-flight guard (see tryBeginTicketPurchase).
func purchaseTicketsCore(ctx context.Context, account, numTickets uint32, vspHost, vspPubkey string, changeAccount uint32, passphrase []byte) (*types.PurchaseTicketsResponse, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC client not initialized")
	}
	if numTickets == 0 {
		return nil, fmt.Errorf("numTickets must be > 0")
	}
	if vspHost == "" || vspPubkey == "" {
		return nil, fmt.Errorf("vspHost and vspPubkey are required")
	}

	// When privacy is configured, buy mixed tickets: fund + split + mix from the
	// "mixed" account, send change to the "unmixed" account, and enable mixing.
	// The backend is the source of truth here, overriding the caller's account so
	// a privacy-enabled wallet can never produce a half-mixed ticket. Otherwise
	// buy plainly with change going back to the source account.
	sourceAccount := account
	changeAcct := changeAccount
	mixing, mixed := TicketMixingParams(ctx)
	if mixed {
		sourceAccount = mixing.Mixed
		changeAcct = mixing.Change
	}

	// The continuous mixer and a ticket purchase both spend the mixed account,
	// so they must not run together: pause a running mixer for the purchase and
	// restart it afterwards. Mirrors Decrediton's purchaseTicketsAttempt.
	if mixed && IsMixerRunning() {
		StopMixer()
		WaitForMixerStop(5 * time.Second)
		// The caller zeroes the passphrase once this returns, but a restarted
		// mixer keeps it for its lifetime, so hand it a copy.
		mixerPass := append([]byte(nil), passphrase...)
		mixerMixed, mixerChange := mixing.Mixed, mixing.Change
		defer func() {
			if err := StartMixer(mixerPass, mixerMixed, privacyMixedAccountBranch, mixerChange); err != nil {
				log.Printf("restart mixer after ticket purchase: %v", err)
			}
		}()
	}

	// Make the source account usable for signing (skips if already unlocked,
	// migrates to per-account encryption if needed).
	if err := unlockAccountForSpend(ctx, sourceAccount, passphrase); err != nil {
		return nil, err
	}
	defer func() {
		relockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = rpc.WalletGrpcClient.LockAccount(relockCtx, &pb.LockAccountRequest{AccountNumber: sourceAccount})
	}()

	purchaseReq := &pb.PurchaseTicketsRequest{
		Passphrase:    passphrase,
		Account:       sourceAccount,
		NumTickets:    numTickets,
		VspHost:       "https://" + strings.TrimPrefix(strings.TrimPrefix(vspHost, "https://"), "http://"),
		VspPubkey:     vspPubkey,
		ChangeAccount: changeAcct,
	}
	if mixed {
		purchaseReq.EnableMixing = true
		purchaseReq.MixedAccount = mixing.Mixed
		purchaseReq.MixedSplitAccount = mixing.Mixed
		purchaseReq.MixedAccountBranch = privacyMixedAccountBranch
	}

	resp, err := rpc.WalletGrpcClient.PurchaseTickets(ctx, purchaseReq)
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

	// Annotate immature tickets with the blocks remaining until they mature into
	// the live pool, using the active network's ticket-maturity parameter.
	if ticketMaturity := currentTicketMaturity(ctx); ticketMaturity > 0 && rpc.DcrdClient != nil {
		if bestHeight, herr := rpc.DcrdClient.GetBlockCount(ctx); herr == nil && bestHeight > 0 {
			for i := range records {
				if records[i].Status != "IMMATURE" || records[i].BlockHeight <= 0 {
					continue
				}
				remaining := ticketMaturity - (int32(bestHeight) - records[i].BlockHeight)
				if remaining < 0 {
					remaining = 0
				}
				records[i].BlocksUntilMature = remaining
			}
		}
	}
	return records, nil
}

// currentTicketMaturity returns the active network's ticket maturity (the
// blocks a ticket must age before it becomes live), or 0 if the network can't
// be resolved so callers can skip the annotation.
func currentTicketMaturity(ctx context.Context) int32 {
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return 0
	}
	var params *chaincfg.Params
	switch network {
	case "mainnet":
		params = chaincfg.MainNetParams()
	case "testnet":
		params = chaincfg.TestNet3Params()
	case "simnet":
		params = chaincfg.SimNetParams()
	default:
		return 0
	}
	return int32(params.TicketMaturity)
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
		// The ticket price is the value of the stake submission output (index 0).
		// The wallet owns it (it pays to the voting address), so it is the credit
		// at index 0. Mirrors Decrediton's ticketPrice = credits[0].amount.
		var priceAtoms int64
		for _, c := range t.GetCredits() {
			if c.GetIndex() == 0 {
				priceAtoms = c.GetAmount()
				break
			}
		}
		out.TicketPrice = float64(priceAtoms) / 1e8
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

// countFeeStatuses tallies a fetchFeeStatusMap result into the four VSP
// fee-processing buckets.
func countFeeStatuses(m map[string]string) types.VSPFeeStatusCounts {
	var c types.VSPFeeStatusCounts
	for _, s := range m {
		switch s {
		case "UNPAID":
			c.Unpaid++
		case "PAID":
			c.Paid++
		case "ERRORED":
			c.Errored++
		case "CONFIRMED":
			c.Confirmed++
		}
	}
	return c
}

// SyncFailedVSPTickets retries fee payment for tickets with VSP fee errors
// against the given VSP. Uses the lazy per-account-encryption migration
// pattern from PurchaseTickets. The SyncVSPFailedTickets RPC returns no data,
// so progress is reported via before/after fee-status snapshots.
func SyncFailedVSPTickets(ctx context.Context, vspHost, vspPubkey string, account, changeAccount uint32, passphrase []byte) (*types.SyncFailedVSPTicketsResponse, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC client not initialized")
	}
	if vspHost == "" || vspPubkey == "" {
		return nil, fmt.Errorf("vspHost and vspPubkey are required")
	}

	unlockedAccts, err := unlockAllAccountsForSpend(ctx, passphrase)
	if err != nil {
		return nil, err
	}
	defer relockAccountsAfterVSP(unlockedAccts)

	normHost := "https://" + strings.TrimPrefix(strings.TrimPrefix(vspHost, "https://"), "http://")

	before := countFeeStatuses(fetchFeeStatusMap(ctx))

	// Retry fee payment for errored tickets, then re-check all managed tickets
	// so paid-but-unconfirmed fees advance to confirmed. Mirrors Decrediton's
	// syncVSPTickets followed by processManagedTickets.
	if _, err := rpc.WalletGrpcClient.SyncVSPFailedTickets(ctx, &pb.SyncVSPTicketsRequest{
		VspHost:       normHost,
		VspPubkey:     vspPubkey,
		Account:       account,
		ChangeAccount: changeAccount,
	}); err != nil {
		return nil, fmt.Errorf("SyncVSPFailedTickets RPC: %w", err)
	}
	if _, err := rpc.WalletGrpcClient.ProcessManagedTickets(ctx, &pb.ProcessManagedTicketsRequest{
		VspHost:       normHost,
		VspPubkey:     vspPubkey,
		FeeAccount:    account,
		ChangeAccount: changeAccount,
	}); err != nil {
		return nil, fmt.Errorf("ProcessManagedTickets RPC: %w", err)
	}
	rememberVSPUsed(ctx, vspHost, vspPubkey)

	after := countFeeStatuses(fetchFeeStatusMap(ctx))

	return &types.SyncFailedVSPTicketsResponse{
		VspHost: vspHost,
		Before:  before,
		After:   after,
	}, nil
}

// ProcessUnmanagedVSPTickets re-associates tickets that the wallet is not yet
// tracking against a VSP with the given VSP. After a seed restore or wallet
// import the on-chain tickets are recovered but their local VSP fee records are
// gone, so they show as untracked; this re-syncs known-paid tickets and pays
// genuinely-unpaid ones, while tickets the VSP does not recognize are skipped.
// Mirrors Decrediton's processUnmanagedTickets (one user-selected VSP per run).
func ProcessUnmanagedVSPTickets(ctx context.Context, vspHost, vspPubkey string, account, changeAccount uint32, passphrase []byte) (*types.SyncFailedVSPTicketsResponse, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC client not initialized")
	}
	if vspHost == "" || vspPubkey == "" {
		return nil, fmt.Errorf("vspHost and vspPubkey are required")
	}

	unlockedAccts, err := unlockAllAccountsForSpend(ctx, passphrase)
	if err != nil {
		return nil, err
	}
	defer relockAccountsAfterVSP(unlockedAccts)

	normHost := "https://" + strings.TrimPrefix(strings.TrimPrefix(vspHost, "https://"), "http://")

	before := countFeeStatuses(fetchFeeStatusMap(ctx))

	if _, err := rpc.WalletGrpcClient.ProcessUnmanagedTickets(ctx, &pb.ProcessUnmanagedTicketsRequest{
		VspHost:       normHost,
		VspPubkey:     vspPubkey,
		FeeAccount:    account,
		ChangeAccount: changeAccount,
	}); err != nil {
		return nil, fmt.Errorf("ProcessUnmanagedTickets RPC: %w", err)
	}
	rememberVSPUsed(ctx, vspHost, vspPubkey)

	after := countFeeStatuses(fetchFeeStatusMap(ctx))

	return &types.SyncFailedVSPTicketsResponse{
		VspHost: vspHost,
		Before:  before,
		After:   after,
	}, nil
}
