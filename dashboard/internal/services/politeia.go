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
	piCacheMu        sync.RWMutex
	piCachedList     []types.Proposal
	piCachedAt       time.Time
	piCachedDetails  = map[string]piDetailCacheEntry{}
	piHTTPClient     = &http.Client{Timeout: politeiaTimeout}
)

type piDetailCacheEntry struct {
	detail *types.ProposalDetail
	at     time.Time
}

// ListProposals returns the union of all current Politeia proposals
// with their summary tallies. Cached in-process for politeiaCacheTTL.
// Returns nil + ErrPoliteiaDisabled if the toggle is off.
func ListProposals(ctx context.Context) ([]types.Proposal, error) {
	if !PoliteiaEnabled() {
		return nil, ErrPoliteiaDisabled
	}

	piCacheMu.RLock()
	if time.Since(piCachedAt) < politeiaCacheTTL && piCachedList != nil {
		cached := piCachedList
		piCacheMu.RUnlock()
		return cached, nil
	}
	piCacheMu.RUnlock()

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

// GetProposalDetail returns one proposal with full description + vote
// options. Cached in-process for politeiaCacheTTL; cleared on successful
// cast-vote against the same token.
func GetProposalDetail(ctx context.Context, token string) (*types.ProposalDetail, error) {
	if !PoliteiaEnabled() {
		return nil, ErrPoliteiaDisabled
	}
	if token == "" {
		return nil, fmt.Errorf("token required")
	}

	piCacheMu.RLock()
	if entry, ok := piCachedDetails[token]; ok && time.Since(entry.at) < politeiaCacheTTL {
		cached := *entry.detail
		piCacheMu.RUnlock()
		// Local "you voted X" cache might have changed since the entry
		// was warmed; refresh that field from the per-wallet cfg.
		if localVotes := loadLocalPoliteiaVotes(ctx); localVotes != nil {
			if v, ok := localVotes[token]; ok {
				cached.CurrentChoice = v
			}
		}
		return &cached, nil
	}
	piCacheMu.RUnlock()

	records, err := piRecordsBatch(ctx, []string{token})
	if err != nil {
		return nil, err
	}
	rec, ok := records[token]
	if !ok {
		return nil, fmt.Errorf("record not found")
	}
	sum, err := piSummariesBatch(ctx, []string{token})
	if err != nil {
		return nil, err
	}
	// Only fetch the heavy vote-details endpoint (returns the full
	// eligible ticket-hash list) for proposals where voting is active.
	// For finished/abandoned/pre-vote proposals the eligible count
	// from the summary is enough and the ticket snapshot is irrelevant.
	// Matches Decrediton's behaviour: getProposalVoteDetails only fires
	// when the proposal is in the voting state.
	var det *piVoteDetailsResp
	if s, ok := sum[token]; ok && s.Status == 3 {
		d, err := piVoteDetails(ctx, token)
		if err != nil {
			log.Printf("politeia vote details %s: %v", token, err)
		} else {
			det = d
		}
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
	if det != nil {
		for _, o := range det.Vote.Params.Options {
			out.VoteOptions = append(out.VoteOptions, types.ProposalVoteOption{
				ID:  o.ID,
				Bit: o.Bit,
			})
		}
		// Prefer the more authoritative count from vote details when
		// available; the proposal summary's eligibleTicketCount may
		// not include all eligible tickets yet for pre-vote proposals.
		if n := int64(len(det.Vote.EligibleTickets)); n > 0 {
			out.EligibleTickets = n
		}
	}

	piCacheMu.Lock()
	piCachedDetails[token] = piDetailCacheEntry{detail: out, at: time.Now()}
	piCacheMu.Unlock()

	return out, nil
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

	det, err := piVoteDetails(ctx, req.Token)
	if err != nil {
		return nil, fmt.Errorf("vote details: %w", err)
	}
	var voteBit uint32
	var found bool
	for _, o := range det.Vote.Params.Options {
		if o.ID == req.VoteOption {
			voteBit = o.Bit
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("vote option %q not allowed for this proposal", req.VoteOption)
	}

	// Intersect eligible tickets with wallet-owned ones. Ticket hashes
	// from Politeia come in big-endian hex; CommittedTickets expects
	// little-endian bytes.
	eligible := det.Vote.EligibleTickets
	if len(eligible) == 0 {
		return &types.CastPoliteiaVoteResult{}, nil
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
	addrs := owned.GetTicketAddresses()
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
	// next view reflects the updated tallies and currentChoice.
	piCacheMu.Lock()
	piCachedAt = time.Time{}
	delete(piCachedDetails, req.Token)
	piCacheMu.Unlock()

	return result, nil
}

// ---- internal HTTP helpers + decoders -------------------------------------

// ErrPoliteiaDisabled is returned by every public function when the
// politeia toggle in Settings is off. Handlers translate to 503.
var ErrPoliteiaDisabled = fmt.Errorf("politeia disabled in settings")

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
