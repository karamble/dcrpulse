// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// AgendaChoice is one option (yes / no / abstain) on a consensus agenda.
type AgendaChoice struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	IsAbstain   bool   `json:"isAbstain"`
	IsNo        bool   `json:"isNo"`
}

// Agenda is one currently-tracked consensus rule change vote.
type Agenda struct {
	ID            string         `json:"id"`
	Description   string         `json:"description"`
	Status        string         `json:"status"`
	StartHeight   int64          `json:"startHeight"`
	ExpireHeight  int64          `json:"expireHeight"`
	Choices       []AgendaChoice `json:"choices"`
	CurrentChoice string         `json:"currentChoice"`
}

// SetAgendaChoiceRequest is the body for the agenda set endpoint.
type SetAgendaChoiceRequest struct {
	AgendaID   string `json:"agendaID"`
	ChoiceID   string `json:"choiceID"`
	Passphrase string `json:"passphrase"`
}

// TreasuryKeyPolicy is one wallet-wide policy for a Politeia treasury key.
type TreasuryKeyPolicy struct {
	Key    string `json:"key"`
	Policy string `json:"policy"`
}

// SetTreasuryKeyPolicyRequest is the body for the treasury-key set endpoint.
type SetTreasuryKeyPolicyRequest struct {
	Key        string `json:"key"`
	Policy     string `json:"policy"`
	Passphrase string `json:"passphrase"`
}

// TSpendPolicy is one wallet-wide policy for a specific TSpend hash.
// Amount, Expiry, BlockHeight are enriched from the treasury scanner
// when the TSpend is also observed in our mempool view.
type TSpendPolicy struct {
	Hash        string `json:"hash"`
	Policy      string `json:"policy"`
	Amount      int64  `json:"amount,omitempty"`
	Expiry      int64  `json:"expiry,omitempty"`
	BlockHeight int64  `json:"blockHeight,omitempty"`
}

// SetTSpendPolicyRequest is the body for the TSpend set endpoint.
type SetTSpendPolicyRequest struct {
	Hash       string `json:"hash"`
	Policy     string `json:"policy"`
	Passphrase string `json:"passphrase"`
}

// Proposal is the list-view shape for a Politeia proposal.
type Proposal struct {
	Token           string           `json:"token"`
	Name            string           `json:"name"`
	Username        string           `json:"username"`
	Status          string           `json:"status"`
	VoteStatus      string           `json:"voteStatus"`
	VoteCounts      map[string]int64 `json:"voteCounts"`
	TotalVotes      int64            `json:"totalVotes"`
	QuorumMin       int64            `json:"quorumMin"`
	EligibleTickets int64            `json:"eligibleTickets"`
	EndBlock        int64            `json:"endBlock"`
	BlocksLeft      int64            `json:"blocksLeft"`
	CurrentChoice   string           `json:"currentChoice"`
}

// ProposalVoteOption mirrors Politeia's vote option definition.
type ProposalVoteOption struct {
	ID  string `json:"id"`
	Bit uint32 `json:"bit"`
}

// ProposalDetail is the full per-token Politeia record + vote data.
// EligibleTickets is inherited from the embedded Proposal field.
type ProposalDetail struct {
	Proposal
	Description     string               `json:"description"`
	DescriptionHTML string               `json:"descriptionHtml"`
	SubmittedAt     int64                `json:"submittedAt"`
	VoteOptions     []ProposalVoteOption `json:"voteOptions"`
}

// CastPoliteiaVoteRequest is the body for the cast-vote endpoint.
type CastPoliteiaVoteRequest struct {
	Token      string `json:"token"`
	VoteOption string `json:"voteOption"`
	Passphrase string `json:"passphrase"`
}

// CastPoliteiaVoteResult summarises a cast attempt.
type CastPoliteiaVoteResult struct {
	Cast    int      `json:"cast"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

// ProposalsResponse is the envelope for the proposals list endpoint: the
// cached list plus the last successful fetch time and when a manual refresh
// is next allowed (both unix seconds; 0 when never fetched).
type ProposalsResponse struct {
	Proposals          []Proposal `json:"proposals"`
	FetchedAt          int64      `json:"fetchedAt"`
	RefreshAvailableAt int64      `json:"refreshAvailableAt"`
}
