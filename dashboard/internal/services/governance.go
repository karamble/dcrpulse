// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v4/rpc/walletrpc"
)

// ---- Consensus agendas -----------------------------------------------------

// ListAgendas combines dcrd getvoteinfo (active agendas + choice
// definitions) with the wallet's current VoteChoices to populate
// CurrentChoice per agenda.
func ListAgendas(ctx context.Context) ([]types.Agenda, error) {
	if rpc.DcrdClient == nil || rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("rpc clients not initialized")
	}

	// dcrd getvoteinfo expects the current stake version. We always pass
	// the latest defined version: dcrd handles "future" versions by
	// returning the most recent live deployment. v7 covers the Phase 1
	// agendas (changesubsidysplit, blake3pow, maxtreasuryspend). Hardcode
	// for now; revisit on next consensus upgrade.
	const stakeVersion = 9
	rawVI, err := rpc.DcrdClient.RawRequest(ctx, "getvoteinfo", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("%d", stakeVersion)),
	})
	if err != nil {
		return nil, fmt.Errorf("getvoteinfo: %w", err)
	}
	var vi struct {
		Currentheight int64 `json:"currentheight"`
		Agendas       []struct {
			ID             string `json:"id"`
			Description    string `json:"description"`
			Mask           uint64 `json:"mask"`
			Starttime      int64  `json:"starttime"`
			Expiretime     int64  `json:"expiretime"`
			Status         string `json:"status"`
			Quorumprogress float64 `json:"quorumprogress"`
			Choices        []struct {
				ID          string  `json:"id"`
				Description string  `json:"description"`
				Bits        uint16  `json:"bits"`
				Isabstain   bool    `json:"isabstain"`
				Isno        bool    `json:"isno"`
				Count       uint32  `json:"count"`
				Progress    float64 `json:"progress"`
			} `json:"choices"`
		} `json:"agendas"`
	}
	if err := json.Unmarshal(rawVI, &vi); err != nil {
		return nil, fmt.Errorf("decode getvoteinfo: %w", err)
	}

	// Current choices from the wallet.
	current := map[string]string{}
	if vc, err := rpc.VotingClient.VoteChoices(ctx, &pb.VoteChoicesRequest{}); err == nil {
		for _, c := range vc.GetChoices() {
			current[c.GetAgendaId()] = c.GetChoiceId()
		}
	} else {
		log.Printf("VoteChoices: %v", err)
	}

	out := make([]types.Agenda, 0, len(vi.Agendas))
	for _, a := range vi.Agendas {
		choices := make([]types.AgendaChoice, 0, len(a.Choices))
		for _, c := range a.Choices {
			choices = append(choices, types.AgendaChoice{
				ID:          c.ID,
				Description: c.Description,
				IsAbstain:   c.Isabstain,
				IsNo:        c.Isno,
			})
		}
		out = append(out, types.Agenda{
			ID:            a.ID,
			Description:   a.Description,
			Status:        a.Status,
			Choices:       choices,
			CurrentChoice: current[a.ID],
		})
	}
	return out, nil
}

// SetAgendaChoice updates one agenda's vote preference. The wallet is
// briefly unlocked, the choice is applied, then re-locked.
func SetAgendaChoice(ctx context.Context, agendaID, choiceID string, passphrase []byte) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC unavailable")
	}
	if err := unlockForVote(ctx, passphrase); err != nil {
		return err
	}
	defer lockAfterVote()

	_, err := rpc.VotingClient.SetVoteChoices(ctx, &pb.SetVoteChoicesRequest{
		Choices: []*pb.SetVoteChoicesRequest_Choice{{
			AgendaId: agendaID,
			ChoiceId: choiceID,
		}},
	})
	if err != nil {
		return fmt.Errorf("SetVoteChoices: %w", err)
	}
	return nil
}

// ---- Treasury (PI keys) ----------------------------------------------------

func ListTreasuryKeyPolicies(ctx context.Context) ([]types.TreasuryKeyPolicy, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC unavailable")
	}
	resp, err := rpc.VotingClient.TreasuryPolicies(ctx, &pb.TreasuryPoliciesRequest{})
	if err != nil {
		return nil, fmt.Errorf("TreasuryPolicies: %w", err)
	}
	out := make([]types.TreasuryKeyPolicy, 0, len(resp.GetPolicies()))
	for _, p := range resp.GetPolicies() {
		out = append(out, types.TreasuryKeyPolicy{
			Key:    hex.EncodeToString(p.GetKey()),
			Policy: p.GetPolicy(),
		})
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
	return nil
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
	_, _ = rpc.WalletGrpcClient.LockWallet(lockCtx, &pb.LockWalletRequest{})
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
