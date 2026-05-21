// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"

	"github.com/gorilla/mux"
)

// GetAgendasHandler returns the list of currently-tracked consensus
// agendas joined with the wallet's current choice per agenda.
func GetAgendasHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	agendas, err := services.ListAgendas(ctx)
	if err != nil {
		log.Printf("ListAgendas: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agendas)
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
	pass := zeroOnReturn([]byte(req.Passphrase))
	defer pass.zero()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := services.SetAgendaChoice(ctx, req.AgendaID, req.ChoiceID, pass.b); err != nil {
		writePassphraseAwareError(w, "SetAgendaChoice", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetTreasuryKeyPoliciesHandler returns wallet-wide PI-key policies.
func GetTreasuryKeyPoliciesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	policies, err := services.ListTreasuryKeyPolicies(ctx)
	if err != nil {
		log.Printf("ListTreasuryKeyPolicies: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(policies)
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
	pass := zeroOnReturn([]byte(req.Passphrase))
	defer pass.zero()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := services.SetTreasuryKeyPolicy(ctx, req.Key, req.Policy, pass.b); err != nil {
		writePassphraseAwareError(w, "SetTreasuryKeyPolicy", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetTSpendPoliciesHandler returns the per-TSpend policies.
func GetTSpendPoliciesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	policies, err := services.ListTSpendPolicies(ctx)
	if err != nil {
		log.Printf("ListTSpendPolicies: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(policies)
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
	pass := zeroOnReturn([]byte(req.Passphrase))
	defer pass.zero()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := services.SetTSpendPolicyForHash(ctx, req.Hash, req.Policy, pass.b); err != nil {
		writePassphraseAwareError(w, "SetTSpendPolicy", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetProposalsHandler returns the Politeia proposals list (cached 5 min).
func GetProposalsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	proposals, err := services.ListProposals(ctx)
	if err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		log.Printf("ListProposals: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proposals)
}

// GetProposalDetailHandler returns one proposal's full record.
func GetProposalDetailHandler(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(mux.Vars(r)["token"])
	if token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	detail, err := services.GetProposalDetail(ctx, token)
	if err != nil {
		if errors.Is(err, services.ErrPoliteiaDisabled) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		log.Printf("GetProposalDetail(%s): %v", token, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
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
	pass := zeroOnReturn([]byte(req.Passphrase))
	defer pass.zero()

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	result, err := services.CastPoliteiaVote(ctx, req, pass.b)
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

type passphraseBuf struct {
	b []byte
}

func (p passphraseBuf) zero() {
	for i := range p.b {
		p.b[i] = 0
	}
}

func zeroOnReturn(b []byte) passphraseBuf {
	return passphraseBuf{b: b}
}

func writePassphraseAwareError(w http.ResponseWriter, label string, err error) {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
		http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
	default:
		log.Printf("%s failed: %v", label, err)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}
