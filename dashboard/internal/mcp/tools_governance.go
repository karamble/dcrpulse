// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

type setVoteChoiceInput struct {
	AgendaID string `json:"agendaId" jsonschema:"consensus agenda id"`
	ChoiceID string `json:"choiceId" jsonschema:"choice id (e.g. yes, no, abstain)"`
}

type castVoteInput struct {
	Token      string `json:"token" jsonschema:"Politeia proposal token"`
	VoteOption string `json:"voteOption" jsonschema:"vote option id for the proposal"`
}

type treasuryPolicyInput struct {
	Key    string `json:"key" jsonschema:"treasury (Pi) public key, hex"`
	Policy string `json:"policy" jsonschema:"policy: yes, no, or abstain"`
}

type tspendPolicyInput struct {
	Hash   string `json:"hash" jsonschema:"tspend transaction hash, hex"`
	Policy string `json:"policy" jsonschema:"policy: yes, no, or abstain"`
}

// governanceTools are the governance domain tools: read agendas/policies/
// proposals, plus voting writes gated on a grant with voting enabled.
var governanceTools = []toolDef{
	readTool("governance", "governance_agendas",
		"List consensus voting agendas and this wallet's current vote choices.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListAgendas(ctx) }),
	readTool("governance", "governance_treasury_policies",
		"List the wallet's treasury key (Pi key) voting policies.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListTreasuryKeyPolicies(ctx) }),
	readTool("governance", "governance_tspend_policies",
		"List the wallet's treasury-spend (TSpend) voting policies.",
		func(ctx context.Context, _ emptyInput) (any, error) { return services.ListTSpendPolicies(ctx) }),
	readTool("governance", "governance_proposals",
		"List Politeia governance proposals.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			proposals, _, err := services.ListProposals(ctx)
			return proposals, err
		}),
	agentTool("governance", "governance_set_vote_choice",
		"Set this wallet's vote choice for a consensus agenda. Requires a spend grant with voting enabled; signs with the held passphrase.",
		func(ctx context.Context, a *agent, in setVoteChoiceInput) (any, error) {
			pass, err := grants.authorizeVoting(a.id, time.Now())
			if err != nil {
				recordSpend(a, "governance_set_vote_choice", 0, 0, in.AgendaID, "denied", err.Error())
				return nil, err
			}
			defer zero(pass)
			if err := services.SetAgendaChoice(ctx, in.AgendaID, in.ChoiceID, pass); err != nil {
				recordSpend(a, "governance_set_vote_choice", 0, 0, in.AgendaID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "governance_set_vote_choice", 0, 0, in.AgendaID, "ok", in.ChoiceID)
			return map[string]any{"agendaId": in.AgendaID, "choiceId": in.ChoiceID, "ok": true}, nil
		}),
	agentTool("governance", "governance_cast_proposal_vote",
		"Cast this wallet's vote on a Politeia proposal. Requires a spend grant with voting enabled.",
		func(ctx context.Context, a *agent, in castVoteInput) (any, error) {
			pass, err := grants.authorizeVoting(a.id, time.Now())
			if err != nil {
				recordSpend(a, "governance_cast_proposal_vote", 0, 0, in.Token, "denied", err.Error())
				return nil, err
			}
			defer zero(pass)
			res, err := services.CastPoliteiaVote(ctx, types.CastPoliteiaVoteRequest{Token: in.Token, VoteOption: in.VoteOption}, pass)
			if err != nil {
				recordSpend(a, "governance_cast_proposal_vote", 0, 0, in.Token, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "governance_cast_proposal_vote", 0, 0, in.Token, "ok", in.VoteOption)
			return res, nil
		}),
	agentTool("governance", "governance_set_treasury_policy",
		"Set this wallet's treasury key (Pi key) voting policy. Requires a spend grant with voting enabled.",
		func(ctx context.Context, a *agent, in treasuryPolicyInput) (any, error) {
			pass, err := grants.authorizeVoting(a.id, time.Now())
			if err != nil {
				recordSpend(a, "governance_set_treasury_policy", 0, 0, in.Key, "denied", err.Error())
				return nil, err
			}
			defer zero(pass)
			if err := services.SetTreasuryKeyPolicy(ctx, in.Key, in.Policy, pass); err != nil {
				recordSpend(a, "governance_set_treasury_policy", 0, 0, in.Key, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "governance_set_treasury_policy", 0, 0, in.Key, "ok", in.Policy)
			return map[string]any{"key": in.Key, "policy": in.Policy, "ok": true}, nil
		}),
	agentTool("governance", "governance_set_tspend_policy",
		"Set this wallet's voting policy for a specific treasury spend (TSpend). Requires a spend grant with voting enabled.",
		func(ctx context.Context, a *agent, in tspendPolicyInput) (any, error) {
			pass, err := grants.authorizeVoting(a.id, time.Now())
			if err != nil {
				recordSpend(a, "governance_set_tspend_policy", 0, 0, in.Hash, "denied", err.Error())
				return nil, err
			}
			defer zero(pass)
			if err := services.SetTSpendPolicyForHash(ctx, in.Hash, in.Policy, pass); err != nil {
				recordSpend(a, "governance_set_tspend_policy", 0, 0, in.Hash, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "governance_set_tspend_policy", 0, 0, in.Hash, "ok", in.Policy)
			return map[string]any{"hash": in.Hash, "policy": in.Policy, "ok": true}, nil
		}),
}
