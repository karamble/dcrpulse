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

// proposalTokenInput parameterizes proposal-by-token reads.
type proposalTokenInput struct {
	Token string `json:"token" jsonschema:"Politeia proposal token"`
}

// proposalDetailInput parameterizes the proposal-detail reads. The rendered HTML
// duplicates of the markdown body and comments are stripped by default to keep the
// payload small; IncludeHtml restores them when a caller actually needs the HTML.
type proposalDetailInput struct {
	Token       string `json:"token" jsonschema:"Politeia proposal token"`
	IncludeHtml bool   `json:"includeHtml,omitempty" jsonschema:"include rendered descriptionHtml/commentHtml (default false)"`
}

// proposalsListInput parameterizes the proposal-list reads with an optional status
// filter on the computed bucket.
type proposalsListInput struct {
	Status string `json:"status,omitempty" jsonschema:"optional status filter: pre-vote, voting, finished, abandoned"`
}

// leanProposalComment is one proposal comment without the rendered HTML body.
type leanProposalComment struct {
	CommentID uint32 `json:"commentID"`
	ParentID  uint32 `json:"parentID"`
	Username  string `json:"username"`
	Comment   string `json:"comment"`
	CreatedAt int64  `json:"createdAt"`
	Upvotes   int64  `json:"upvotes"`
	Downvotes int64  `json:"downvotes"`
	Deleted   bool   `json:"deleted"`
	Reason    string `json:"reason,omitempty"`
}

// leanProposalDetail is a proposal detail without the rendered HTML duplicates of the
// markdown description and comments.
type leanProposalDetail struct {
	types.Proposal
	Description string                     `json:"description"`
	SubmittedAt int64                      `json:"submittedAt"`
	VoteOptions []types.ProposalVoteOption `json:"voteOptions"`
	Comments    []leanProposalComment      `json:"comments"`
}

// shapeProposalDetail returns the detail as-is when includeHtml is set, otherwise a
// lean view that drops the HTML re-renders of the markdown body and comments.
func shapeProposalDetail(d *types.ProposalDetail, includeHtml bool) any {
	if d == nil || includeHtml {
		return d
	}
	lean := leanProposalDetail{
		Proposal:    d.Proposal,
		Description: d.Description,
		SubmittedAt: d.SubmittedAt,
		VoteOptions: d.VoteOptions,
		Comments:    make([]leanProposalComment, len(d.Comments)),
	}
	for i, c := range d.Comments {
		lean.Comments[i] = leanProposalComment{
			CommentID: c.CommentID,
			ParentID:  c.ParentID,
			Username:  c.Username,
			Comment:   c.Comment,
			CreatedAt: c.CreatedAt,
			Upvotes:   c.Upvotes,
			Downvotes: c.Downvotes,
			Deleted:   c.Deleted,
			Reason:    c.Reason,
		}
	}
	return lean
}

// filterProposalsByStatus narrows a proposal list to a single computed status bucket;
// an empty status returns the list unchanged.
func filterProposalsByStatus(ps []types.Proposal, status string) []types.Proposal {
	if status == "" {
		return ps
	}
	out := make([]types.Proposal, 0, len(ps))
	for _, p := range ps {
		if p.Status == status {
			out = append(out, p)
		}
	}
	return out
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
		"List Politeia governance proposals. Optionally filter by status: pre-vote, voting, finished, abandoned.",
		func(ctx context.Context, in proposalsListInput) (any, error) {
			proposals, _, err := services.ListProposals(ctx)
			if err != nil {
				return nil, err
			}
			return filterProposalsByStatus(proposals, in.Status), nil
		}),
	readTool("governance", "governance_proposal_detail",
		"Get one Politeia proposal's full record (cached, auto-fetched on first access). Rendered HTML is omitted unless includeHtml is set.",
		func(ctx context.Context, in proposalDetailInput) (any, error) {
			detail, _, err := services.GetProposalDetail(ctx, in.Token)
			if err != nil {
				return nil, err
			}
			return shapeProposalDetail(detail, in.IncludeHtml), nil
		}),
	readTool("governance", "governance_proposal_vote_eligibility",
		"Compute this wallet's vote eligibility for a proposal (owned-ticket count, options, already-voted state).",
		func(ctx context.Context, in proposalTokenInput) (any, error) {
			return services.PrepareProposalVote(ctx, in.Token)
		}),
	readTool("governance", "governance_refresh_proposals",
		"Force a Politeia re-fetch of the proposals list, subject to the refresh cooldown. Optionally filter by status: pre-vote, voting, finished, abandoned.",
		func(ctx context.Context, in proposalsListInput) (any, error) {
			proposals, _, err := services.RefreshProposals(ctx)
			if err != nil {
				return nil, err
			}
			return filterProposalsByStatus(proposals, in.Status), nil
		}),
	readTool("governance", "governance_refresh_proposal_detail",
		"Force a re-fetch of one proposal's detail, subject to the refresh cooldown. Rendered HTML is omitted unless includeHtml is set.",
		func(ctx context.Context, in proposalDetailInput) (any, error) {
			detail, _, err := services.RefreshProposalDetail(ctx, in.Token)
			if err != nil {
				return nil, err
			}
			return shapeProposalDetail(detail, in.IncludeHtml), nil
		}),
	agentTool("governance", "governance_set_vote_choice",
		"Set this wallet's vote choice for a consensus agenda. Requires a spend grant with voting enabled; signs with the held passphrase.",
		func(ctx context.Context, a *agent, in setVoteChoiceInput) (any, error) {
			pass, err := grants.authorizeActionPass(a.id, scopeGovernance, time.Now())
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
			pass, err := grants.authorizeActionPass(a.id, scopeGovernance, time.Now())
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
			pass, err := grants.authorizeActionPass(a.id, scopeGovernance, time.Now())
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
			pass, err := grants.authorizeActionPass(a.id, scopeGovernance, time.Now())
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
