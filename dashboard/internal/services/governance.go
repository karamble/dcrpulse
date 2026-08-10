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

// SetAgendaChoice updates one agenda's vote preference. The wallet is
// briefly unlocked, the choice is applied, then re-locked.
func SetAgendaChoice(ctx context.Context, agendaID, choiceID string, passphrase []byte) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC unavailable")
	}
	if rpc.DcrdClient == nil {
		return fmt.Errorf("rpc clients not initialized")
	}
	// A settled agenda is rejected before the wallet is ever unlocked.
	status, err := agendaStatus(ctx, agendaID)
	if err != nil {
		return err
	}
	if !agendaVotable(status) {
		return fmt.Errorf("%w (status %q)", ErrAgendaVoteSettled, status)
	}
	if err := unlockForVote(ctx, passphrase); err != nil {
		return err
	}
	defer lockAfterVote()

	_, err = rpc.VotingClient.SetVoteChoices(ctx, &pb.SetVoteChoicesRequest{
		Choices: []*pb.SetVoteChoicesRequest_Choice{{
			AgendaId: agendaID,
			ChoiceId: choiceID,
		}},
	})
	if err != nil {
		return fmt.Errorf("SetVoteChoices: %w", err)
	}
	syncVoteChoicesToVSP(ctx)
	return nil
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

func SetTreasuryKeyPolicy(ctx context.Context, keyHex, policy string, passphrase []byte) error {
	key, err := hex.DecodeString(strings.TrimSpace(keyHex))
	if err != nil {
		return fmt.Errorf("invalid key hex: %w", err)
	}
	if err := validatePolicy(policy); err != nil {
		return err
	}
	if err := unlockForVote(ctx, passphrase); err != nil {
		return err
	}
	defer lockAfterVote()

	_, err = rpc.VotingClient.SetTreasuryPolicy(ctx, &pb.SetTreasuryPolicyRequest{
		Key:    key,
		Policy: policy,
	})
	if err != nil {
		return fmt.Errorf("SetTreasuryPolicy: %w", err)
	}
	syncVoteChoicesToVSP(ctx)
	return nil
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

func SetTSpendPolicyForHash(ctx context.Context, hashHex, policy string, passphrase []byte) error {
	hashBytes, err := hex.DecodeString(strings.TrimSpace(hashHex))
	if err != nil {
		return fmt.Errorf("invalid hash hex: %w", err)
	}
	// dcrwallet expects little-endian byte order for hashes over the wire.
	hashBytes = reversed(hashBytes)
	if err := validatePolicy(policy); err != nil {
		return err
	}
	if err := unlockForVote(ctx, passphrase); err != nil {
		return err
	}
	defer lockAfterVote()

	_, err = rpc.VotingClient.SetTSpendPolicy(ctx, &pb.SetTSpendPolicyRequest{
		Hash:   hashBytes,
		Policy: policy,
	})
	if err != nil {
		return fmt.Errorf("SetTSpendPolicy: %w", err)
	}
	syncVoteChoicesToVSP(ctx)
	return nil
}

// ---- VSP sync --------------------------------------------------------------

// syncVoteChoicesToVSP pushes the wallet's current vote/treasury/tspend
// choices to every VSP we have tickets with. Mirrors Decrediton's
// setVSPDVoteChoices (actions/VSPActions.js:470). dcrwallet handles the
// signing + HTTP per-ticket internally; we just supply (host, pubkey,
// fee_account, change_account) per VSP.
//
// Logged-and-swallowed: a VSP being briefly unreachable shouldn't roll
// back a successful local policy change. Decrediton takes the same
// SETVSPDVOTECHOICE_PARTIAL_SUCCESS path.
func syncVoteChoicesToVSP(ctx context.Context) {
	network, err := CurrentNetwork(ctx)
	if err != nil {
		govnLog.Warnf("VSP sync: resolve network: %v", err)
		return
	}
	wc, err := config.LoadWalletCfg(network, CurrentWalletName())
	if err != nil {
		govnLog.Warnf("VSP sync: load wallet cfg: %v", err)
		return
	}
	used, err := wc.UsedVSPs()
	if err != nil {
		govnLog.Warnf("VSP sync: list used VSPs: %v", err)
		return
	}
	if len(used) == 0 {
		return // nothing to do — wallet has never delegated to a VSP
	}
	for host, meta := range used {
		if meta.Pubkey == "" {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err := rpc.WalletGrpcClient.SetVspdVoteChoices(callCtx, &pb.SetVspdVoteChoicesRequest{
			VspHost:       host,
			VspPubkey:     meta.Pubkey,
			FeeAccount:    0,
			ChangeAccount: 0,
		})
		cancel()
		if err != nil {
			govnLog.Warnf("VSP sync %s: %v", host, err)
		}
	}
}

// ---- Shared helpers --------------------------------------------------------

func validatePolicy(p string) error {
	switch p {
	case "yes", "no", "abstain", "invalid":
		return nil
	}
	return fmt.Errorf("policy must be yes|no|abstain (got %q)", p)
}

// unlockForVote performs a full-wallet unlock for voting operations. The
// wallet is locked again via lockAfterVote in the caller's defer.
func unlockForVote(ctx context.Context, passphrase []byte) error {
	unlockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := rpc.WalletGrpcClient.UnlockWallet(unlockCtx, &pb.UnlockWalletRequest{
		Passphrase: passphrase,
	})
	if err != nil {
		return fmt.Errorf("unlock wallet: %w", err)
	}
	return nil
}

func lockAfterVote() {
	lockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// A failed lock leaves the wallet's keys usable, so say so rather than
	// dropping it.
	if _, err := rpc.WalletGrpcClient.LockWallet(lockCtx, &pb.LockWalletRequest{}); err != nil {
		govnLog.Errorf("lock wallet after vote: %v", err)
	}
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
