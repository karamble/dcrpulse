// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"dcrpulse/internal/types"
)

// The vote-trickle worker submits a proposal's signed votes spread out over a
// configurable duration instead of in one batch, a port of upstream
// politeiavoter's "trickle" (politeiawww/cmd/politeiavoter/trickle.go). Tor is
// already applied at the dcrpulse stack level (piPost -> ExternalTransport), so
// the value here is the time-spreading. A run is launched per proposal token;
// several proposals can trickle at once. Each run mirrors the autobuyer pattern:
// in-memory state, start/stop, and a status snapshot. Events from every run share
// one 200-entry ring + subscriber set and are tagged with their token so the UI
// can route them to the right per-proposal card.

const (
	voteTrickleEventBufferSize = 200
	voteTrickleMinDuration     = 30 * time.Second
	voteTrickleSubmitAttempts  = 6
)

var (
	vtMu       sync.Mutex
	vtRuns     = map[string]*vtRunState{} // keyed by proposal token; running + finished-not-dismissed
	vtStarting = map[string]bool{}        // tokens whose votes are being signed (guards the slow start)

	vtEventsMu sync.Mutex
	vtEvents   []types.VoteTrickleEvent

	vtSubsMu      sync.Mutex
	vtSubscribers []chan types.VoteTrickleEvent
)

// vtRunState holds one proposal's trickle run. Immutable fields are set once at
// launch; the counters are updated by the per-vote goroutines via atomics. The
// cancel/done/lastErr fields are guarded by vtMu.
type vtRunState struct {
	token        string
	proposalName string
	voteOption   string
	total        int
	startedAt    time.Time
	finishAt     time.Time // latest scheduled vote time (for the countdown)
	durationSecs int64
	sortedSched  []time.Time // submission times, ascending (for nextAt)

	cast   atomic.Int64
	failed atomic.Int64

	cancel  context.CancelFunc // guarded by vtMu
	done    bool               // guarded by vtMu; true once the run goroutine returns
	lastErr string             // guarded by vtMu
}

// IsVoteTrickleRunning reports whether any proposal is currently trickling.
func IsVoteTrickleRunning() bool {
	vtMu.Lock()
	defer vtMu.Unlock()
	for _, st := range vtRuns {
		if !st.done {
			return true
		}
	}
	return false
}

// LastVoteTrickleEvents returns up to n most-recent events, oldest first.
func LastVoteTrickleEvents(n int) []types.VoteTrickleEvent {
	vtEventsMu.Lock()
	defer vtEventsMu.Unlock()
	if n <= 0 || n > len(vtEvents) {
		n = len(vtEvents)
	}
	out := make([]types.VoteTrickleEvent, n)
	copy(out, vtEvents[len(vtEvents)-n:])
	return out
}

// SubscribeVoteTrickleEvents returns a channel receiving every future event plus
// a cleanup func to call when the subscriber detaches.
func SubscribeVoteTrickleEvents() (<-chan types.VoteTrickleEvent, func()) {
	ch := make(chan types.VoteTrickleEvent, 32)
	vtSubsMu.Lock()
	vtSubscribers = append(vtSubscribers, ch)
	vtSubsMu.Unlock()
	return ch, func() {
		vtSubsMu.Lock()
		defer vtSubsMu.Unlock()
		for i, sub := range vtSubscribers {
			if sub == ch {
				vtSubscribers = append(vtSubscribers[:i], vtSubscribers[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

func recordVoteTrickleEvent(token, level, kind, msg, ticket string) {
	ev := types.VoteTrickleEvent{
		Timestamp: time.Now().UTC(), Token: token, Level: level, Kind: kind, Message: msg, Ticket: ticket,
	}
	vtEventsMu.Lock()
	vtEvents = append(vtEvents, ev)
	if len(vtEvents) > voteTrickleEventBufferSize {
		vtEvents = vtEvents[len(vtEvents)-voteTrickleEventBufferSize:]
	}
	vtEventsMu.Unlock()

	vtSubsMu.Lock()
	for _, sub := range vtSubscribers {
		select {
		case sub <- ev:
		default:
		}
	}
	vtSubsMu.Unlock()
}

// VoteTrickleWorkersSnapshot returns the live status of every trickle run
// (running and finished-but-not-dismissed), oldest first.
func VoteTrickleWorkersSnapshot() []types.VoteTrickleStatus {
	vtMu.Lock()
	defer vtMu.Unlock()
	out := make([]types.VoteTrickleStatus, 0, len(vtRuns))
	for _, st := range vtRuns {
		out = append(out, st.snapshotLocked())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

// snapshotLocked builds a status for one run. Caller holds vtMu.
func (st *vtRunState) snapshotLocked() types.VoteTrickleStatus {
	cast := int(st.cast.Load())
	failed := int(st.failed.Load())
	completed := cast + failed
	pending := st.total - completed
	if pending < 0 {
		pending = 0
	}
	running := !st.done
	var nextAt time.Time
	if running {
		now := time.Now()
		for _, t := range st.sortedSched {
			if t.After(now) {
				nextAt = t
				break
			}
		}
	}
	return types.VoteTrickleStatus{
		Running:      running,
		Token:        st.token,
		ProposalName: st.proposalName,
		VoteOption:   st.voteOption,
		Total:        st.total,
		Cast:         cast,
		Failed:       failed,
		Pending:      pending,
		StartedAt:    st.startedAt,
		NextAt:       nextAt,
		FinishAt:     st.finishAt,
		DurationSecs: st.durationSecs,
		LastError:    st.lastErr,
	}
}

// StartVoteTrickle signs every eligible ticket's vote for a proposal up front
// (one unlock; the wallet is re-locked inside buildSignedVotes before this
// returns) and launches a background worker that submits them one ballot at a
// time on a randomized schedule over `duration`. The worker never holds the
// passphrase. Several proposals can trickle concurrently; a second run for a
// token that is still trickling is rejected.
func StartVoteTrickle(ctx context.Context, token, voteOption string, duration time.Duration, bunches int, passphrase []byte) error {
	if !PoliteiaEnabled() {
		return ErrPoliteiaDisabled
	}

	vtMu.Lock()
	if vtStarting[token] {
		vtMu.Unlock()
		return fmt.Errorf("a vote trickle is already starting for this proposal")
	}
	if st := vtRuns[token]; st != nil && !st.done {
		vtMu.Unlock()
		return fmt.Errorf("a vote trickle is already running for this proposal")
	}
	vtStarting[token] = true
	vtMu.Unlock()
	defer func() {
		vtMu.Lock()
		delete(vtStarting, token)
		vtMu.Unlock()
	}()

	// Sign all votes up front. buildSignedVotes unlocks the owning accounts,
	// signs, and RE-LOCKS them before returning (its deferred relock fires on
	// this return), so the worker below holds only signatures - never the
	// passphrase and no unlocked accounts.
	votes, _, err := buildSignedVotes(ctx, token, voteOption, passphrase)
	if err != nil {
		return err
	}
	if len(votes) == 0 {
		return fmt.Errorf("no eligible tickets to vote on this proposal")
	}

	if duration < voteTrickleMinDuration {
		duration = voteTrickleMinDuration
	}
	if bunches < 1 {
		bunches = 1
	}
	sched := generateVoteSchedule(len(votes), bunches, duration)
	sortedSched := append([]time.Time(nil), sched...)
	sort.Slice(sortedSched, func(i, j int) bool { return sortedSched[i].Before(sortedSched[j]) })
	finishAt := time.Time{}
	if len(sortedSched) > 0 {
		finishAt = sortedSched[len(sortedSched)-1]
	}

	runCtx, cancel := context.WithCancel(context.Background())
	st := &vtRunState{
		token:        token,
		proposalName: proposalNameFromCache(token),
		voteOption:   voteOption,
		total:        len(votes),
		startedAt:    time.Now(),
		finishAt:     finishAt,
		durationSecs: int64(duration / time.Second),
		sortedSched:  sortedSched,
		cancel:       cancel,
	}
	vtMu.Lock()
	vtRuns[token] = st // replaces any finished-not-dismissed run for this token
	vtMu.Unlock()

	recordVoteTrickleEvent(token, "info", "scheduled",
		fmt.Sprintf("Trickling %d vote(s) as %q over %s.", len(votes), voteOption, duration.Round(time.Second)), "")

	go runVoteTrickle(runCtx, st, votes, sched)
	return nil
}

// StopVoteTrickle stops/dismisses one proposal's run. A running run is cancelled
// (it transitions to a finished card that stays until dismissed); a finished run
// is removed. Safe to call for an unknown token.
func StopVoteTrickle(token string) {
	vtMu.Lock()
	st := vtRuns[token]
	if st == nil {
		vtMu.Unlock()
		return
	}
	if st.done {
		delete(vtRuns, token)
		vtMu.Unlock()
		return
	}
	cancel := st.cancel
	vtMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func runVoteTrickle(ctx context.Context, st *vtRunState, votes []piBallotVote, sched []time.Time) {
	var wg sync.WaitGroup
	for i := range votes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			trickleOneVote(ctx, st, votes[i], sched[i])
		}(i)
	}
	wg.Wait()

	cast := int(st.cast.Load())
	failed := int(st.failed.Load())
	stopped := ctx.Err() != nil

	vtMu.Lock()
	st.done = true
	st.cancel = nil
	vtMu.Unlock()

	// Reflect the run in the cached proposal list once, at the end.
	if cast > 0 {
		bumpCachedVoteTally(context.Background(), st.token, st.voteOption, cast)
	}
	if stopped {
		recordVoteTrickleEvent(st.token, "info", "done",
			fmt.Sprintf("Trickle stopped: %d cast, %d failed of %d.", cast, failed, st.total), "")
	} else {
		recordVoteTrickleEvent(st.token, "info", "done",
			fmt.Sprintf("Trickle finished: %d cast, %d failed of %d.", cast, failed, st.total), "")
	}
}

func trickleOneVote(ctx context.Context, st *vtRunState, vote piBallotVote, at time.Time) {
	if err := vtWaitUntil(ctx, at); err != nil {
		return // cancelled before this vote's time
	}
	for attempt := 0; attempt < voteTrickleSubmitAttempts; attempt++ {
		if ctx.Err() != nil {
			return
		}
		if attempt > 0 {
			// Back off a random 3-17s between retries (mirrors politeiavoter).
			if err := vtWaitFor(ctx, randomJitter(3, 17)); err != nil {
				return
			}
		}
		var resp piCastBallotResponse
		if err := piPost(ctx, "/ticketvote/v1/castballot", piCastBallotRequest{Votes: []piBallotVote{vote}}, &resp); err != nil {
			if ctx.Err() != nil {
				return
			}
			recordVoteTrickleEvent(st.token, "warn", "failed",
				fmt.Sprintf("Ticket %s submit error (attempt %d/%d): %v", shortHex(vote.Ticket), attempt+1, voteTrickleSubmitAttempts, err), vote.Ticket)
			continue // transient; retry
		}
		if len(resp.Receipts) > 0 && resp.Receipts[0].ErrorCode != 0 {
			markVoteFailed(st, vote, fmt.Sprintf("rejected: %s", resp.Receipts[0].ErrorMsg))
			return
		}
		markVoteCast(st, vote)
		return
	}
	markVoteFailed(st, vote, "submit failed after retries")
}

func markVoteCast(st *vtRunState, vote piBallotVote) {
	done := st.cast.Add(1) + st.failed.Load()
	recordVoteTrickleEvent(st.token, "info", "cast",
		fmt.Sprintf("Cast vote for ticket %s (%d/%d).", shortHex(vote.Ticket), done, st.total), vote.Ticket)
}

func markVoteFailed(st *vtRunState, vote piBallotVote, reason string) {
	st.failed.Add(1)
	vtMu.Lock()
	st.lastErr = reason
	vtMu.Unlock()
	recordVoteTrickleEvent(st.token, "error", "failed",
		fmt.Sprintf("Ticket %s failed: %s", shortHex(vote.Ticket), reason), vote.Ticket)
}

// generateVoteSchedule returns one submission time per vote, distributed across
// `bunches` random windows over `dur`. Ported from politeiavoter's
// generateVoteAlarm/randomTime: each bunch starts within the first 90% of half
// the duration and ends within the second half; each vote fires at a uniform-
// random time inside its bunch's window.
func generateVoteSchedule(n, bunches int, dur time.Duration) []time.Time {
	if n <= 0 {
		return nil
	}
	if bunches < 1 {
		bunches = 1
	}
	if bunches > n {
		bunches = n
	}
	now := time.Now()
	half := int64(dur / 2)
	starts := make([]time.Time, bunches)
	ends := make([]time.Time, bunches)
	for b := 0; b < bunches; b++ {
		st := cryptoRandInt64(half * 90 / 100)        // [0, 0.9*half]
		et := half + cryptoRandInt64(int64(dur)-half) // [half, dur]
		starts[b] = now.Add(time.Duration(st))
		ends[b] = now.Add(time.Duration(et))
	}
	out := make([]time.Time, n)
	for k := 0; k < n; k++ {
		b := k % bunches
		span := ends[b].Sub(starts[b])
		var off time.Duration
		if span > 0 {
			off = time.Duration(cryptoRandInt64(int64(span)))
		}
		out[k] = starts[b].Add(off)
	}
	return out
}

// cryptoRandInt64 returns a uniform random value in [0, max) using crypto/rand.
func cryptoRandInt64(max int64) int64 {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return 0
	}
	return n.Int64()
}

// randomJitter returns a random duration between min and max seconds inclusive.
func randomJitter(min, max int64) time.Duration {
	if max < min {
		max = min
	}
	return time.Duration(min+cryptoRandInt64(max-min+1)) * time.Second
}

func vtWaitUntil(ctx context.Context, t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		return nil
	}
	return vtWaitFor(ctx, d)
}

func vtWaitFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// proposalNameFromCache best-effort resolves a proposal's display name from the
// cached lists so the status card can show it without a fetch.
func proposalNameFromCache(token string) string {
	piCacheMu.Lock()
	defer piCacheMu.Unlock()
	for _, bucket := range piCachedLists {
		for i := range bucket.list {
			if bucket.list[i].Token == token {
				return bucket.list[i].Name
			}
		}
	}
	return ""
}

func shortHex(s string) string {
	if len(s) > 12 {
		return s[:12] + "..."
	}
	return s
}
