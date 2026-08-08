// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"

	"github.com/gorilla/mux"
)

// GetAgendasHandler returns every consensus agenda across all stake versions,
// the current voting window, and the wallet's current choice per agenda.
func GetAgendasHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	agendas, err := services.ListAgendas(ctx)
	if err != nil {
		govnLog.Errorf("ListAgendas: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agendas)
}

// GetAgendaVotesHandler returns one agenda's vote tally over its own voting
// window. The window is read from the chain rather than the request, so the
// fetch is detached from the caller: a client that navigates away must not
// cancel a computation whose result is shared through the cache.
func GetAgendaVotesHandler(w http.ResponseWriter, r *http.Request) {
	agendaID := mux.Vars(r)["id"]
	if agendaID == "" {
		http.Error(w, "agenda id required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	votes, err := services.AgendaVoteHistory(ctx, agendaID)
	if err != nil {
		govnLog.Errorf("AgendaVoteHistory %s: %v", agendaID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(votes)
}

// SetAgendaChoiceHandler updates one agenda's vote preference.
func SetAgendaChoiceHandler(w http.ResponseWriter, r *http.Request) {
	var req types.SetAgendaChoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.AgendaID == "" || req.ChoiceID == "" {
		http.Error(w, "agendaID and choiceID required", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	pass := []byte(req.Passphrase)
	defer zeroBytes(pass)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := services.SetAgendaChoice(ctx, req.AgendaID, req.ChoiceID, pass); err != nil {
		writePassphraseAwareError(w, "SetAgendaChoice", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetTreasuryKeyPoliciesHandler returns wallet-wide PI-key policies.
func GetTreasuryKeyPoliciesHandler(w http.ResponseWriter, r *http.Request) {
	treasuryPolicies(w, r, "ListTreasuryKeyPolicies", func(ctx context.Context) (any, error) {
		return services.ListTreasuryKeyPolicies(ctx)
	})
}

// SetTreasuryKeyPolicyHandler updates one PI-key policy.
func SetTreasuryKeyPolicyHandler(w http.ResponseWriter, r *http.Request) {
	var req types.SetTreasuryKeyPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	pass := []byte(req.Passphrase)
	defer zeroBytes(pass)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := services.SetTreasuryKeyPolicy(ctx, req.Key, req.Policy, pass); err != nil {
		writePassphraseAwareError(w, "SetTreasuryKeyPolicy", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// treasuryPolicies is the shared body of the two policy list reads.
func treasuryPolicies(w http.ResponseWriter, r *http.Request, label string, list func(ctx context.Context) (any, error)) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	policies, err := list(ctx)
	if err != nil {
		govnLog.Errorf("%s: %v", label, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(policies)
}

// GetTSpendPoliciesHandler returns the per-TSpend policies.
func GetTSpendPoliciesHandler(w http.ResponseWriter, r *http.Request) {
	treasuryPolicies(w, r, "ListTSpendPolicies", func(ctx context.Context) (any, error) {
		return services.ListTSpendPolicies(ctx)
	})
}

// SetTSpendPolicyHandler updates one TSpend's policy.
func SetTSpendPolicyHandler(w http.ResponseWriter, r *http.Request) {
	var req types.SetTSpendPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Hash == "" {
		http.Error(w, "hash required", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	pass := []byte(req.Passphrase)
	defer zeroBytes(pass)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := services.SetTSpendPolicyForHash(ctx, req.Hash, req.Policy, pass); err != nil {
		writePassphraseAwareError(w, "SetTSpendPolicy", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeProposalsResponse encodes the proposals envelope (list + last-fetch time
// + when a manual refresh is next allowed) at the given status.
func writeProposalsResponse(w http.ResponseWriter, status int, proposals []types.Proposal, fetchedAt time.Time, hasMore bool) {
	if proposals == nil {
		proposals = []types.Proposal{}
	}
	var fetched, refreshAt int64
	if !fetchedAt.IsZero() {
		fetched = fetchedAt.Unix()
		refreshAt = fetchedAt.Add(services.ProposalsRefreshCooldown).Unix()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(types.ProposalsResponse{
		Proposals:          proposals,
		FetchedAt:          fetched,
		RefreshAvailableAt: refreshAt,
		HasMore:            hasMore,
	})
}

// GetProposalsHandler returns the Politeia proposals list. The list is cached
// indefinitely and auto-fetched once when empty (e.g. after a restart).
func GetProposalsHandler(w http.ResponseWriter, r *http.Request) {
	bucket := strings.TrimSpace(r.URL.Query().Get("status"))
	if bucket == "" {
		bucket = "voting"
	}
	if !services.IsProposalBucket(bucket) {
		http.Error(w, "unknown proposal status", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), services.ProposalsFetchTimeout)
	defer cancel()
	proposals, fetchedAt, err := services.ListProposals(ctx, bucket)
	if err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		govnLog.Errorf("ListProposals: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeProposalsResponse(w, http.StatusOK, proposals, fetchedAt, services.ProposalsHaveMore(bucket))
}

// RefreshProposalsHandler forces a Politeia re-fetch, subject to the refresh
// cooldown. Returns 429 (with the envelope, so the UI can re-sync the
// countdown) while cooling down, and 503 if Politeia is disabled.
func RefreshProposalsHandler(w http.ResponseWriter, r *http.Request) {
	bucket := strings.TrimSpace(r.URL.Query().Get("status"))
	if bucket == "" {
		bucket = "voting"
	}
	if !services.IsProposalBucket(bucket) {
		http.Error(w, "unknown proposal status", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), services.ProposalsFetchTimeout)
	defer cancel()
	proposals, fetchedAt, err := services.RefreshProposals(ctx, bucket)
	if err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, services.ErrProposalsRefreshCoolingDown) {
			writeProposalsResponse(w, http.StatusTooManyRequests, proposals, fetchedAt, services.ProposalsHaveMore(bucket))
			return
		}
		govnLog.Errorf("RefreshProposals: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeProposalsResponse(w, http.StatusOK, proposals, fetchedAt, services.ProposalsHaveMore(bucket))
}

// LoadMoreProposalsHandler appends the next inventory page of each of the
// bucket's vote statuses to the cached list. Politeia serves at most one page
// per status, so without this the list silently ends at the first page.
func LoadMoreProposalsHandler(w http.ResponseWriter, r *http.Request) {
	bucket := strings.TrimSpace(r.URL.Query().Get("status"))
	if bucket == "" {
		bucket = "voting"
	}
	if !services.IsProposalBucket(bucket) {
		http.Error(w, "unknown proposal status", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), services.ProposalsFetchTimeout)
	defer cancel()
	proposals, fetchedAt, err := services.LoadMoreProposals(ctx, bucket)
	if err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		govnLog.Errorf("LoadMoreProposals: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeProposalsResponse(w, http.StatusOK, proposals, fetchedAt, services.ProposalsHaveMore(bucket))
}

// writeProposalDetailResponse encodes the proposal-detail envelope (record +
// last-fetch time + when a manual refresh is next allowed) at the given status.
func writeProposalDetailResponse(w http.ResponseWriter, status int, detail *types.ProposalDetail, fetchedAt time.Time) {
	var fetched, refreshAt int64
	if !fetchedAt.IsZero() {
		fetched = fetchedAt.Unix()
		refreshAt = fetchedAt.Add(services.ProposalsRefreshCooldown).Unix()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(types.ProposalDetailResponse{
		Detail:             detail,
		FetchedAt:          fetched,
		RefreshAvailableAt: refreshAt,
	})
}

// GetProposalDetailHandler returns one proposal's full record. Cached
// indefinitely per token and auto-fetched on first access.
func GetProposalDetailHandler(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(mux.Vars(r)["token"])
	if token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), services.ProposalsFetchTimeout)
	defer cancel()
	detail, fetchedAt, err := services.GetProposalDetail(ctx, token)
	if err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		govnLog.Errorf("GetProposalDetail(%s): %v", token, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeProposalDetailResponse(w, http.StatusOK, detail, fetchedAt)
}

// RefreshProposalDetailHandler forces a re-fetch of one proposal's detail,
// subject to the refresh cooldown. Returns 429 (with the envelope) while
// cooling down, and 503 if Politeia is disabled.
func RefreshProposalDetailHandler(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(mux.Vars(r)["token"])
	if token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), services.ProposalsFetchTimeout)
	defer cancel()
	detail, fetchedAt, err := services.RefreshProposalDetail(ctx, token)
	if err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, services.ErrProposalsRefreshCoolingDown) {
			writeProposalDetailResponse(w, http.StatusTooManyRequests, detail, fetchedAt)
			return
		}
		govnLog.Errorf("RefreshProposalDetail(%s): %v", token, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeProposalDetailResponse(w, http.StatusOK, detail, fetchedAt)
}

// PrepareProposalVoteHandler computes the wallet's vote eligibility for a
// proposal (owned-ticket count, options, already-voted state) on demand when
// the user opens the vote modal. This is where the heavy eligible-ticket
// snapshot work runs, never on a plain detail-page view.
func PrepareProposalVoteHandler(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(mux.Vars(r)["token"])
	if token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), services.ProposalsFetchTimeout)
	defer cancel()
	elig, err := services.PrepareProposalVote(ctx, token)
	if err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		govnLog.Errorf("PrepareProposalVote(%s): %v", token, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(elig)
}

// CastPoliteiaVoteHandler runs the sign + ballot-cast flow.
func CastPoliteiaVoteHandler(w http.ResponseWriter, r *http.Request) {
	var req types.CastPoliteiaVoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Token == "" || req.VoteOption == "" {
		http.Error(w, "token and voteOption required", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	pass := []byte(req.Passphrase)
	defer zeroBytes(pass)

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	result, err := services.CastPoliteiaVote(ctx, req, pass)
	if err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		writePassphraseAwareError(w, "CastPoliteiaVote", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ---- shared local helpers --------------------------------------------------

func writePassphraseAwareError(w http.ResponseWriter, label string, err error) {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
		http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
	default:
		govnLog.Errorf("%s failed: %v", label, err)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}
