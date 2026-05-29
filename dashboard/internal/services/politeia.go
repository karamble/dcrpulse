// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

const (
	politeiaBaseURL          = "https://proposals.decred.org/api"
	politeiaCacheTTL         = 1 * time.Hour
	politeiaTimeout          = 30 * time.Second
	politeiaCastTimeout      = 60 * time.Second
	politeiaSignMessagesChunk = 100

	// ProposalsRefreshCooldown is how long the manual proposals refresh stays
	// disabled after a successful fetch. ProposalsFetchTimeout caps a full
	// list fetch (many sequential upstream calls). Exported so handlers can
	// size their request context and refresh-availability math to match.
	ProposalsRefreshCooldown = 8 * time.Hour
	ProposalsFetchTimeout    = 1 * time.Minute

	// preparedVoteTTL bounds how long a PrepareProposalVote result is reused
	// by a follow-up cast. The eligible-ticket snapshot and committed-ticket
	// set are stable for the duration of a vote, so this only needs to span a
	// single open-modal-then-cast interaction.
	preparedVoteTTL = 15 * time.Minute
)

// Markdown renderer + sanitizer for Politeia proposal descriptions.
// renderProposalMarkdown is kept as a thin wrapper to minimise diff in
// politeia.go's call sites. See services.RenderMarkdownHTML for the
// underlying goldmark + bluemonday pipeline.
func renderProposalMarkdown(src string) string {
	return RenderMarkdownHTML(src)
}

// PoliteiaEnabled reports whether the Politeia external-request toggle
// is on. Defaults to true when no global config is present yet.
func PoliteiaEnabled() bool {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return true
	}
	allowed, _ := gc.AllowedExternalRequests()
	if allowed == nil {
		return true
	}
	v, ok := allowed[config.ExternalRequestPoliteia]
	if !ok {
		return true
	}
	return v
}

// In-memory caches for politeia HTTP responses. Both the proposals list
// envelope and per-token detail entries reuse the same 5-min TTL window
// so repeated navigation between the list and a detail page doesn't
// hammer proposals.decred.org. Cast-vote invalidates everything for the
// touched token (and the whole list) so tallies update on next view.
var (
	piCacheMu       sync.RWMutex
	piCachedList    []types.Proposal
	piCachedAt      time.Time
	piCachedDetails = map[string]piDetailCacheEntry{}
	piPreparedVotes = map[string]piPreparedVote{}
	piHTTPClient    = &http.Client{Timeout: politeiaTimeout}
)

type piDetailCacheEntry struct {
	detail *types.ProposalDetail
	at     time.Time
}

// piPreparedVote caches what PrepareProposalVote computed (the wallet's owned
// eligible tickets + the vote options) so a subsequent cast can reuse it
// without re-fetching the eligible-ticket snapshot or re-running
// CommittedTickets. Keyed by network|wallet|token. A present entry implies the
// wallet has NOT already voted (PrepareProposalVote stores it only on that path).
type piPreparedVote struct {
	ownedTickets []*pb.CommittedTicketsResponse_TicketAddress
	options      []types.ProposalVoteOption
	at           time.Time
}

// ListProposals returns the union of all current Politeia proposals with
// their summary tallies, plus the time of the last successful fetch. The
// list is cached in-process indefinitely; an empty cache (e.g. after a
// restart) auto-fetches once on first access. Use RefreshProposals to force
// a re-fetch. Returns ErrPoliteiaDisabled if the toggle is off.
func ListProposals(ctx context.Context) ([]types.Proposal, time.Time, error) {
	if !PoliteiaEnabled() {
		return nil, time.Time{}, ErrPoliteiaDisabled
	}

	piCacheMu.RLock()
	if piCachedList != nil {
		cached, at := piCachedList, piCachedAt
		piCacheMu.RUnlock()
		return cached, at, nil
	}
	piCacheMu.RUnlock()

	out, err := fetchAndCacheProposals(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	piCacheMu.RLock()
	at := piCachedAt
	piCacheMu.RUnlock()
	return out, at, nil
}

// RefreshProposals forces a re-fetch of the proposals list, subject to the
// ProposalsRefreshCooldown measured from the last successful fetch. While
// cooling down it returns the cached list, the last-fetch time, and
// ErrProposalsRefreshCoolingDown. On success it returns the fresh list and
// the new fetch time. A failed fetch does not start the cooldown.
func RefreshProposals(ctx context.Context) ([]types.Proposal, time.Time, error) {
	if !PoliteiaEnabled() {
		return nil, time.Time{}, ErrPoliteiaDisabled
	}

	piCacheMu.RLock()
	at, cached := piCachedAt, piCachedList
	piCacheMu.RUnlock()
	if !at.IsZero() && time.Since(at) < ProposalsRefreshCooldown {
		return cached, at, ErrProposalsRefreshCoolingDown
	}

	out, err := fetchAndCacheProposals(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	piCacheMu.RLock()
	newAt := piCachedAt
	piCacheMu.RUnlock()
	return out, newAt, nil
}

// fetchAndCacheProposals fetches the full proposal list from Politeia and
// stores it in the in-process cache. piCachedList/piCachedAt are written ONLY
// on success, so a failed fetch leaves the cache (and the refresh cooldown
// anchor) untouched. Capped at ProposalsFetchTimeout.
func fetchAndCacheProposals(ctx context.Context) ([]types.Proposal, error) {
	ctx, cancel := context.WithTimeout(ctx, ProposalsFetchTimeout)
	defer cancel()

	inv, err := piInventory(ctx)
	if err != nil {
		return nil, err
	}
	tokens := flattenInventory(inv)
	if len(tokens) == 0 {
		piCacheMu.Lock()
		piCachedList = []types.Proposal{}
		piCachedAt = time.Now()
		piCacheMu.Unlock()
		return []types.Proposal{}, nil
	}

	// Batch the records call so we don't blow past Politeia's request
	// size limit on the records endpoint (5 tokens per batch is what
	// Decrediton uses).
	records := map[string]piRecord{}
	for i := 0; i < len(tokens); i += 5 {
		end := i + 5
		if end > len(tokens) {
			end = len(tokens)
		}
		chunk, err := piRecordsBatch(ctx, tokens[i:end])
		if err != nil {
			log.Printf("politeia records batch: %v", err)
			continue
		}
		for k, v := range chunk {
			records[k] = v
		}
	}

	summaries := map[string]piSummary{}
	for i := 0; i < len(tokens); i += 5 {
		end := i + 5
		if end > len(tokens) {
			end = len(tokens)
		}
		chunk, err := piSummariesBatch(ctx, tokens[i:end])
		if err != nil {
			log.Printf("politeia summaries batch: %v", err)
			continue
		}
		for k, v := range chunk {
			summaries[k] = v
		}
	}

	// Local cache of "you voted X" choices so the UI can show the
	// choice without rehitting Politeia's vote-results endpoint.
	localVotes := loadLocalPoliteiaVotes(ctx)

	out := make([]types.Proposal, 0, len(tokens))
	for _, t := range tokens {
		rec := records[t]
		sum := summaries[t]
		proposal := proposalFromRecordAndSummary(t, rec, sum)
		proposal.CurrentChoice = localVotes[t]
		out = append(out, proposal)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].EndBlock > out[j].EndBlock
	})

	piCacheMu.Lock()
	piCachedList = out
	piCachedAt = time.Now()
	piCacheMu.Unlock()

	return out, nil
}

// GetProposalDetail returns one proposal with full description + vote options,
// plus the time it was fetched. Cached in-process indefinitely (per token);
// auto-fetched on first access and cleared on a successful cast-vote against
// the token. Use RefreshProposalDetail to force a re-fetch.
func GetProposalDetail(ctx context.Context, token string) (*types.ProposalDetail, time.Time, error) {
	if !PoliteiaEnabled() {
		return nil, time.Time{}, ErrPoliteiaDisabled
	}
	if token == "" {
		return nil, time.Time{}, fmt.Errorf("token required")
	}

	piCacheMu.RLock()
	if entry, ok := piCachedDetails[token]; ok {
		cached := *entry.detail
		at := entry.at
		piCacheMu.RUnlock()
		// Local "you voted X" cache might have changed since the entry
		// was warmed; refresh that field from the per-wallet cfg.
		if localVotes := loadLocalPoliteiaVotes(ctx); localVotes != nil {
			if v, ok := localVotes[token]; ok {
				cached.CurrentChoice = v
			}
		}
		return &cached, at, nil
	}
	piCacheMu.RUnlock()

	return fetchAndCacheProposalDetail(ctx, token)
}

// RefreshProposalDetail forces a re-fetch of one proposal's detail, subject to
// the ProposalsRefreshCooldown measured from that token's last successful
// fetch. While cooling down it returns the cached detail, its fetch time, and
// ErrProposalsRefreshCoolingDown. A failed fetch does not start the cooldown.
func RefreshProposalDetail(ctx context.Context, token string) (*types.ProposalDetail, time.Time, error) {
	if !PoliteiaEnabled() {
		return nil, time.Time{}, ErrPoliteiaDisabled
	}
	if token == "" {
		return nil, time.Time{}, fmt.Errorf("token required")
	}

	piCacheMu.RLock()
	entry, ok := piCachedDetails[token]
	piCacheMu.RUnlock()
	if ok && !entry.at.IsZero() && time.Since(entry.at) < ProposalsRefreshCooldown {
		cached := *entry.detail
		return &cached, entry.at, ErrProposalsRefreshCoolingDown
	}

	return fetchAndCacheProposalDetail(ctx, token)
}

// fetchAndCacheProposalDetail fetches one proposal's full record + summary
// (+ vote details when voting is active) from Politeia and caches it per token.
// The cache entry (and thus the refresh cooldown anchor) is written ONLY on
// success. Capped at ProposalsFetchTimeout.
func fetchAndCacheProposalDetail(ctx context.Context, token string) (*types.ProposalDetail, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, ProposalsFetchTimeout)
	defer cancel()

	records, err := piRecordsBatch(ctx, []string{token})
	if err != nil {
		return nil, time.Time{}, err
	}
	rec, ok := records[token]
	if !ok {
		return nil, time.Time{}, fmt.Errorf("record not found")
	}
	sum, err := piSummariesBatch(ctx, []string{token})
	if err != nil {
		return nil, time.Time{}, err
	}

	proposal := proposalFromRecordAndSummary(token, rec, sum[token])
	localVotes := loadLocalPoliteiaVotes(ctx)
	proposal.CurrentChoice = localVotes[token]

	desc := indexMarkdown(rec)
	out := &types.ProposalDetail{
		Proposal:        proposal,
		Description:     desc,
		DescriptionHTML: renderProposalMarkdown(desc),
		SubmittedAt:     rec.Timestamp,
	}

	// Comments are a best-effort enrichment: a failure here must not block
	// the proposal view.
	if cmts, err := piComments(ctx, token); err != nil {
		log.Printf("politeia comments %s: %v", token, err)
	} else {
		out.Comments = commentsFromPi(cmts)
	}

	// Vote options come from the already-fetched summary (its per-option
	// results carry the id + vote bit), so a detail view never needs the
	// heavy eligible-ticket snapshot from /ticketvote/v1/details. That
	// snapshot is fetched on demand only when the user opens the vote modal
	// (see PrepareProposalVote).
	out.VoteOptions = voteOptionsFromSummary(sum[token])

	at := time.Now()
	piCacheMu.Lock()
	piCachedDetails[token] = piDetailCacheEntry{detail: out, at: at}
	piCacheMu.Unlock()

	return out, at, nil
}

// PrepareProposalVote computes everything the vote modal needs when the user
// opens it: how many of the proposal's eligible tickets the wallet owns, the
// vote options, and whether the wallet has already voted. The heavy work (the
// eligible-ticket snapshot + CommittedTickets + the recorded-votes
// reconciliation) runs only here, on explicit user action, never on a normal
// detail-page view. A locally recorded choice short-circuits all of it.
func PrepareProposalVote(ctx context.Context, token string) (*types.VoteEligibility, error) {
	if !PoliteiaEnabled() {
		return nil, ErrPoliteiaDisabled
	}
	if token == "" {
		return nil, fmt.Errorf("token required")
	}

	// Options for display come from the already-cached summary/detail, not the
	// heavy snapshot.
	out := &types.VoteEligibility{VoteOptions: cachedVoteOptions(token)}

	// Fast-path: a locally recorded choice means this wallet already voted
	// through dcrpulse. Detect it instantly with no Politeia or gRPC calls.
	if choice := loadLocalPoliteiaVotes(ctx)[token]; choice != "" {
		out.AlreadyVoted = true
		out.CurrentChoice = choice
		return out, nil
	}

	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC unavailable")
	}

	ctx, cancel := context.WithTimeout(ctx, ProposalsFetchTimeout)
	defer cancel()

	det, err := piVoteDetails(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("vote details: %w", err)
	}
	options := make([]types.ProposalVoteOption, 0, len(det.Vote.Params.Options))
	for _, o := range det.Vote.Params.Options {
		options = append(options, types.ProposalVoteOption{ID: o.ID, Bit: o.Bit})
	}
	out.VoteOptions = options
	out.EligibleTickets = int64(len(det.Vote.EligibleTickets))

	addrs, err := committedFromEligible(ctx, det.Vote.EligibleTickets)
	if err != nil {
		return nil, err
	}
	out.OwnedEligibleCount = len(addrs)
	if len(addrs) == 0 {
		return out, nil
	}

	// Reconcile against Politeia's recorded (off-chain) votes: if one of the
	// wallet's owned eligible tickets already appears in the results, the
	// wallet has voted. Persist the discovered choice so future opens hit the
	// local fast-path. Mirrors Decrediton getVoteOption (assumes a uniform bit
	// across the wallet's tickets).
	if votes, err := piResults(ctx, token); err != nil {
		log.Printf("politeia results %s: %v", token, err)
	} else if choice := walletVoteChoice(addrs, votes, options); choice != "" {
		out.AlreadyVoted = true
		out.CurrentChoice = choice
		if err := persistLocalPoliteiaVote(ctx, token, choice); err != nil {
			log.Printf("persist politeia vote: %v", err)
		}
		return out, nil
	}

	// Cache the owned-ticket set so the follow-up cast reuses it.
	piCacheMu.Lock()
	piPreparedVotes[preparedVoteKey(ctx, token)] = piPreparedVote{
		ownedTickets: addrs,
		options:      options,
		at:           time.Now(),
	}
	piCacheMu.Unlock()

	return out, nil
}

// committedFromEligible intersects a Politeia eligible-ticket snapshot (big-
// endian hex hashes) with the wallet's committed tickets, returning the tickets
// the wallet owns. CommittedTickets expects little-endian bytes.
func committedFromEligible(ctx context.Context, eligible []string) ([]*pb.CommittedTicketsResponse_TicketAddress, error) {
	if len(eligible) == 0 {
		return nil, nil
	}
	ticketBytes := make([][]byte, 0, len(eligible))
	for _, t := range eligible {
		b, err := hex.DecodeString(t)
		if err != nil {
			continue
		}
		ticketBytes = append(ticketBytes, reversed(b))
	}
	owned, err := rpc.WalletGrpcClient.CommittedTickets(ctx, &pb.CommittedTicketsRequest{Tickets: ticketBytes})
	if err != nil {
		return nil, fmt.Errorf("CommittedTickets: %w", err)
	}
	return owned.GetTicketAddresses(), nil
}

// walletVoteChoice returns the option id the wallet voted, or "" if none of its
// owned tickets appear in the recorded votes.
func walletVoteChoice(owned []*pb.CommittedTicketsResponse_TicketAddress, votes []piCastVote, options []types.ProposalVoteOption) string {
	if len(owned) == 0 || len(votes) == 0 {
		return ""
	}
	ownedHex := make(map[string]struct{}, len(owned))
	for _, ta := range owned {
		ownedHex[hex.EncodeToString(reversed(ta.GetTicket()))] = struct{}{}
	}
	for _, v := range votes {
		if _, ok := ownedHex[v.Ticket]; !ok {
			continue
		}
		bit, err := parseVoteBit(v.VoteBit)
		if err != nil {
			continue
		}
		for _, o := range options {
			if uint64(o.Bit) == bit {
				return o.ID
			}
		}
	}
	return ""
}

// parseVoteBit parses a Politeia vote bit, which may be decimal or hex.
func parseVoteBit(s string) (uint64, error) {
	if n, err := strconv.ParseUint(s, 10, 32); err == nil {
		return n, nil
	}
	return strconv.ParseUint(s, 16, 32)
}

// bitForOption returns the vote bit for the option with the given id.
func bitForOption(options []types.ProposalVoteOption, id string) (uint32, bool) {
	for _, o := range options {
		if o.ID == id {
			return o.Bit, true
		}
	}
	return 0, false
}

// cachedVoteOptions returns the proposal's vote options from the in-process
// detail cache, or nil if the detail has not been fetched yet.
func cachedVoteOptions(token string) []types.ProposalVoteOption {
	piCacheMu.RLock()
	defer piCacheMu.RUnlock()
	if e, ok := piCachedDetails[token]; ok && e.detail != nil {
		return e.detail.VoteOptions
	}
	return nil
}

// preparedVoteKey scopes a prepared-vote cache entry to the active network +
// wallet so it can never be reused across wallets.
func preparedVoteKey(ctx context.Context, token string) string {
	network, _ := CurrentNetwork(ctx)
	return network + "|" + CurrentWalletName() + "|" + token
}

// loadPreparedVote returns a fresh prepared-vote entry for the token, if any.
func loadPreparedVote(ctx context.Context, token string) (piPreparedVote, bool) {
	piCacheMu.RLock()
	pv, ok := piPreparedVotes[preparedVoteKey(ctx, token)]
	piCacheMu.RUnlock()
	if !ok || time.Since(pv.at) > preparedVoteTTL {
		return piPreparedVote{}, false
	}
	return pv, true
}

// CastPoliteiaVote runs the full sign + cast flow: fetch eligible tickets,
// intersect with wallet-owned tickets, sign each message, POST castballot.
// Returns aggregate counts + per-ticket errors.
func CastPoliteiaVote(ctx context.Context, req types.CastPoliteiaVoteRequest, passphrase []byte) (*types.CastPoliteiaVoteResult, error) {
	if !PoliteiaEnabled() {
		return nil, ErrPoliteiaDisabled
	}
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC unavailable")
	}

	// Reuse the owned-ticket set + options computed when the user opened the
	// vote modal (PrepareProposalVote), avoiding a redundant snapshot fetch +
	// CommittedTickets pass. Fall back to computing them here when no fresh
	// prepared state exists.
	var addrs []*pb.CommittedTicketsResponse_TicketAddress
	var options []types.ProposalVoteOption
	if pv, ok := loadPreparedVote(ctx, req.Token); ok {
		addrs = pv.ownedTickets
		options = pv.options
	} else {
		det, err := piVoteDetails(ctx, req.Token)
		if err != nil {
			return nil, fmt.Errorf("vote details: %w", err)
		}
		for _, o := range det.Vote.Params.Options {
			options = append(options, types.ProposalVoteOption{ID: o.ID, Bit: o.Bit})
		}
		addrs, err = committedFromEligible(ctx, det.Vote.EligibleTickets)
		if err != nil {
			return nil, err
		}
	}

	voteBit, found := bitForOption(options, req.VoteOption)
	if !found {
		return nil, fmt.Errorf("vote option %q not allowed for this proposal", req.VoteOption)
	}
	if len(addrs) == 0 {
		return &types.CastPoliteiaVoteResult{}, nil
	}

	// Unlock the wallet so SignMessages can sign with the per-ticket
	// commitment private keys.
	if err := unlockForVote(ctx, passphrase); err != nil {
		return nil, err
	}
	defer lockAfterVote()

	bitHex := fmt.Sprintf("%x", voteBit)
	bitDecimal := fmt.Sprintf("%d", voteBit)

	// Build signature requests. The message format is
	// token||ticketHashHex||voteBitHex.
	signMsgs := make([]*pb.SignMessagesRequest_Message, 0, len(addrs))
	ticketHexByIndex := make([]string, 0, len(addrs))
	for _, ta := range addrs {
		ticketHex := hex.EncodeToString(reversed(ta.GetTicket()))
		ticketHexByIndex = append(ticketHexByIndex, ticketHex)
		signMsgs = append(signMsgs, &pb.SignMessagesRequest_Message{
			Address: ta.GetAddress(),
			Message: req.Token + ticketHex + bitHex,
		})
	}

	// Chunk the SignMessages calls.
	result := &types.CastPoliteiaVoteResult{}
	signatures := make([]string, len(signMsgs))
	for i := 0; i < len(signMsgs); i += politeiaSignMessagesChunk {
		end := i + politeiaSignMessagesChunk
		if end > len(signMsgs) {
			end = len(signMsgs)
		}
		resp, err := rpc.WalletGrpcClient.SignMessages(ctx, &pb.SignMessagesRequest{
			Passphrase: passphrase,
			Messages:   signMsgs[i:end],
		})
		if err != nil {
			return nil, fmt.Errorf("SignMessages: %w", err)
		}
		for j, reply := range resp.GetReplies() {
			if reply.GetError() != "" {
				result.Errors = append(result.Errors, fmt.Sprintf("sign %s: %s", ticketHexByIndex[i+j], reply.GetError()))
				result.Skipped++
				continue
			}
			signatures[i+j] = base64.StdEncoding.EncodeToString(reply.GetSignature())
		}
	}

	// Build the ballot.
	type castVote struct {
		Token     string `json:"token"`
		Ticket    string `json:"ticket"`
		VoteBit   string `json:"votebit"`
		Signature string `json:"signature"`
	}
	type castBallotRequest struct {
		Votes []castVote `json:"votes"`
	}
	type castBallotReceipt struct {
		Ticket    string `json:"ticket"`
		Receipt   string `json:"receipt"`
		ErrorCode int    `json:"errorcode"`
		ErrorMsg  string `json:"errorcontext"`
	}
	type castBallotResponse struct {
		Receipts []castBallotReceipt `json:"receipts"`
	}

	ballot := castBallotRequest{Votes: make([]castVote, 0, len(signatures))}
	for i, sig := range signatures {
		if sig == "" {
			continue
		}
		ballot.Votes = append(ballot.Votes, castVote{
			Token:     req.Token,
			Ticket:    ticketHexByIndex[i],
			VoteBit:   bitDecimal,
			Signature: sig,
		})
	}
	if len(ballot.Votes) == 0 {
		return result, nil
	}

	var resp castBallotResponse
	if err := piPost(ctx, "/ticketvote/v1/castballot", ballot, &resp); err != nil {
		return nil, fmt.Errorf("castballot: %w", err)
	}
	for _, r := range resp.Receipts {
		if r.ErrorCode != 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("cast %s: %s", r.Ticket, r.ErrorMsg))
			result.Skipped++
			continue
		}
		result.Cast++
	}

	// Persist local "you voted X" cache.
	if result.Cast > 0 {
		if err := persistLocalPoliteiaVote(ctx, req.Token, req.VoteOption); err != nil {
			log.Printf("persist politeia vote: %v", err)
		}
	}

	// Invalidate the list cache and this proposal's detail cache so the
	// next view reflects the updated tallies and currentChoice. The list
	// cache is now unlimited-TTL, so clear the slice itself (not just the
	// timestamp) to force a re-fetch on next view.
	piCacheMu.Lock()
	piCachedList = nil
	piCachedAt = time.Time{}
	delete(piCachedDetails, req.Token)
	delete(piPreparedVotes, preparedVoteKey(ctx, req.Token))
	piCacheMu.Unlock()

	return result, nil
}

// ---- internal HTTP helpers + decoders -------------------------------------

// ErrPoliteiaDisabled is returned by every public function when the
// politeia toggle in Settings is off. Handlers translate to 503.
var ErrPoliteiaDisabled = fmt.Errorf("politeia disabled in settings")

// ErrProposalsRefreshCoolingDown is returned by RefreshProposals when a manual
// refresh is requested within ProposalsRefreshCooldown of the last successful
// fetch. Handlers translate to 429.
var ErrProposalsRefreshCoolingDown = fmt.Errorf("proposals refresh cooling down")

type piInventoryResp struct {
	Vetted    map[string][]string `json:"vetted"`
	BestBlock int64               `json:"bestblock"`
}

type piRecord struct {
	State           int          `json:"state"`
	Status          int          `json:"status"`
	Version         int          `json:"version"`
	Timestamp       int64        `json:"timestamp"`
	Username        string       `json:"username"`
	Files           []piFile     `json:"files"`
	CensorshipRecord piCensorship `json:"censorshiprecord"`
}

type piCensorship struct {
	Token string `json:"token"`
}

type piFile struct {
	Name    string `json:"name"`
	MIME    string `json:"mime"`
	Payload string `json:"payload"`
}

type piRecordsResp struct {
	Records map[string]piRecord `json:"records"`
}

type piSummary struct {
	Type                int                 `json:"type"`
	Status              int                 `json:"status"`
	Duration            int64               `json:"duration"`
	StartBlockHeight    int64               `json:"startblockheight"`
	EndBlockHeight      int64               `json:"endblockheight"`
	EligibleTicketCount int64               `json:"eligibletickets"`
	QuorumPercentage    int                 `json:"quorumpercentage"`
	PassPercentage      int                 `json:"passpercentage"`
	BestBlock           int64               `json:"bestblock"`
	Results             []piVoteResultEntry `json:"results"`
}

// Politeia ticketvote v1 status codes -> human-readable bucket name.
var piStatusName = map[int]string{
	0: "invalid",
	1: "unauthorized",
	2: "authorized",
	3: "started",
	4: "finished",
	5: "approved",
	6: "rejected",
	7: "ineligible",
	8: "abandoned",
}

type piVoteResultEntry struct {
	ID      string `json:"id"`
	VoteBit uint32 `json:"votebit"`
	Votes   int64  `json:"votes"`
}

type piSummariesResp struct {
	Summaries map[string]piSummary `json:"summaries"`
}

type piVoteDetailsResp struct {
	Auths []json.RawMessage `json:"auths"`
	Vote  piVoteSection     `json:"vote"`
}

type piResultsResp struct {
	Votes []piCastVote `json:"votes"`
}

type piCastVote struct {
	Token   string `json:"token"`
	Ticket  string `json:"ticket"`
	VoteBit string `json:"votebit"`
}

type piCommentsResp struct {
	Comments []piComment `json:"comments"`
}

type piComment struct {
	CommentID uint32 `json:"commentid"`
	ParentID  uint32 `json:"parentid"`
	Username  string `json:"username"`
	Comment   string `json:"comment"`
	CreatedAt int64  `json:"createdat"`
	Upvotes   int64  `json:"upvotes"`
	Downvotes int64  `json:"downvotes"`
	Deleted   bool   `json:"deleted"`
	Reason    string `json:"reason"`
}

type piVoteSection struct {
	Params           piVoteParams `json:"params"`
	StartBlockHeight int64        `json:"startblockheight"`
	StartBlockHash   string       `json:"startblockhash"`
	EndBlockHeight   int64        `json:"endblockheight"`
	EligibleTickets  []string     `json:"eligibletickets"`
}

type piVoteParams struct {
	Token            string         `json:"token"`
	Type             int            `json:"type"`
	Options          []piVoteOption `json:"options"`
	Duration         int64          `json:"duration"`
	QuorumPercentage int            `json:"quorumpercentage"`
	PassPercentage   int            `json:"passpercentage"`
}

type piVoteOption struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Bit         uint32 `json:"bit"`
}

func piInventory(ctx context.Context) (piInventoryResp, error) {
	var out piInventoryResp
	err := piPost(ctx, "/ticketvote/v1/inventory", struct{}{}, &out)
	return out, err
}

func piRecordsBatch(ctx context.Context, tokens []string) (map[string]piRecord, error) {
	type recordReq struct {
		Token     string   `json:"token"`
		Filenames []string `json:"filenames,omitempty"`
	}
	body := struct {
		Requests []recordReq `json:"requests"`
	}{}
	for _, t := range tokens {
		body.Requests = append(body.Requests, recordReq{
			Token:     t,
			Filenames: []string{"proposalmetadata.json", "index.md"},
		})
	}
	var resp piRecordsResp
	if err := piPost(ctx, "/records/v1/records", body, &resp); err != nil {
		return nil, err
	}
	return resp.Records, nil
}

func piSummariesBatch(ctx context.Context, tokens []string) (map[string]piSummary, error) {
	body := struct {
		Tokens []string `json:"tokens"`
	}{Tokens: tokens}
	var resp piSummariesResp
	if err := piPost(ctx, "/ticketvote/v1/summaries", body, &resp); err != nil {
		return nil, err
	}
	return resp.Summaries, nil
}

func piVoteDetails(ctx context.Context, token string) (*piVoteDetailsResp, error) {
	body := struct {
		Token string `json:"token"`
	}{Token: token}
	var resp piVoteDetailsResp
	if err := piPost(ctx, "/ticketvote/v1/details", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// piResults fetches the votes Politeia has recorded for a proposal. Politeia
// voting is off-chain, so this is the authoritative record of cast votes.
func piResults(ctx context.Context, token string) ([]piCastVote, error) {
	body := struct {
		Token string `json:"token"`
	}{Token: token}
	var resp piResultsResp
	if err := piPost(ctx, "/ticketvote/v1/results", body, &resp); err != nil {
		return nil, err
	}
	return resp.Votes, nil
}

// piComments fetches the full comment thread for a proposal. Politeia returns
// every comment for the record in a single response (no pagination).
func piComments(ctx context.Context, token string) ([]piComment, error) {
	body := struct {
		Token string `json:"token"`
	}{Token: token}
	var resp piCommentsResp
	if err := piPost(ctx, "/comments/v1/comments", body, &resp); err != nil {
		return nil, err
	}
	return resp.Comments, nil
}

func piPost(ctx context.Context, path string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	rctx, cancel := context.WithTimeout(ctx, politeiaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, politeiaBaseURL+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := piHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("politeia %s: status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}

// flattenInventory turns the vetted-by-status map into a single token
// slice in a stable order (started/voting first, then approved, rejected,
// unauthorized, authorized, ineligible, abandoned).
func flattenInventory(inv piInventoryResp) []string {
	order := []string{"started", "authorized", "unauthorized", "approved", "rejected", "ineligible", "abandoned"}
	seen := map[string]bool{}
	out := []string{}
	for _, key := range order {
		for _, t := range inv.Vetted[key] {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	// Any unknown status buckets we haven't enumerated explicitly:
	for k, v := range inv.Vetted {
		known := false
		for _, k2 := range order {
			if k == k2 {
				known = true
				break
			}
		}
		if known {
			continue
		}
		for _, t := range v {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

func proposalFromRecordAndSummary(token string, rec piRecord, sum piSummary) types.Proposal {
	statusName := piStatusName[sum.Status]
	if statusName == "" {
		statusName = "unknown"
	}
	proposal := types.Proposal{
		Token:      token,
		Name:       proposalName(rec),
		Username:   rec.Username,
		Status:     summaryStatusToBucket(statusName),
		VoteStatus: statusName,
		VoteCounts: map[string]int64{},
		EndBlock:   sum.EndBlockHeight,
	}
	for _, r := range sum.Results {
		proposal.VoteCounts[r.ID] = r.Votes
		proposal.TotalVotes += r.Votes
	}
	proposal.EligibleTickets = sum.EligibleTicketCount
	if sum.EligibleTicketCount > 0 && sum.QuorumPercentage > 0 {
		proposal.QuorumMin = sum.EligibleTicketCount * int64(sum.QuorumPercentage) / 100
	}
	if sum.BestBlock > 0 && sum.EndBlockHeight > sum.BestBlock {
		proposal.BlocksLeft = sum.EndBlockHeight - sum.BestBlock
	}
	return proposal
}

// voteOptionsFromSummary derives the proposal's vote options from the summary's
// per-option results (each carries the option id + vote bit), so the detail view
// can render them without the heavy eligible-ticket snapshot.
func voteOptionsFromSummary(sum piSummary) []types.ProposalVoteOption {
	opts := make([]types.ProposalVoteOption, 0, len(sum.Results))
	for _, r := range sum.Results {
		opts = append(opts, types.ProposalVoteOption{ID: r.ID, Bit: r.VoteBit})
	}
	return opts
}

func summaryStatusToBucket(status string) string {
	switch status {
	case "unauthorized", "authorized":
		return "pre-vote"
	case "started":
		return "voting"
	case "approved", "rejected", "ineligible":
		return "finished"
	case "abandoned":
		return "abandoned"
	}
	if status == "" {
		return "unknown"
	}
	return status
}

func proposalName(rec piRecord) string {
	for _, f := range rec.Files {
		if f.Name == "proposalmetadata.json" {
			raw, err := base64.StdEncoding.DecodeString(f.Payload)
			if err != nil {
				continue
			}
			var meta struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(raw, &meta); err == nil && meta.Name != "" {
				return meta.Name
			}
		}
	}
	return rec.CensorshipRecord.Token
}

// commentsFromPi maps Politeia comments to the frontend shape, sorted oldest
// first. Deleted comments keep their reason but have no displayable text;
// Politeia already blanks the comment body in that case.
func commentsFromPi(in []piComment) []types.ProposalComment {
	out := make([]types.ProposalComment, 0, len(in))
	for _, c := range in {
		pc := types.ProposalComment{
			CommentID: c.CommentID,
			ParentID:  c.ParentID,
			Username:  c.Username,
			CreatedAt: c.CreatedAt,
			Upvotes:   c.Upvotes,
			Downvotes: c.Downvotes,
			Deleted:   c.Deleted,
			Reason:    c.Reason,
		}
		if !c.Deleted {
			pc.Comment = c.Comment
			pc.CommentHTML = renderProposalMarkdown(c.Comment)
		}
		out = append(out, pc)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out
}

func indexMarkdown(rec piRecord) string {
	for _, f := range rec.Files {
		if f.Name == "index.md" {
			raw, err := base64.StdEncoding.DecodeString(f.Payload)
			if err == nil {
				return string(raw)
			}
		}
	}
	return ""
}

// loadLocalPoliteiaVotes returns the map of {token: choice} previously
// cast through this dashboard, or an empty map.
func loadLocalPoliteiaVotes(ctx context.Context) map[string]string {
	network, err := CurrentNetwork(ctx)
	if err != nil || network == "" {
		return nil
	}
	wc, err := config.LoadWalletCfg(network, CurrentWalletName())
	if err != nil {
		return nil
	}
	m, _ := wc.PoliteiaVotes()
	return m
}

func persistLocalPoliteiaVote(ctx context.Context, token, choice string) error {
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return err
	}
	wc, err := config.LoadWalletCfg(network, CurrentWalletName())
	if err != nil {
		return err
	}
	if err := wc.UpsertPoliteiaVote(token, choice); err != nil {
		return err
	}
	return wc.Save()
}
