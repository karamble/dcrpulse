// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	dcrwpb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"encoding/hex"

	"encoding/base64"

	"github.com/decred/dcrlnd/lnrpc"
	"github.com/decred/dcrlnd/lnrpc/autopilotrpc"
	"github.com/decred/dcrlnd/lnrpc/invoicesrpc"
	"github.com/decred/dcrlnd/lnrpc/routerrpc"
	"github.com/decred/dcrlnd/lnrpc/verrpc"
	"github.com/decred/dcrlnd/lnrpc/wtclientrpc"
)

const (
	// LightningAccountName is the dcrwallet account dedicated to LN
	// channel funding + on-chain LN operations. Mirrors Decrediton's
	// per-wallet "lightning" account convention.
	LightningAccountName = "lightning"
)

// sentinelPath is the per-wallet file the dashboard's setup wizard writes to
// unblock the dcrlnd supervisor for the active wallet. It lives in that wallet's
// dcrlnd directory, the same path the dcrlnd supervisor polls.
func sentinelPath() string {
	return filepath.Join(config.DcrlndDir(CurrentWalletName()), ".account")
}

// SetupLightningAccount creates (or looks up) the dedicated "lightning"
// dcrwallet account, sets a per-account passphrase on it so dcrlnd can
// UnlockAccount it independently of the wallet-wide passphrase, then
// writes its number to /app-data/dcrlnd/.account so the dcrlnd
// container's entrypoint can proceed.
//
// The per-account passphrase is critical: dcrlnd's dcrw chain backend
// calls walletrpc.UnlockAccount on the LN account, which dcrwallet
// rejects with "account is not encrypted with a unique passphrase"
// unless the account has been migrated to per-account encryption via
// SetAccountPassphrase. We reuse the user's wallet passphrase as the
// per-account passphrase so the LN setup wizard takes one input.
func SetupLightningAccount(ctx context.Context, passphrase []byte) (uint32, error) {
	if rpc.WalletGrpcClient == nil {
		return 0, fmt.Errorf("dcrwallet gRPC unavailable")
	}

	// Existing-account lookup first — re-running the wizard after a
	// container wipe should reuse the on-chain account rather than
	// creating account 2, 3, 4…
	acctsResp, err := rpc.WalletGrpcClient.Accounts(ctx, &dcrwpb.AccountsRequest{})
	if err != nil {
		return 0, fmt.Errorf("list accounts: %w", err)
	}
	var acctNum uint32
	var found bool
	for _, a := range acctsResp.GetAccounts() {
		if a.GetAccountName() == LightningAccountName {
			acctNum = a.GetAccountNumber()
			found = true
			break
		}
	}

	if !found {
		// Account doesn't exist yet — create it. NextAccount requires
		// the wallet passphrase because it derives a new BIP44 branch.
		naResp, err := rpc.WalletGrpcClient.NextAccount(ctx, &dcrwpb.NextAccountRequest{
			Passphrase:  passphrase,
			AccountName: LightningAccountName,
		})
		if err != nil {
			return 0, fmt.Errorf("create lightning account: %w", err)
		}
		acctNum = naResp.GetAccountNumber()
	}

	if err := ensureLightningAccountPassphrase(ctx, acctNum, passphrase); err != nil {
		return 0, err
	}

	if err := writeSentinel(acctNum); err != nil {
		return 0, err
	}
	return acctNum, nil
}

// ensureLightningAccountPassphrase migrates the lightning dcrwallet
// account into per-account encryption mode if it is not already. dcrlnd
// calls walletrpc.UnlockAccount on the LN account; that gRPC fails on
// default-passphrase accounts with "account is not encrypted with a
// unique passphrase". Mirrors Decrediton's `setAccountPassphrase` call
// inside `getNextAccountAttempt` (ControlActions.js:141-165) which
// always sets the per-account passphrase equal to the wallet passphrase
// so the user has only one to remember. See
// [[project_dcrwallet_unlock_semantics]].
//
// Idempotent: an already-migrated account is detected via the
// "already" substring in dcrwallet's error and treated as success.
func ensureLightningAccountPassphrase(ctx context.Context, acctNum uint32, passphrase []byte) error {
	_, err := rpc.WalletGrpcClient.SetAccountPassphrase(ctx, &dcrwpb.SetAccountPassphraseRequest{
		AccountNumber:        acctNum,
		WalletPassphrase:     passphrase,
		NewAccountPassphrase: passphrase,
	})
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "already") {
		return nil
	}
	return fmt.Errorf("set lightning account passphrase: %w", err)
}

func writeSentinel(account uint32) error {
	if err := os.MkdirAll(filepath.Dir(sentinelPath()), 0o700); err != nil {
		return fmt.Errorf("create dcrlnd state dir: %w", err)
	}
	return os.WriteFile(sentinelPath(), []byte(strconv.FormatUint(uint64(account), 10)), 0o600)
}

// readSentinelAccount returns the dcrwallet account number stored in
// the sentinel file, or (_, false) if the wizard has not run yet.
func readSentinelAccount() (uint32, bool) {
	b, err := os.ReadFile(sentinelPath())
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

// InitLightningWallet bootstraps dcrlnd's own internal wallet. dcrlnd
// keeps its own seed (separate from dcrwallet) for channel signing
// keys and per-channel state. We don't display the seed to the user —
// Decrediton doesn't either; SCB backups cover recovery. Reused for
// the first-time init only.
func InitLightningWallet(ctx context.Context, passphrase []byte) error {
	if rpc.WalletUnlockerClient == nil {
		if err := rpc.ReinitDcrlndClient(); err != nil {
			return fmt.Errorf("dcrlnd not reachable: %w", err)
		}
	}
	if rpc.WalletUnlockerClient == nil {
		return fmt.Errorf("dcrlnd wallet unlocker unavailable")
	}

	seed, err := rpc.WalletUnlockerClient.GenSeed(ctx, &lnrpc.GenSeedRequest{})
	if err != nil {
		return fmt.Errorf("GenSeed: %w", err)
	}
	_, err = rpc.WalletUnlockerClient.InitWallet(ctx, &lnrpc.InitWalletRequest{
		WalletPassword:     passphrase,
		CipherSeedMnemonic: seed.GetCipherSeedMnemonic(),
	})
	if err != nil {
		return fmt.Errorf("InitWallet: %w", err)
	}
	return nil
}

// UnlockLightningWallet is called on subsequent dashboard starts when
// dcrlnd's wallet is already initialised but still locked.
func UnlockLightningWallet(ctx context.Context, passphrase []byte) error {
	if rpc.WalletUnlockerClient == nil {
		if err := rpc.ReinitDcrlndClient(); err != nil {
			return fmt.Errorf("dcrlnd not reachable: %w", err)
		}
	}
	if rpc.WalletUnlockerClient == nil {
		return fmt.Errorf("dcrlnd wallet unlocker unavailable")
	}
	_, err := rpc.WalletUnlockerClient.UnlockWallet(ctx, &lnrpc.UnlockWalletRequest{
		WalletPassword: passphrase,
	})
	if err != nil {
		return fmt.Errorf("UnlockWallet: %w", err)
	}
	return nil
}

// LightningStatus reports the high-level stage the UI should render.
// Decrediton drives an equivalent state machine off LNActions.js's
// stage constants (STARTUPSTAGE_*).
func LightningStatus(ctx context.Context) types.LightningStatus {
	out := types.LightningStatus{Stage: "unavailable"}

	if _, ok := readSentinelAccount(); !ok {
		out.Stage = "needs-setup"
		return out
	}

	// The sentinel exists; the dcrlnd container should now be running. A nil
	// client means the TLS cert is absent, i.e. dcrlnd has not come up yet —
	// report "starting" rather than the unlock wizard.
	if rpc.LightningClient == nil {
		_ = rpc.ReinitDcrlndClient()
	}
	if rpc.LightningClient == nil {
		out.Stage = "unavailable"
		out.Message = DaemonStartupHint(ctx, LogComponentDcrlnd).Message
		return out
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := rpc.LightningClient.GetInfo(callCtx, &lnrpc.GetInfoRequest{})
	if err == nil {
		out.Stage = "syncing"
		if info.GetSyncedToChain() && info.GetSyncedToGraph() {
			out.Stage = "ready"
		}
		out.IdentityPubkey = info.GetIdentityPubkey()
		out.Alias = info.GetAlias()
		out.BlockHeight = info.GetBlockHeight()
		out.SyncedToChain = info.GetSyncedToChain()
		out.SyncedToGraph = info.GetSyncedToGraph()
		out.NumActiveChans = info.GetNumActiveChannels()
		out.NumPendingChans = info.GetNumPendingChannels()
		return out
	}

	// Classify the failure: "no wallet on disk" routes back to the setup
	// wizard; a daemon that is down or still starting up shows the "starting"
	// state; everything else (locked / unknown) keeps the unlock-only path.
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "not created"), strings.Contains(lower, "wallet exists"):
		out.Stage = "needs-setup"
	case LndStartupOrUnreachable(err):
		out.Stage = "unavailable"
		out.Message = DaemonStartupHint(ctx, LogComponentDcrlnd).Message
	default:
		log.Printf("LightningStatus: GetInfo: %v", err)
		out.Stage = "needs-unlock"
	}
	return out
}

// GetLightningInfo wraps GetInfo for the read-only info endpoint.
func GetLightningInfo(ctx context.Context) (*types.LightningInfo, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := rpc.LightningClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf("GetInfo: %w", err)
	}
	// Render the version as "v<major>.<minor>.<patch>" to match how
	// dcrd/dcrwallet appear in the footer ("v2.1.5"). The Versioner
	// sub-RPC exposes the clean integer triple; its `version` string
	// field includes "-pre+<commit>" build metadata which we don't
	// want here. GetInfo's `version` field is the last-resort fallback
	// if the verrpc call fails.
	version := "v" + strings.TrimPrefix(resp.GetVersion(), "v")
	if rpc.VersionerClient != nil {
		if vresp, verr := rpc.VersionerClient.GetVersion(ctx, &verrpc.VersionRequest{}); verr == nil {
			version = fmt.Sprintf("v%d.%d.%d", vresp.GetAppMajor(), vresp.GetAppMinor(), vresp.GetAppPatch())
		}
	}
	return &types.LightningInfo{
		IdentityPubkey:      resp.GetIdentityPubkey(),
		Alias:               resp.GetAlias(),
		Version:             version,
		BlockHeight:         resp.GetBlockHeight(),
		BlockHash:           resp.GetBlockHash(),
		SyncedToChain:       resp.GetSyncedToChain(),
		SyncedToGraph:       resp.GetSyncedToGraph(),
		NumActiveChannels:   resp.GetNumActiveChannels(),
		NumInactiveChannels: resp.GetNumInactiveChannels(),
		NumPendingChannels:  resp.GetNumPendingChannels(),
		NumPeers:            resp.GetNumPeers(),
		BestHeaderTimestamp: resp.GetBestHeaderTimestamp(),
		Chains:              chainStrings(resp.GetChains()),
	}, nil
}

func chainStrings(chains []*lnrpc.Chain) []string {
	out := make([]string, 0, len(chains))
	for _, c := range chains {
		out = append(out, c.GetChain()+"/"+c.GetNetwork())
	}
	return out
}

// GetLightningBalance merges WalletBalance + ChannelBalance into the
// shape the Overview grid renders.
//
// dcrlnd's WalletBalance.{Total,Confirmed,Unconfirmed}Balance fields
// SUM every dcrwallet account (rpcserver.go:3025-3055). For an LN
// dashboard those numbers mix in funds that LN cannot touch (default,
// mixed, unmixed, etc.). dcrlnd bounds channel funding to the account
// passed via --dcrwallet.accountnumber, so the only meaningful on-chain
// balance is that one account's. We pull the per-account breakdown
// (AccountBalance map) and look up the "lightning" entry.
func GetLightningBalance(ctx context.Context) (*types.LightningBalance, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	wb, err := rpc.LightningClient.WalletBalance(ctx, &lnrpc.WalletBalanceRequest{})
	if err != nil {
		return nil, fmt.Errorf("WalletBalance: %w", err)
	}
	cb, err := rpc.LightningClient.ChannelBalance(ctx, &lnrpc.ChannelBalanceRequest{})
	if err != nil {
		return nil, fmt.Errorf("ChannelBalance: %w", err)
	}
	out := &types.LightningBalance{}
	if lnAcct, ok := wb.GetAccountBalance()[LightningAccountName]; ok {
		out.OnChainConfirmed = lnAcct.GetConfirmedBalance()
		out.OnChainUnconfirmed = lnAcct.GetUnconfirmedBalance()
		out.OnChainTotal = lnAcct.GetConfirmedBalance() + lnAcct.GetUnconfirmedBalance()
	}
	if lb := cb.GetLocalBalance(); lb != nil {
		out.ChannelLocal = int64(lb.GetAtoms())
	}
	if rb := cb.GetRemoteBalance(); rb != nil {
		out.ChannelRemote = int64(rb.GetAtoms())
	}
	if pb := cb.GetPendingOpenLocalBalance(); pb != nil {
		out.ChannelPending = int64(pb.GetAtoms())
	}
	return out, nil
}

// GetLightningActivity returns a merged top-N feed of recent invoices,
// payments, and channel events for the Overview tab. Decrediton
// renders the same union via the OverviewTab recent-activity list.
func GetLightningActivity(ctx context.Context) (*types.LightningActivity, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	entries := make([]types.LightningActivityEntry, 0, 30)

	invResp, err := rpc.LightningClient.ListInvoices(ctx, &lnrpc.ListInvoiceRequest{
		NumMaxInvoices: 10, Reversed: true,
	})
	if err == nil {
		for _, inv := range invResp.GetInvoices() {
			entries = append(entries, types.LightningActivityEntry{
				Kind:      "invoice",
				Timestamp: inv.GetCreationDate(),
				Amount:    inv.GetValue(),
				State:     inv.GetState().String(),
				Memo:      inv.GetMemo(),
			})
		}
	} else {
		log.Printf("ListInvoices: %v", err)
	}

	payResp, err := rpc.LightningClient.ListPayments(ctx, &lnrpc.ListPaymentsRequest{
		MaxPayments: 10, Reversed: true,
	})
	if err == nil {
		for _, p := range payResp.GetPayments() {
			entries = append(entries, types.LightningActivityEntry{
				Kind:      "payment",
				Timestamp: p.GetCreationTimeNs() / 1_000_000_000,
				Amount:    p.GetValueAtoms(),
				State:     p.GetStatus().String(),
			})
		}
	} else {
		log.Printf("ListPayments: %v", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})
	if len(entries) > 10 {
		entries = entries[:10]
	}
	return &types.LightningActivity{Entries: entries}, nil
}

// ---- Channels (Decrediton parity) ------------------------------------------

// ListLightningChannels merges the three dcrlnd RPCs Decrediton runs in
// parallel (ListChannels + PendingChannels + ClosedChannels) into one
// flat slice with a status discriminator, matching Decrediton's
// LNActions.js:464-540 pattern.
func ListLightningChannels(ctx context.Context) (*types.LightningChannels, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	out := &types.LightningChannels{Channels: []types.LightningChannel{}}

	openResp, err := rpc.LightningClient.ListChannels(ctx, &lnrpc.ListChannelsRequest{})
	if err == nil {
		for _, c := range openResp.GetChannels() {
			out.Channels = append(out.Channels, types.LightningChannel{
				Status:         types.ChannelStatusOpen,
				ChannelPoint:   c.GetChannelPoint(),
				ChannelID:      c.GetChanId(),
				RemotePubkey:   c.GetRemotePubkey(),
				Capacity:       c.GetCapacity(),
				LocalBalance:   c.GetLocalBalance(),
				RemoteBalance:  c.GetRemoteBalance(),
				CommitFee:      c.GetCommitFee(),
				UnsettledBal:   c.GetUnsettledBalance(),
				TotalSentAtoms: c.GetTotalAtomsSent(),
				TotalRecvAtoms: c.GetTotalAtomsReceived(),
				NumUpdates:     c.GetNumUpdates(),
				CSVDelay:       c.GetCsvDelay(),
				Active:         c.GetActive(),
				Private:        c.GetPrivate(),
				Initiator:      c.GetInitiator(),
			})
		}
	} else {
		log.Printf("ListChannels: %v", err)
	}

	pendResp, err := rpc.LightningClient.PendingChannels(ctx, &lnrpc.PendingChannelsRequest{})
	if err == nil {
		for _, p := range pendResp.GetPendingOpenChannels() {
			row := pendingChannelRow(p.GetChannel(), types.ChannelStatusPendingOpen, "", 0)
			// Enrich with funding-tx confirmation progress. dcrwallet
			// already has the funding tx in its local index (we
			// broadcast it from the lightning account), so reading its
			// confirmation count is a cheap hash-keyed lookup. The
			// required count is derived from capacity (dcrlnd never
			// populates ConfirmationHeight on pending-open channels).
			row.CurrentConfs, row.RequiredConfs = fundingTxConfProgress(ctx, row.ChannelPoint, row.Capacity, row.RemoteBalance)
			out.Channels = append(out.Channels, row)
		}
		for _, p := range pendResp.GetPendingClosingChannels() {
			out.Channels = append(out.Channels, pendingChannelRow(p.GetChannel(), types.ChannelStatusPendingCloseCoop, p.GetClosingTxid(), 0))
		}
		for _, p := range pendResp.GetPendingForceClosingChannels() {
			out.Channels = append(out.Channels, pendingChannelRow(p.GetChannel(), types.ChannelStatusPendingCloseForce, p.GetClosingTxid(), p.GetLimboBalance()))
		}
		for _, p := range pendResp.GetWaitingCloseChannels() {
			out.Channels = append(out.Channels, pendingChannelRow(p.GetChannel(), types.ChannelStatusPendingWaitClose, p.GetClosingTxid(), p.GetLimboBalance()))
		}
	} else {
		log.Printf("PendingChannels: %v", err)
	}

	closedResp, err := rpc.LightningClient.ClosedChannels(ctx, &lnrpc.ClosedChannelsRequest{})
	if err == nil {
		for _, c := range closedResp.GetChannels() {
			out.Channels = append(out.Channels, types.LightningChannel{
				Status:         types.ChannelStatusClosed,
				ChannelPoint:   c.GetChannelPoint(),
				ChannelID:      c.GetChanId(),
				RemotePubkey:   c.GetRemotePubkey(),
				Capacity:       c.GetCapacity(),
				CloseType:      c.GetCloseType().String(),
				ClosingTxHash:  c.GetClosingTxHash(),
				SettledBalance: c.GetSettledBalance(),
				TimeLockedBal:  c.GetTimeLockedBalance(),
			})
		}
	} else {
		log.Printf("ClosedChannels: %v", err)
	}

	return out, nil
}

func pendingChannelRow(c *lnrpc.PendingChannelsResponse_PendingChannel, status, closingTxid string, limbo int64) types.LightningChannel {
	if c == nil {
		return types.LightningChannel{Status: status}
	}
	return types.LightningChannel{
		Status:        status,
		ChannelPoint:  c.GetChannelPoint(),
		RemotePubkey:  c.GetRemoteNodePub(),
		Capacity:      c.GetCapacity(),
		LocalBalance:  c.GetLocalBalance(),
		RemoteBalance: c.GetRemoteBalance(),
		ClosingTxHash: closingTxid,
		LimboBalance:  limbo,
		// PendingChannel reports Initiator as an enum (Local/Remote/Both);
		// open channels (above) use a bool. Map to bool so the type stays
		// uniform across channel statuses.
		Initiator: c.GetInitiator() == lnrpc.Initiator_INITIATOR_LOCAL,
	}
}

// lightningChannelTxIDs returns the funding and closing transaction IDs of
// every channel dcrlnd knows about (open, pending, closed), used to tag
// wallet-history rows. Best-effort: an unavailable dcrlnd or a failing RPC
// yields empty maps rather than an error.
func lightningChannelTxIDs(ctx context.Context) (funding, closing map[string]bool) {
	funding = make(map[string]bool)
	closing = make(map[string]bool)
	if rpc.LightningClient == nil {
		return funding, closing
	}

	addFunding := func(channelPoint string) {
		if txid, _, err := splitChannelPoint(channelPoint); err == nil {
			funding[txid] = true
		}
	}
	addClosing := func(txid string) {
		if txid != "" {
			closing[txid] = true
		}
	}

	if resp, err := rpc.LightningClient.ListChannels(ctx, &lnrpc.ListChannelsRequest{}); err == nil {
		for _, c := range resp.GetChannels() {
			addFunding(c.GetChannelPoint())
		}
	}
	if resp, err := rpc.LightningClient.PendingChannels(ctx, &lnrpc.PendingChannelsRequest{}); err == nil {
		for _, p := range resp.GetPendingOpenChannels() {
			addFunding(p.GetChannel().GetChannelPoint())
		}
		for _, p := range resp.GetPendingClosingChannels() {
			addFunding(p.GetChannel().GetChannelPoint())
			addClosing(p.GetClosingTxid())
		}
		for _, p := range resp.GetPendingForceClosingChannels() {
			addFunding(p.GetChannel().GetChannelPoint())
			addClosing(p.GetClosingTxid())
		}
		for _, p := range resp.GetWaitingCloseChannels() {
			addFunding(p.GetChannel().GetChannelPoint())
			addClosing(p.GetClosingTxid())
		}
	}
	if resp, err := rpc.LightningClient.ClosedChannels(ctx, &lnrpc.ClosedChannelsRequest{}); err == nil {
		for _, c := range resp.GetChannels() {
			addFunding(c.GetChannelPoint())
			addClosing(c.GetClosingTxHash())
		}
	}
	return funding, closing
}

// OpenLightningChannel parses the user-supplied peer URI (`pubkey` or
// `pubkey@host:port`), runs ConnectPeer when an address is present, and
// then OpenChannelSync. Decrediton's flow at LNActions.js:775-851 uses
// the streaming variant; we use the sync variant for simpler HTTP
// semantics — the channel's progression from pending to open is then
// reflected via the live channel-events WebSocket.
func OpenLightningChannel(ctx context.Context, req *types.OpenChannelRequest) (*types.OpenChannelResponse, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	pubkeyHex, hostPort, err := splitPeerURI(req.PeerURI)
	if err != nil {
		return nil, err
	}
	pubkey, err := hexDecodeStrict(pubkeyHex, 33)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %w", err)
	}
	if hostPort != "" {
		_, cerr := rpc.LightningClient.ConnectPeer(ctx, &lnrpc.ConnectPeerRequest{
			Addr: &lnrpc.LightningAddress{Pubkey: pubkeyHex, Host: hostPort},
		})
		if cerr != nil && !strings.Contains(strings.ToLower(cerr.Error()), "already connected") {
			return nil, fmt.Errorf("ConnectPeer: %w", cerr)
		}
	}
	oresp, err := rpc.LightningClient.OpenChannelSync(ctx, &lnrpc.OpenChannelRequest{
		NodePubkey:         pubkey,
		LocalFundingAmount: req.LocalAtoms,
		PushAtoms:          req.PushAtoms,
		Private:            req.Private,
	})
	if err != nil {
		return nil, fmt.Errorf("OpenChannelSync: %w", err)
	}
	txid := ""
	if hashBytes := oresp.GetFundingTxidBytes(); len(hashBytes) > 0 {
		txid = reversedHex(hashBytes)
	} else {
		txid = oresp.GetFundingTxidStr()
	}
	return &types.OpenChannelResponse{
		FundingTxid: txid,
		OutputIndex: oresp.GetOutputIndex(),
	}, nil
}

// CloseLightningChannel opens dcrlnd's streaming CloseChannel and reads
// events until closePending arrives. Decrediton waits for both
// closePending + chanClose, but the latter can take days for cooperative
// closes; returning on closePending is enough for the dashboard to move
// the channel to its pending-close state in the list.
func CloseLightningChannel(ctx context.Context, channelPoint string, force bool) (*types.CloseChannelResponse, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	cp, err := parseChannelPoint(channelPoint)
	if err != nil {
		return nil, err
	}
	stream, err := rpc.LightningClient.CloseChannel(ctx, &lnrpc.CloseChannelRequest{
		ChannelPoint: cp,
		Force:        force,
	})
	if err != nil {
		return nil, fmt.Errorf("CloseChannel: %w", err)
	}
	for {
		upd, err := stream.Recv()
		if err != nil {
			return nil, fmt.Errorf("CloseChannel stream: %w", err)
		}
		if pend := upd.GetClosePending(); pend != nil {
			return &types.CloseChannelResponse{ClosingTxid: reversedHex(pend.GetTxid())}, nil
		}
		// chanClose can fire too; we already have the txid from closePending.
		if upd.GetChanClose() != nil {
			return &types.CloseChannelResponse{}, nil
		}
	}
}

// GetLightningAutopilotStatus reads the current autopilot active flag.
func GetLightningAutopilotStatus(ctx context.Context) (*types.AutopilotStatus, error) {
	if rpc.AutopilotClient == nil {
		return nil, fmt.Errorf("dcrlnd autopilot rpc not available")
	}
	resp, err := rpc.AutopilotClient.Status(ctx, &autopilotrpc.StatusRequest{})
	if err != nil {
		return nil, fmt.Errorf("AutopilotStatus: %w", err)
	}
	return &types.AutopilotStatus{Active: resp.GetActive()}, nil
}

// SetLightningAutopilotStatus toggles autopilot. Mirrors Decrediton's
// LNActions.js:1181-1193.
func SetLightningAutopilotStatus(ctx context.Context, enable bool) error {
	if rpc.AutopilotClient == nil {
		return fmt.Errorf("dcrlnd autopilot rpc not available")
	}
	_, err := rpc.AutopilotClient.ModifyStatus(ctx, &autopilotrpc.ModifyStatusRequest{Enable: enable})
	if err != nil {
		return fmt.Errorf("ModifyStatus: %w", err)
	}
	return nil
}

// SearchLightningNodes queries dcrlnd's DescribeGraph and filters
// client-side by substring match against alias + pubkey. Capped at 50.
// Until the channel graph syncs (i.e. the wallet has a connected peer
// gossiping to it) DescribeGraph returns an empty list; that's the
// expected behaviour and the UI renders the empty state.
func SearchLightningNodes(ctx context.Context, query string) (*types.NodeSearchResponse, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := rpc.LightningClient.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{IncludeUnannounced: false})
	if err != nil {
		return nil, fmt.Errorf("DescribeGraph: %w", err)
	}
	out := &types.NodeSearchResponse{Matches: []types.NodeMatch{}}
	q := strings.ToLower(strings.TrimSpace(query))
	for _, n := range resp.GetNodes() {
		if q != "" {
			if !strings.Contains(strings.ToLower(n.GetAlias()), q) &&
				!strings.Contains(strings.ToLower(n.GetPubKey()), q) {
				continue
			}
		}
		out.Matches = append(out.Matches, types.NodeMatch{
			Pubkey: n.GetPubKey(),
			Alias:  n.GetAlias(),
			Color:  n.GetColor(),
		})
		if len(out.Matches) >= 50 {
			break
		}
	}
	return out, nil
}

// ---- Channel helpers -------------------------------------------------------

func splitPeerURI(uri string) (pubkey, hostPort string, err error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", "", fmt.Errorf("empty peer URI")
	}
	parts := strings.SplitN(uri, "@", 2)
	if len(parts) == 1 {
		return parts[0], "", nil
	}
	if parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("malformed peer URI: %q", uri)
	}
	return parts[0], parts[1], nil
}

func hexDecodeStrict(s string, wantLen int) ([]byte, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) != wantLen*2 {
		return nil, fmt.Errorf("expected %d hex chars, got %d", wantLen*2, len(s))
	}
	out := make([]byte, wantLen)
	for i := 0; i < wantLen; i++ {
		hi := hexVal(s[i*2])
		lo := hexVal(s[i*2+1])
		if hi < 0 || lo < 0 {
			return nil, fmt.Errorf("invalid hex")
		}
		out[i] = byte(hi<<4 | lo)
	}
	return out, nil
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c - 'a' + 10)
	}
	return -1
}

func parseChannelPoint(s string) (*lnrpc.ChannelPoint, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("channel point must be txid:idx")
	}
	idx, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid output index: %w", err)
	}
	return &lnrpc.ChannelPoint{
		FundingTxid: &lnrpc.ChannelPoint_FundingTxidStr{FundingTxidStr: parts[0]},
		OutputIndex: uint32(idx),
	}, nil
}

func reversedHex(b []byte) string {
	rev := make([]byte, len(b))
	for i, v := range b {
		rev[len(b)-1-i] = v
	}
	const hexdig = "0123456789abcdef"
	out := make([]byte, len(rev)*2)
	for i, v := range rev {
		out[i*2] = hexdig[v>>4]
		out[i*2+1] = hexdig[v&0x0f]
	}
	return string(out)
}

// fundingTxConfProgress returns (currentConfs, requiredConfs) for a
// pending-open channel. requiredConfs is derived from the channel
// capacity (see requiredConfsForCapacity) because dcrlnd never sets a
// confirmation height on PendingOpenChannel (rpcserver.go builds it
// without that field), so the only way to surface "X of Y" progress is
// to recompute Y the same way dcrlnd does. currentConfs comes from
// dcrwallet's GetTransaction RPC: the funding tx was broadcast from the
// lightning account, so dcrwallet has it locally indexed and the lookup
// is a cheap hash-keyed read with no dcrd full-chain scan. currentConfs
// is 0 while the funding tx is still in the mempool; it is capped at
// requiredConfs so the display never shows more confs than needed.
func fundingTxConfProgress(ctx context.Context, channelPoint string, capacity, pushAmt int64) (int32, int32) {
	required := requiredConfsForCapacity(capacity, pushAmt)
	current := int32(0)
	if rpc.WalletGrpcClient != nil {
		if txidHex, _, err := splitChannelPoint(channelPoint); err == nil {
			if hashBytes, err := hexDecodeStrict(txidHex, 32); err == nil {
				// dcrwallet expects little-endian hash bytes on the wire.
				revHash := make([]byte, len(hashBytes))
				for i, v := range hashBytes {
					revHash[len(hashBytes)-1-i] = v
				}
				callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if resp, err := rpc.WalletGrpcClient.GetTransaction(callCtx, &dcrwpb.GetTransactionRequest{
					TransactionHash: revHash,
				}); err == nil {
					if c := resp.GetConfirmations(); c > 0 {
						current = c
					}
				}
			}
		}
	}
	if current > required {
		current = required
	}
	return current, required
}

// requiredConfsForCapacity mirrors dcrlnd's NumRequiredConfs
// (server.go:1192): a channel is considered open after a confirmation
// count that scales linearly from 3 to 6 with channel size, with wumbo
// channels requiring the max. dcrlnd does not expose this per-channel
// over RPC, so we recompute it for pending-channel progress UX. pushAmt
// is the amount pushed to the remote (the remote's opening balance);
// dcrlnd folds it into the stake. The milli-atom scaling dcrlnd applies
// cancels in the ratio, so we work in atoms directly.
func requiredConfsForCapacity(capacity, pushAmt int64) int32 {
	const (
		minConf = 3
		maxConf = 6
		// MaxDecredFundingAmount = dcrutil.Amount(1<<30) - 1.
		maxFundingAmount = int64(1)<<30 - 1
	)
	if capacity > maxFundingAmount {
		return maxConf
	}
	stake := capacity + pushAmt
	conf := int64(maxConf) * stake / maxFundingAmount
	if conf < minConf {
		conf = minConf
	}
	if conf > maxConf {
		conf = maxConf
	}
	return int32(conf)
}

func splitChannelPoint(s string) (txidHex string, outputIdx uint32, err error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("channel point must be txid:idx")
	}
	idx, perr := strconv.ParseUint(parts[1], 10, 32)
	if perr != nil {
		return "", 0, perr
	}
	return parts[0], uint32(idx), nil
}

// ---- Global network statistics --------------------------------------------

// GetLightningNetworkInfo is a passthrough of dcrlnd's GetNetworkInfo
// RPC — graph-wide aggregates (nodes/channels/total capacity + size
// distribution + topology). Cheap enough for per-render polling.
func GetLightningNetworkInfo(ctx context.Context) (*types.LightningNetworkInfo, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := rpc.LightningClient.GetNetworkInfo(ctx, &lnrpc.NetworkInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf("GetNetworkInfo: %w", err)
	}
	return &types.LightningNetworkInfo{
		NumNodes:             resp.GetNumNodes(),
		NumChannels:          resp.GetNumChannels(),
		TotalNetworkCapacity: resp.GetTotalNetworkCapacity(),
		AvgChannelSize:       resp.GetAvgChannelSize(),
		MedianChannelSize:    resp.GetMedianChannelSizeSat(),
		MinChannelSize:       resp.GetMinChannelSize(),
		MaxChannelSize:       resp.GetMaxChannelSize(),
		GraphDiameter:        resp.GetGraphDiameter(),
		AvgOutDegree:         resp.GetAvgOutDegree(),
	}, nil
}

// describeGraphCache holds a typed snapshot of the per-node aggregates
// derived from dcrlnd's DescribeGraph response. DescribeGraph is a
// heavy call (every node + every edge) so we wrap it in a 10-minute
// in-memory cache. First call after expiry blocks; subsequent calls
// within the window return cached data instantly.
var (
	describeGraphMu   sync.Mutex
	describeGraphData []types.TopLightningNode // sorted by capacity desc
	describeGraphTime time.Time
	describeGraphTTL  = 10 * time.Minute
)

// GetTopLightningNodes returns the top-n nodes by total channel
// capacity. n is capped at len(cached). On cache miss, walks
// DescribeGraph once: edge has node1/node2 pubkeys + capacity, so each
// edge contributes its capacity to BOTH endpoints' totals and counts
// as one channel for each.
func GetTopLightningNodes(ctx context.Context, n int) ([]types.TopLightningNode, error) {
	describeGraphMu.Lock()
	defer describeGraphMu.Unlock()

	if time.Since(describeGraphTime) < describeGraphTTL && describeGraphData != nil {
		return takeTopN(describeGraphData, n), nil
	}

	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	graph, err := rpc.LightningClient.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{IncludeUnannounced: false})
	if err != nil {
		// Keep previous cache if it exists; surface the fresh error.
		return nil, fmt.Errorf("DescribeGraph: %w", err)
	}

	type nodeAgg struct {
		alias    string
		color    string
		channels uint32
		capacity int64
	}
	agg := map[string]*nodeAgg{}
	for _, node := range graph.GetNodes() {
		agg[node.GetPubKey()] = &nodeAgg{
			alias: node.GetAlias(),
			color: node.GetColor(),
		}
	}
	addCapacity := func(pubkey string, capacity int64) {
		if pubkey == "" {
			return
		}
		entry, ok := agg[pubkey]
		if !ok {
			entry = &nodeAgg{}
			agg[pubkey] = entry
		}
		entry.channels++
		entry.capacity += capacity
	}
	for _, edge := range graph.GetEdges() {
		addCapacity(edge.GetNode1Pub(), edge.GetCapacity())
		addCapacity(edge.GetNode2Pub(), edge.GetCapacity())
	}

	out := make([]types.TopLightningNode, 0, len(agg))
	for pubkey, a := range agg {
		out = append(out, types.TopLightningNode{
			Pubkey:        pubkey,
			Alias:         a.alias,
			Color:         a.color,
			NumChannels:   a.channels,
			CapacityAtoms: a.capacity,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CapacityAtoms > out[j].CapacityAtoms
	})

	describeGraphData = out
	describeGraphTime = time.Now()
	return takeTopN(out, n), nil
}

func takeTopN(in []types.TopLightningNode, n int) []types.TopLightningNode {
	if n <= 0 || n > len(in) {
		n = len(in)
	}
	return in[:n]
}

// ---- Send tab ---------------------------------------------------------------

// DecodeLightningInvoice wraps lnrpc.Lightning.DecodePayReq for the Send
// tab's invoice preview. Mirrors Decrediton's decodePayRequest action
// (LNActions.js:683-690). PaymentAddr is converted from raw bytes to
// lowercase hex for display.
func DecodeLightningInvoice(ctx context.Context, payReq string) (*types.LightningDecodedPayReq, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := rpc.LightningClient.DecodePayReq(ctx, &lnrpc.PayReqString{PayReq: payReq})
	if err != nil {
		return nil, fmt.Errorf("DecodePayReq: %w", err)
	}
	out := &types.LightningDecodedPayReq{
		Destination:  resp.GetDestination(),
		PaymentHash:  resp.GetPaymentHash(),
		NumAtoms:     resp.GetNumAtoms(),
		Timestamp:    resp.GetTimestamp(),
		Expiry:       resp.GetExpiry(),
		Description:  resp.GetDescription(),
		FallbackAddr: resp.GetFallbackAddr(),
		CltvExpiry:   resp.GetCltvExpiry(),
	}
	if pa := resp.GetPaymentAddr(); len(pa) > 0 {
		const hexdig = "0123456789abcdef"
		buf := make([]byte, len(pa)*2)
		for i, b := range pa {
			buf[i*2] = hexdig[b>>4]
			buf[i*2+1] = hexdig[b&0x0f]
		}
		out.PaymentAddr = string(buf)
	}
	return out, nil
}

// paymentStatusToString maps lnrpc.Payment.PaymentStatus to the
// frontend-facing label. Mirrors Decrediton's hooks.js logic which
// treats SUCCEEDED as "confirmed", FAILED as "failed", IN_FLIGHT (and
// the legacy UNKNOWN status before the daemon publishes a first
// snapshot) as "pending".
func paymentStatusToString(s lnrpc.Payment_PaymentStatus) string {
	switch s {
	case lnrpc.Payment_SUCCEEDED:
		return "confirmed"
	case lnrpc.Payment_FAILED:
		return "failed"
	default:
		return "pending"
	}
}

// paymentToType maps an lnrpc.Payment snapshot to the flat
// types.LightningPayment used by the Send tab. Limits to the first 3
// HTLC attempts and 5 hops per HTLC to keep the JSON payload bounded.
func paymentToType(p *lnrpc.Payment) types.LightningPayment {
	if p == nil {
		return types.LightningPayment{}
	}
	out := types.LightningPayment{
		PaymentHash:     p.GetPaymentHash(),
		ValueAtoms:      p.GetValueAtoms(),
		FeeAtoms:        p.GetFeeAtoms(),
		CreationDate:    p.GetCreationTimeNs() / 1_000_000_000,
		Status:          paymentStatusToString(p.GetStatus()),
		PaymentPreimage: p.GetPaymentPreimage(),
		PaymentRequest:  p.GetPaymentRequest(),
	}
	if p.GetStatus() == lnrpc.Payment_FAILED {
		out.FailureReason = p.GetFailureReason().String()
	}
	for i, h := range p.GetHtlcs() {
		if i >= 3 {
			break
		}
		htlc := types.LightningHTLC{
			Status: h.GetStatus().String(),
		}
		if route := h.GetRoute(); route != nil {
			htlc.TotalAmt = route.GetTotalAmt()
			htlc.TotalFees = route.GetTotalFees()
			for j, hop := range route.GetHops() {
				if j >= 5 {
					break
				}
				htlc.Hops = append(htlc.Hops, types.LightningHop{
					PubKey:       hop.GetPubKey(),
					FeeAtoms:     hop.GetFee(),
					AmtToForward: hop.GetAmtToForward(),
				})
			}
			// Final-hop destination is implicit in lnrpc.Hop list.
			if last := lastHop(route.GetHops()); last != nil && out.Destination == "" {
				out.Destination = last.GetPubKey()
			}
		}
		out.HTLCs = append(out.HTLCs, htlc)
	}
	return out
}

func lastHop(hops []*lnrpc.Hop) *lnrpc.Hop {
	if len(hops) == 0 {
		return nil
	}
	return hops[len(hops)-1]
}

// defaultRoutingFeeLimitAtoms mirrors dcrlnd's
// lnwallet.DefaultRoutingFeeLimitForAmount: a payment is allowed a 100%
// routing fee up to 1000 atoms, where per-hop base fees dominate, and 5%
// above that. routerrpc.SendPaymentV2 provides no default of its own, so a
// caller that omits a fee limit gets this same curve instead of the
// 0-fee-only behaviour of an unset field.
func defaultRoutingFeeLimitAtoms(amountAtoms int64) int64 {
	if amountAtoms <= 1000 {
		return amountAtoms
	}
	return amountAtoms * 5 / 100
}

// StreamLightningPayment opens Router.SendPaymentV2 and pushes every
// snapshot the daemon emits onto the returned channel. The channel is
// closed when the stream terminates (terminal snapshot) or ctx is
// cancelled. Mirrors Decrediton's handlePaymentStream (LNActions.js:697-732):
// NoInflightUpdates is false on purpose so the UI can render the
// in-flight state, not just the terminal one.
func StreamLightningPayment(ctx context.Context, req *types.LightningSendPaymentRequest) (<-chan types.LightningPayment, error) {
	if rpc.RouterClient == nil {
		return nil, fmt.Errorf("dcrlnd router not available")
	}
	timeout := req.TimeoutSec
	if timeout <= 0 {
		timeout = 60
	}
	feeLimit := req.FeeLimitAtoms
	if feeLimit <= 0 {
		// routerrpc applies no default fee ceiling, and a 0 limit makes
		// dcrlnd reject every fee-bearing route (FAILURE_REASON_NO_ROUTE).
		// When the caller omits the limit, fall back to dcrlnd's own
		// default curve, resolving the amount from the request and, for a
		// normal invoice that carries the value, from the decoded pay req.
		amt := req.Amt
		if amt <= 0 && rpc.LightningClient != nil {
			if dec, err := rpc.LightningClient.DecodePayReq(ctx, &lnrpc.PayReqString{PayReq: req.PayReq}); err == nil {
				amt = dec.NumAtoms
			}
		}
		feeLimit = defaultRoutingFeeLimitAtoms(amt)
	}
	rpcReq := &routerrpc.SendPaymentRequest{
		PaymentRequest:    req.PayReq,
		Amt:               req.Amt,
		FeeLimitAtoms:     feeLimit,
		TimeoutSeconds:    timeout,
		NoInflightUpdates: false,
	}
	stream, err := rpc.RouterClient.SendPaymentV2(ctx, rpcReq)
	if err != nil {
		return nil, fmt.Errorf("SendPaymentV2: %w", err)
	}
	out := make(chan types.LightningPayment, 8)
	go func() {
		defer close(out)
		for {
			snap, err := stream.Recv()
			if err != nil {
				// EOF on terminal snapshot is expected.
				return
			}
			select {
			case out <- paymentToType(snap):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// ListLightningPayments wraps lnrpc.Lightning.ListPayments for the Send
// tab's history list. Reversed: true returns newest-first.
// IncludeIncomplete: true mirrors Decrediton's listLatestPayments
// (LNActions.js) which also fetches pending and failed.
func ListLightningPayments(ctx context.Context) (*types.LightningPaymentList, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := rpc.LightningClient.ListPayments(ctx, &lnrpc.ListPaymentsRequest{
		MaxPayments:       100,
		Reversed:          true,
		IncludeIncomplete: true,
	})
	if err != nil {
		return nil, fmt.Errorf("ListPayments: %w", err)
	}
	out := &types.LightningPaymentList{Payments: make([]types.LightningPayment, 0, len(resp.GetPayments()))}
	for _, p := range resp.GetPayments() {
		out.Payments = append(out.Payments, paymentToType(p))
	}
	return out, nil
}

// ---- Receive tab ----------------------------------------------------------

// invoiceToType maps an lnrpc.Invoice to the flat types.LightningInvoice the
// Receive tab uses. Status is derived in the Decrediton manner: an OPEN
// invoice whose creation+expiry has elapsed collapses to "expired"; a
// CANCELED one past expiry also collapses to "expired" so the UI does not
// need to recompute.
func invoiceToType(inv *lnrpc.Invoice) types.LightningInvoice {
	if inv == nil {
		return types.LightningInvoice{}
	}
	rh := inv.GetRHash()
	const hexdig = "0123456789abcdef"
	rHashHex := make([]byte, len(rh)*2)
	for i, b := range rh {
		rHashHex[i*2] = hexdig[b>>4]
		rHashHex[i*2+1] = hexdig[b&0x0f]
	}
	now := time.Now().Unix()
	state := inv.GetState()
	expiry := inv.GetExpiry()
	creation := inv.GetCreationDate()
	status := "open"
	switch state {
	case lnrpc.Invoice_SETTLED:
		status = "settled"
	case lnrpc.Invoice_CANCELED:
		if creation+expiry < now {
			status = "expired"
		} else {
			status = "canceled"
		}
	case lnrpc.Invoice_OPEN, lnrpc.Invoice_ACCEPTED:
		if creation+expiry < now {
			status = "expired"
		} else {
			status = "open"
		}
	}
	return types.LightningInvoice{
		Memo:           inv.GetMemo(),
		RHashHex:       string(rHashHex),
		PaymentRequest: inv.GetPaymentRequest(),
		ValueAtoms:     inv.GetValue(),
		AmtPaidAtoms:   inv.GetAmtPaidAtoms(),
		CreationDate:   creation,
		SettleDate:     inv.GetSettleDate(),
		Expiry:         expiry,
		AddIndex:       inv.GetAddIndex(),
		SettleIndex:    inv.GetSettleIndex(),
		Private:        inv.GetPrivate(),
		Status:         status,
	}
}

// AddLightningInvoice mints a fresh invoice via lnrpc.AddInvoice, then
// fetches the canonical record via LookupInvoice so the returned object
// has creationDate/expiry/state populated (AddInvoice's response is
// minimal: r_hash, payment_request, add_index).
func AddLightningInvoice(ctx context.Context, req *types.LightningAddInvoiceRequest) (*types.LightningInvoice, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	expiry := req.ExpirySec
	if expiry <= 0 {
		expiry = 3600
	}
	resp, err := rpc.LightningClient.AddInvoice(ctx, &lnrpc.Invoice{
		Memo:   req.Memo,
		Value:  req.ValueAtoms,
		Expiry: expiry,
	})
	if err != nil {
		return nil, fmt.Errorf("AddInvoice: %w", err)
	}
	lookup, err := rpc.LightningClient.LookupInvoice(ctx, &lnrpc.PaymentHash{
		RHash: resp.GetRHash(),
	})
	if err != nil {
		// Fall back to a minimal record so the user still sees the
		// payment request even if the second roundtrip fails.
		const hexdig = "0123456789abcdef"
		rh := resp.GetRHash()
		rHashHex := make([]byte, len(rh)*2)
		for i, b := range rh {
			rHashHex[i*2] = hexdig[b>>4]
			rHashHex[i*2+1] = hexdig[b&0x0f]
		}
		return &types.LightningInvoice{
			Memo:           req.Memo,
			RHashHex:       string(rHashHex),
			PaymentRequest: resp.GetPaymentRequest(),
			ValueAtoms:     req.ValueAtoms,
			CreationDate:   time.Now().Unix(),
			Expiry:         expiry,
			AddIndex:       resp.GetAddIndex(),
			Status:         "open",
		}, nil
	}
	out := invoiceToType(lookup)
	return &out, nil
}

// ListLightningInvoices wraps lnrpc.ListInvoices for the Receive tab's
// history list. Reversed: true returns newest-first.
func ListLightningInvoices(ctx context.Context) (*types.LightningInvoiceList, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := rpc.LightningClient.ListInvoices(ctx, &lnrpc.ListInvoiceRequest{
		NumMaxInvoices: 100,
		Reversed:       true,
	})
	if err != nil {
		return nil, fmt.Errorf("ListInvoices: %w", err)
	}
	out := &types.LightningInvoiceList{Invoices: make([]types.LightningInvoice, 0, len(resp.GetInvoices()))}
	for _, inv := range resp.GetInvoices() {
		out.Invoices = append(out.Invoices, invoiceToType(inv))
	}
	return out, nil
}

// StreamLightningInvoiceEvents opens lnrpc.SubscribeInvoices and pushes
// each snapshot onto the returned channel. Closes on stream end or ctx
// cancel. Mirrors Decrediton's subscribeToInvoices (LNActions.js:620-663).
func StreamLightningInvoiceEvents(ctx context.Context) (<-chan types.LightningInvoice, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	stream, err := rpc.LightningClient.SubscribeInvoices(ctx, &lnrpc.InvoiceSubscription{})
	if err != nil {
		return nil, fmt.Errorf("SubscribeInvoices: %w", err)
	}
	out := make(chan types.LightningInvoice, 16)
	go func() {
		defer close(out)
		for {
			snap, err := stream.Recv()
			if err != nil {
				return
			}
			select {
			case out <- invoiceToType(snap):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// CancelLightningInvoice cancels an OPEN invoice via invoicesrpc. Mirrors
// Decrediton's cancelInvoice (LNActions.js:602-618 via inClient).
func CancelLightningInvoice(ctx context.Context, paymentHashHex string) error {
	if rpc.InvoicesClient == nil {
		return fmt.Errorf("dcrlnd invoices service not available")
	}
	hashBytes, err := hex.DecodeString(strings.TrimSpace(paymentHashHex))
	if err != nil {
		return fmt.Errorf("invalid payment hash: %w", err)
	}
	if len(hashBytes) != 32 {
		return fmt.Errorf("invalid payment hash length: got %d, want 32", len(hashBytes))
	}
	_, err = rpc.InvoicesClient.CancelInvoice(ctx, &invoicesrpc.CancelInvoiceMsg{
		PaymentHash: hashBytes,
	})
	if err != nil {
		return fmt.Errorf("CancelInvoice: %w", err)
	}
	return nil
}

// ---- Advanced tab ---------------------------------------------------------

// ExportLightningChannelBackup wraps lnrpc.ExportAllChannelBackups and
// returns the bytes base64-encoded for browser delivery.
func ExportLightningChannelBackup(ctx context.Context) (*types.LightningChannelBackup, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := rpc.LightningClient.ExportAllChannelBackups(ctx, &lnrpc.ChanBackupExportRequest{})
	if err != nil {
		return nil, fmt.Errorf("ExportAllChannelBackups: %w", err)
	}
	multi := resp.GetMultiChanBackup()
	if multi == nil {
		return &types.LightningChannelBackup{}, nil
	}
	return &types.LightningChannelBackup{
		BackupBase64: base64.StdEncoding.EncodeToString(multi.GetMultiChanBackup()),
		NumChannels:  len(multi.GetChanPoints()),
	}, nil
}

// VerifyLightningChannelBackup decodes the user-uploaded base64 blob and
// calls lnrpc.VerifyChanBackup. Returns an OK/Error pair so the frontend
// can surface validation results inline rather than as a 500 error.
func VerifyLightningChannelBackup(ctx context.Context, b64 string) *types.LightningVerifyBackupResponse {
	if rpc.LightningClient == nil {
		return &types.LightningVerifyBackupResponse{OK: false, Error: "dcrlnd not available"}
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return &types.LightningVerifyBackupResponse{OK: false, Error: "invalid base64 payload"}
	}
	_, err = rpc.LightningClient.VerifyChanBackup(ctx, &lnrpc.ChanBackupSnapshot{
		MultiChanBackup: &lnrpc.MultiChanBackup{MultiChanBackup: raw},
	})
	if err != nil {
		return &types.LightningVerifyBackupResponse{OK: false, Error: err.Error()}
	}
	return &types.LightningVerifyBackupResponse{OK: true}
}

// AddLightningWatchtower registers a watchtower with dcrlnd's wtclient.
func AddLightningWatchtower(ctx context.Context, pubKeyHex, addr string) error {
	if rpc.WatchtowerClient == nil {
		return fmt.Errorf("dcrlnd watchtower client not available")
	}
	pubkey, err := hex.DecodeString(strings.TrimSpace(pubKeyHex))
	if err != nil {
		return fmt.Errorf("invalid pubkey: %w", err)
	}
	if len(pubkey) != 33 {
		return fmt.Errorf("invalid pubkey length: got %d, want 33", len(pubkey))
	}
	_, err = rpc.WatchtowerClient.AddTower(ctx, &wtclientrpc.AddTowerRequest{
		Pubkey:  pubkey,
		Address: strings.TrimSpace(addr),
	})
	if err != nil {
		return fmt.Errorf("AddTower: %w", err)
	}
	return nil
}

// ListLightningWatchtowers maps the wtclient ListTowers response into the
// flat shape the Advanced tab renders.
func ListLightningWatchtowers(ctx context.Context) (*types.LightningWatchtowerList, error) {
	if rpc.WatchtowerClient == nil {
		return nil, fmt.Errorf("dcrlnd watchtower client not available")
	}
	resp, err := rpc.WatchtowerClient.ListTowers(ctx, &wtclientrpc.ListTowersRequest{
		IncludeSessions: true,
	})
	if err != nil {
		return nil, fmt.Errorf("ListTowers: %w", err)
	}
	out := &types.LightningWatchtowerList{
		Towers: make([]types.LightningWatchtower, 0, len(resp.GetTowers())),
	}
	const hexdig = "0123456789abcdef"
	for _, t := range resp.GetTowers() {
		pk := t.GetPubkey()
		pkHex := make([]byte, len(pk)*2)
		for i, b := range pk {
			pkHex[i*2] = hexdig[b>>4]
			pkHex[i*2+1] = hexdig[b&0x0f]
		}
		out.Towers = append(out.Towers, types.LightningWatchtower{
			PubKeyHex:              string(pkHex),
			Addresses:              t.GetAddresses(),
			NumSessions:            t.GetNumSessions(),
			ActiveSessionCandidate: t.GetActiveSessionCandidate(),
		})
	}
	return out, nil
}

// RemoveLightningWatchtower deregisters a watchtower.
func RemoveLightningWatchtower(ctx context.Context, pubKeyHex string) error {
	if rpc.WatchtowerClient == nil {
		return fmt.Errorf("dcrlnd watchtower client not available")
	}
	pubkey, err := hex.DecodeString(strings.TrimSpace(pubKeyHex))
	if err != nil {
		return fmt.Errorf("invalid pubkey: %w", err)
	}
	_, err = rpc.WatchtowerClient.RemoveTower(ctx, &wtclientrpc.RemoveTowerRequest{
		Pubkey: pubkey,
	})
	if err != nil {
		return fmt.Errorf("RemoveTower: %w", err)
	}
	return nil
}

func nodePolicyToType(p *lnrpc.RoutingPolicy) *types.LightningNodePolicy {
	if p == nil {
		return nil
	}
	return &types.LightningNodePolicy{
		Disabled:      p.GetDisabled(),
		TimeLockDelta: p.GetTimeLockDelta(),
		MinHtlcAtoms:  p.GetMinHtlc(),
		MaxHtlcAtoms:  int64(p.GetMaxHtlcMAtoms() / 1000),
		LastUpdate:    p.GetLastUpdate(),
		FeeBaseMAtoms: p.GetFeeBaseMAtoms(),
		FeeRateMAtoms: p.GetFeeRateMilliMAtoms(),
	}
}

// QueryLightningNodeInfo wraps GetNodeInfo with includeChannels=true and
// renders the per-channel policy summaries the Advanced tab displays.
func QueryLightningNodeInfo(ctx context.Context, pubkeyHex string) (*types.LightningNodeInfo, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := rpc.LightningClient.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          strings.TrimSpace(pubkeyHex),
		IncludeChannels: true,
	})
	if err != nil {
		return nil, fmt.Errorf("GetNodeInfo: %w", err)
	}
	node := resp.GetNode()
	out := &types.LightningNodeInfo{
		PubKey:     node.GetPubKey(),
		Alias:      node.GetAlias(),
		Color:      node.GetColor(),
		LastUpdate: node.GetLastUpdate(),
		Channels:   make([]types.LightningNodeChannel, 0, len(resp.GetChannels())),
	}
	var total int64
	for _, c := range resp.GetChannels() {
		total += c.GetCapacity()
		out.Channels = append(out.Channels, types.LightningNodeChannel{
			ChannelID:   c.GetChannelId(),
			ChanPoint:   c.GetChanPoint(),
			Capacity:    c.GetCapacity(),
			LastUpdate:  c.GetLastUpdate(),
			Node1Pubkey: c.GetNode1Pub(),
			Node2Pubkey: c.GetNode2Pub(),
			Node1Policy: nodePolicyToType(c.GetNode1Policy()),
			Node2Policy: nodePolicyToType(c.GetNode2Policy()),
		})
	}
	out.TotalCapacity = total
	return out, nil
}

// QueryLightningRoutes wraps QueryRoutes for the Advanced tab's route
// discovery panel.
func QueryLightningRoutes(ctx context.Context, pubkeyHex string, amtAtoms int64) (*types.LightningQueryRoutesResponse, error) {
	if rpc.LightningClient == nil {
		return nil, fmt.Errorf("dcrlnd not available")
	}
	resp, err := rpc.LightningClient.QueryRoutes(ctx, &lnrpc.QueryRoutesRequest{
		PubKey: strings.TrimSpace(pubkeyHex),
		Amt:    amtAtoms,
	})
	if err != nil {
		return nil, fmt.Errorf("QueryRoutes: %w", err)
	}
	out := &types.LightningQueryRoutesResponse{
		SuccessProb: resp.GetSuccessProb(),
		Routes:      make([]types.LightningRoute, 0, len(resp.GetRoutes())),
	}
	for _, r := range resp.GetRoutes() {
		route := types.LightningRoute{
			TotalAmtAtoms:  r.GetTotalAmt(),
			TotalFeesAtoms: r.GetTotalFees(),
			Hops:           make([]types.LightningRouteHop, 0, len(r.GetHops())),
		}
		for _, h := range r.GetHops() {
			route.Hops = append(route.Hops, types.LightningRouteHop{
				PubKey:       h.GetPubKey(),
				FeeAtoms:     h.GetFee(),
				AmtToForward: h.GetAmtToForward(),
			})
		}
		out.Routes = append(out.Routes, route)
	}
	return out, nil
}
