// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrjson/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// ---- Consensus agendas -----------------------------------------------------

// voteVersionsNewestFirst returns every stake version the network defines
// deployments for, highest first. dcrd answers getvoteinfo for exactly one
// version and rejects anything it has no deployments for, and no RPC reports
// which versions a node accepts, so the set has to come from chaincfg.
func voteVersionsNewestFirst(params *chaincfg.Params) []uint32 {
	versions := make([]uint32, 0, len(params.Deployments))
	for v := range params.Deployments {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] > versions[j] })
	return versions
}

// agendaVoteWindow derives the blocks an agenda was voted over from the height
// it entered its current state. dcrd tallies a window [X-RCI+1, X] and applies
// the result to [X+1, X+RCI], so Since (always the first block of the interval
// the state took effect in) walks straight back to the vote.
//
// A settled agenda is the only reason this exists: dcrd reports per-choice
// counts solely while an agenda is "started".
func agendaVoteWindow(status string, since int64, params *chaincfg.Params) (start, end int64, ok bool) {
	rci := int64(params.RuleChangeActivationInterval)
	// Since is absent for a "defined" agenda, and short-circuits to 1 for a
	// forced choice on the test networks. Neither has a real window.
	if since <= 1 || rci <= 0 {
		return 0, 0, false
	}

	switch status {
	case "active":
		end = since - rci - 1
	case "lockedin", "failed":
		end = since - 1
	default:
		// "started" is tallied live by dcrd, "defined" never voted.
		return 0, 0, false
	}
	start = end - rci + 1

	// Windows sit on the interval grid anchored at StakeValidationHeight.
	// Anything off it means the assumptions above did not hold.
	svh := params.StakeValidationHeight
	if start < svh || (start-svh)%rci != 0 {
		return 0, 0, false
	}
	return start, end, true
}

// ErrAgendaVoteSettled rejects a preference change for an agenda whose vote
// has already concluded.
var ErrAgendaVoteSettled = fmt.Errorf("agenda vote has settled; the choice can no longer be changed")

// agendaVotable reports whether a preference may still be changed. dcrd
// reports exactly one of defined|started|lockedin|active|failed; only the
// first two precede or overlap the vote, so anything else fails closed.
func agendaVotable(status string) bool {
	return status == "defined" || status == "started"
}

// agendaStatus finds the status dcrd reports for one agenda, newest vote
// version first.
func agendaStatus(ctx context.Context, agendaID string) (string, error) {
	params, err := chainParams(ctx)
	if err != nil {
		return "", err
	}
	for _, v := range voteVersionsNewestFirst(params) {
		vi, err := rpc.DcrdClient.GetVoteInfo(ctx, v)
		if err != nil {
			var rpcErr *dcrjson.RPCError
			if errors.As(err, &rpcErr) && rpcErr.Code == dcrjson.ErrRPCInvalidParameter {
				continue
			}
			return "", fmt.Errorf("getvoteinfo %d: %w", v, err)
		}
		for _, a := range vi.Agendas {
			if a.ID == agendaID {
				return a.Status, nil
			}
		}
	}
	return "", fmt.Errorf("unknown agenda %q", agendaID)
}

// agendaTally counts vote bits for one agenda. Votes whose bits match no
// declared choice are reported as abstain, matching dcrd's own reporting path,
// and counted separately so an unexpected number stays visible.
type agendaTally struct {
	byChoice  map[string]int64
	unmatched int64
	total     int64
}

// countAgendaVotes classifies raw vote bits for a single agenda. Every agenda of
// a vote version shares one 16-bit field, so the agenda's mask isolates its own
// choice before comparison.
func countAgendaVotes(bits []uint16, mask uint16, choices []chainjson.Choice) agendaTally {
	t := agendaTally{byChoice: make(map[string]int64, len(choices)), total: int64(len(bits))}
	for _, b := range bits {
		sel := b & mask
		matched := false
		for _, c := range choices {
			if sel == c.Bits {
				t.byChoice[c.ID]++
				matched = true
				break
			}
		}
		if !matched {
			t.unmatched++
			for _, c := range choices {
				if c.IsAbstain {
					t.byChoice[c.ID]++
					break
				}
			}
		}
	}
	return t
}

// ListAgendas returns every consensus agenda the connected node knows, across
// all stake versions, joined with the wallet's current choice per agenda.
func ListAgendas(ctx context.Context) (*types.ConsensusVoteInfo, error) {
	if rpc.DcrdClient == nil || rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("rpc clients not initialized")
	}

	params, err := chainParams(ctx)
	if err != nil {
		return nil, err
	}

	// Current choices from the wallet. Keyed by agenda ID, so the stake
	// version an agenda belongs to cannot desync them.
	current := map[string]string{}
	if vc, err := rpc.VotingClient.VoteChoices(ctx, &pb.VoteChoicesRequest{}); err == nil {
		for _, c := range vc.GetChoices() {
			current[c.GetAgendaId()] = c.GetChoiceId()
		}
	} else {
		govnLog.Warnf("VoteChoices: %v", err)
	}

	// getvoteinfo reports no history once an agenda has settled, but
	// getblockchaininfo carries the height each one entered its current state,
	// which is what the voting window is derived from.
	since := map[string]int64{}
	if bci, err := rpc.DcrdClient.GetBlockChainInfo(ctx); err == nil {
		for id, d := range bci.Deployments {
			since[id] = d.Since
		}
	} else {
		govnLog.Warnf("GetBlockChainInfo for agenda history: %v", err)
	}

	var (
		out                []types.Agenda
		newest             *chainjson.GetVoteInfoResult
		voteInProgress     bool
		recognisedVersions int
	)
	for _, v := range voteVersionsNewestFirst(params) {
		vi, err := rpc.DcrdClient.GetVoteInfo(ctx, v)
		if err != nil {
			var rpcErr *dcrjson.RPCError
			if errors.As(err, &rpcErr) && rpcErr.Code == dcrjson.ErrRPCInvalidParameter {
				// A version this build knows but the node does not.
				continue
			}
			return nil, fmt.Errorf("getvoteinfo %d: %w", v, err)
		}
		recognisedVersions++
		if newest == nil {
			newest = vi
		}

		for _, a := range vi.Agendas {
			if a.Status == "started" {
				voteInProgress = true
			}
			choices := make([]types.AgendaChoice, 0, len(a.Choices))
			for _, c := range a.Choices {
				choices = append(choices, types.AgendaChoice{
					ID:          c.ID,
					Description: c.Description,
					IsAbstain:   c.IsAbstain,
					IsNo:        c.IsNo,
					Count:       c.Count,
					Progress:    c.Progress,
				})
			}
			start, end, _ := agendaVoteWindow(a.Status, since[a.ID], params)
			out = append(out, types.Agenda{
				ID:              a.ID,
				Description:     a.Description,
				Status:          a.Status,
				VoteVersion:     vi.VoteVersion,
				VoteStartHeight: start,
				VoteEndHeight:   end,
				StartTime:       int64(a.StartTime),
				ExpireTime:      int64(a.ExpireTime),
				Since:           since[a.ID],
				QuorumProgress:  a.QuorumProgress,
				Choices:         choices,
				CurrentChoice:   current[a.ID],
			})
		}
	}
	if recognisedVersions == 0 {
		return nil, fmt.Errorf("no stake version recognised by dcrd")
	}

	return &types.ConsensusVoteInfo{
		CurrentVoteVersion: newest.VoteVersion,
		VoteInProgress:     voteInProgress,
		Quorum:             newest.Quorum,
		TotalVotes:         newest.TotalVotes,
		CurrentHeight:      newest.CurrentHeight,
		WindowStartHeight:  newest.StartHeight,
		WindowEndHeight:    newest.EndHeight,
		Agendas:            out,
	}, nil
}

// SetAgendaChoice updates one agenda's vote preference. The local write needs
// no keys, so the wallet stays locked for it; only the VSP sync afterwards
// opens accounts, and only for its own window.
func SetAgendaChoice(ctx context.Context, agendaID, choiceID string, passphrase []byte) (*types.VSPSyncSummary, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC unavailable")
	}
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("rpc clients not initialized")
	}
	// A settled agenda is rejected before the passphrase is even checked.
	status, err := agendaStatus(ctx, agendaID)
	if err != nil {
		return nil, err
	}
	if !agendaVotable(status) {
		return nil, fmt.Errorf("%w (status %q)", ErrAgendaVoteSettled, status)
	}
	if err := verifyGovernancePassphrase(ctx, passphrase); err != nil {
		return nil, err
	}
	_, err = rpc.VotingClient.SetVoteChoices(ctx, &pb.SetVoteChoicesRequest{
		Choices: []*pb.SetVoteChoicesRequest_Choice{{
			AgendaId: agendaID,
			ChoiceId: choiceID,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("SetVoteChoices: %w", err)
	}
	return syncVoteChoicesToVSPs(ctx, passphrase), nil
}

// ---- Treasury (PI keys) ----------------------------------------------------

// sanctionedPiKeys returns the consensus-sanctioned Politeia key for the
// current network as a lowercase hex string, sourced from dcrd's chaincfg.
// Only the first key is exposed, matching Decrediton's
// app/constants/decred.js:75-78 where the second mainnet key sits
// commented out.
func sanctionedPiKeys(ctx context.Context) ([]string, error) {
	params, err := chainParams(ctx)
	if err != nil {
		return nil, err
	}
	if len(params.PiKeys) == 0 {
		return nil, nil
	}
	return []string{hex.EncodeToString(params.PiKeys[0])}, nil
}

func ListTreasuryKeyPolicies(ctx context.Context) ([]types.TreasuryKeyPolicy, error) {
	// Sanctioned keys are wallet-independent — we can always show them, even
	// before the wallet's VotingService finishes initializing. Stored
	// policies are best-effort: a transient gRPC failure should not hide
	// the consensus key the user is here to vote on.
	sanctioned, err := sanctionedPiKeys(ctx)
	if err != nil {
		return nil, err
	}
	stored := map[string]string{}
	if rpc.WalletGrpcClient != nil && rpc.VotingClient != nil {
		if resp, err := rpc.VotingClient.TreasuryPolicies(ctx, &pb.TreasuryPoliciesRequest{}); err == nil {
			for _, p := range resp.GetPolicies() {
				stored[hex.EncodeToString(p.GetKey())] = p.GetPolicy()
			}
		} else {
			govnLog.Warnf("TreasuryPolicies: %v", err)
		}
	}

	out := make([]types.TreasuryKeyPolicy, 0, len(sanctioned)+len(stored))
	seen := make(map[string]struct{}, len(sanctioned))
	for _, k := range sanctioned {
		out = append(out, types.TreasuryKeyPolicy{Key: k, Policy: stored[k]})
		seen[k] = struct{}{}
	}
	for k, pol := range stored {
		if _, ok := seen[k]; ok {
			continue
		}
		out = append(out, types.TreasuryKeyPolicy{Key: k, Policy: pol})
	}
	return out, nil
}

func SetTreasuryKeyPolicy(ctx context.Context, keyHex, policy string, passphrase []byte) (*types.VSPSyncSummary, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC unavailable")
	}
	key, err := hex.DecodeString(strings.TrimSpace(keyHex))
	if err != nil {
		return nil, fmt.Errorf("invalid key hex: %w", err)
	}
	if err := validatePolicy(policy); err != nil {
		return nil, err
	}
	if err := verifyGovernancePassphrase(ctx, passphrase); err != nil {
		return nil, err
	}
	_, err = rpc.VotingClient.SetTreasuryPolicy(ctx, &pb.SetTreasuryPolicyRequest{
		Key:    key,
		Policy: policy,
	})
	if err != nil {
		return nil, fmt.Errorf("SetTreasuryPolicy: %w", err)
	}
	return syncVoteChoicesToVSPs(ctx, passphrase), nil
}

// ---- Treasury (per-TSpend hash) -------------------------------------------

func ListTSpendPolicies(ctx context.Context) ([]types.TSpendPolicy, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC unavailable")
	}
	resp, err := rpc.VotingClient.TSpendPolicies(ctx, &pb.TSpendPoliciesRequest{})
	if err != nil {
		return nil, fmt.Errorf("TSpendPolicies: %w", err)
	}
	out := make([]types.TSpendPolicy, 0, len(resp.GetPolicies()))
	for _, p := range resp.GetPolicies() {
		out = append(out, types.TSpendPolicy{
			Hash:   hex.EncodeToString(reversed(p.GetHash())),
			Policy: p.GetPolicy(),
		})
	}
	return out, nil
}

func SetTSpendPolicyForHash(ctx context.Context, hashHex, policy string, passphrase []byte) (*types.VSPSyncSummary, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC unavailable")
	}
	hashBytes, err := hex.DecodeString(strings.TrimSpace(hashHex))
	if err != nil {
		return nil, fmt.Errorf("invalid hash hex: %w", err)
	}
	// dcrwallet expects little-endian byte order for hashes over the wire.
	hashBytes = reversed(hashBytes)
	if err := validatePolicy(policy); err != nil {
		return nil, err
	}
	if err := verifyGovernancePassphrase(ctx, passphrase); err != nil {
		return nil, err
	}
	_, err = rpc.VotingClient.SetTSpendPolicy(ctx, &pb.SetTSpendPolicyRequest{
		Hash:   hashBytes,
		Policy: policy,
	})
	if err != nil {
		return nil, fmt.Errorf("SetTSpendPolicy: %w", err)
	}
	return syncVoteChoicesToVSPs(ctx, passphrase), nil
}

// ---- VSP sync --------------------------------------------------------------

// verifyGovernancePassphrase checks the passphrase against the default account
// while leaving every lock exactly as found, so a wrong passphrase is a clean
// 401 before any policy is written and the wallet stays locked for the local
// write, which needs no keys at all.
func verifyGovernancePassphrase(ctx context.Context, passphrase []byte) error {
	return verifyAccountPassphrase(ctx, 0, passphrase)
}

// syncVoteChoicesToVSPs pushes the wallet's current vote/treasury/tspend
// choices to every VSP that actually holds one of this wallet's tickets.
// dcrwallet compares the request host EXACT-STRING against each ticket's
// stored purchase host, so the hosts are taken verbatim from the wallet's own
// ticket records - never constructed here. dcrwallet signs each ticket's vspd
// request with its commitment key, which lives in a per-account-encrypted
// account, so the accounts are unlocked for exactly this window and re-locked
// by defer.
//
// The local policy is already saved when this runs; per-host failures are
// returned for the caller to surface, never rolled back (Decrediton behaves
// the same way).
//
// Known upstream quirks tolerated here: any solo (non-VSP) live ticket makes
// the RPC return a NotExist "no VSP info" error even on success, filtered
// below; abstain tspends go out as ""; vspd rejects requests carrying more
// than 3 tspend or treasury policies.
func syncVoteChoicesToVSPs(ctx context.Context, passphrase []byte) *types.VSPSyncSummary {
	sum := &types.VSPSyncSummary{}
	fail := func(host, msg string) {
		govnLog.Warnf("VSP sync %s: %s", host, msg)
		sum.Failed = append(sum.Failed, types.VSPSyncFailure{Host: host, Error: msg})
	}

	tickets, err := ListTickets(ctx)
	if err != nil {
		fail("", "list tickets: "+err.Error())
		return sum
	}
	hosts := vspHostsFromTickets(tickets)
	sum.Hosts = len(hosts)
	if len(hosts) == 0 {
		return sum // no VSP tickets - nothing to push
	}

	// Pubkeys come from the used-VSP metadata (stored bare-host, so the match
	// ignores the scheme), else a live probe of the VSP itself.
	used := map[string]config.VSPMetadata{}
	if network, err := CurrentNetwork(ctx); err == nil {
		if wc, err := config.LoadWalletCfg(network, CurrentWalletName()); err == nil {
			if u, err := wc.UsedVSPs(); err == nil {
				used = u
			}
		}
	}
	pubkeys := make(map[string]string, len(hosts))
	for _, host := range hosts {
		pubkey := pubkeyForVSPHost(used, host)
		if pubkey == "" {
			if info, err := GetVSPInfo(ctx, host); err == nil {
				pubkey = info.PubKey
			}
		}
		if pubkey == "" {
			fail(host, "no pubkey known for this VSP")
			continue
		}
		pubkeys[host] = pubkey
	}
	if len(pubkeys) == 0 {
		return sum // nothing reachable - no reason to open any account
	}

	// dcrwallet caches a VSP client per host on first use and keeps its fee
	// policy for later fee payments, so pass the same accounts a purchase
	// would rather than poisoning the cache with 0/0 on a mixing wallet.
	var feeAcct, changeAcct uint32
	if mix, ok := TicketMixingParams(ctx); ok {
		feeAcct, changeAcct = mix.Mixed, mix.Change
	}

	pushVoteChoicesToVSPs(ctx, hosts, pubkeys, feeAcct, changeAcct, passphrase, fail)
	return sum
}

// pushVoteChoicesToVSPs runs the unlock -> per-host SetVspdVoteChoices ->
// relock window. One unlock window for the whole fan-out, the
// buildSignedVotes shape: the mutex is registered before the relock defer so,
// LIFO, the relock completes before the lock releases and a concurrent
// signing flow can never re-lock accounts this one is still using.
func pushVoteChoicesToVSPs(ctx context.Context, hosts []string, pubkeys map[string]string,
	feeAcct, changeAcct uint32, passphrase []byte, fail func(host, msg string)) {

	vspSignMu.Lock()
	defer vspSignMu.Unlock()
	beginUnlockedOp()
	defer endUnlockedOp()
	unlocked, err := unlockAllAccountsForSpend(ctx, passphrase)
	if err != nil {
		fail("", "unlock accounts: "+err.Error())
		return
	}
	defer relockAccountsAfterVSP(unlocked)

	for _, host := range hosts {
		pubkey, ok := pubkeys[host]
		if !ok {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err := rpc.WalletGrpcClient.SetVspdVoteChoices(callCtx, &pb.SetVspdVoteChoicesRequest{
			VspHost:       host,
			VspPubkey:     pubkey,
			FeeAccount:    feeAcct,
			ChangeAccount: changeAcct,
		})
		cancel()
		if err != nil {
			if msg := filterSoloTicketNoise(err.Error()); msg != "" {
				fail(host, msg)
			}
		}
	}
}

// vspHostsFromTickets returns the distinct VSP hosts holding this wallet's
// votable tickets, exactly as the wallet stored them. Only statuses dcrwallet's
// own sync iterates (unmined, immature, live) count; solo tickets have no host
// and are skipped.
func vspHostsFromTickets(tickets []types.TicketRecord) []string {
	seen := map[string]struct{}{}
	var hosts []string
	for _, t := range tickets {
		switch t.Status {
		case "UNMINED", "IMMATURE", "LIVE":
		default:
			continue
		}
		if t.VSPHost == "" {
			continue
		}
		if _, ok := seen[t.VSPHost]; ok {
			continue
		}
		seen[t.VSPHost] = struct{}{}
		hosts = append(hosts, t.VSPHost)
	}
	sort.Strings(hosts)
	return hosts
}

// pubkeyForVSPHost finds the stored pubkey for a ticket's VSP host. used_vsps
// keys are bare hosts (Decrediton config compatibility) while ticket hosts
// carry their scheme, so the comparison strips schemes but never rewrites the
// host that goes on the wire.
func pubkeyForVSPHost(used map[string]config.VSPMetadata, host string) string {
	want := strings.TrimSuffix(stripVSPScheme(host), "/")
	for key, meta := range used {
		if strings.EqualFold(strings.TrimSuffix(stripVSPScheme(key), "/"), want) {
			return meta.Pubkey
		}
	}
	return ""
}

func stripVSPScheme(host string) string {
	return strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
}

// filterSoloTicketNoise drops the per-ticket "no VSP info" errors dcrwallet's
// gRPC path reports for solo tickets (the JSON-RPC path swallows them) and
// returns whatever real failure text remains, or "" when the error was only
// that noise.
func filterSoloTicketNoise(msg string) string {
	var kept []string
	for _, line := range strings.Split(msg, "\n") {
		if strings.Contains(line, "no VSP info for ticket") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
	}
	joined := strings.Join(kept, "\n")
	// The aggregate prefix alone carries no failure.
	if strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(joined), "ForUnspentUnexpiredTickets failed. Error:")) == "" {
		return ""
	}
	return joined
}

// ---- Shared helpers --------------------------------------------------------

func validatePolicy(p string) error {
	switch p {
	case "yes", "no", "abstain", "invalid":
		return nil
	}
	return fmt.Errorf("policy must be yes|no|abstain (got %q)", p)
}

// reversed returns a copy of b with the byte order reversed. Hashes
// shuttle through dcrwallet's gRPC in little-endian while the rest of
// the codebase (and the UI) uses big-endian display hex.
func reversed(b []byte) []byte {
	out := make([]byte, len(b))
	for i, v := range b {
		out[len(b)-1-i] = v
	}
	return out
}
