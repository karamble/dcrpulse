// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/pkg/bisonw"
)

// The handler answers 202 and posts in a goroutine, so "one bond per host at a
// time" cannot be seen from a response alone. These tests park the post inside
// the seam and watch the guard, the status endpoint, and how often the post ran.

// bondPostSeams installs a session gate that always passes and a bond post that
// runs tx, and returns how many times tx ran. The web client is a zero value:
// its only reader is the seam that replaces the real post.
func bondPostSeams(t *testing.T, tx func(ctx context.Context, host string) error) *atomic.Int32 {
	t.Helper()
	prevSession, prevTx := dexBondSession, postDexBondTx
	dexBondSession = func(http.ResponseWriter) (*bisonw.WebClient, bool) { return new(bisonw.WebClient), true }
	var calls atomic.Int32
	postDexBondTx = func(ctx context.Context, _ *bisonw.WebClient, host string, _ uint64, _ uint32, _ *bool) error {
		calls.Add(1)
		return tx(ctx, host)
	}
	t.Cleanup(func() { dexBondSession, postDexBondTx = prevSession, prevTx })
	return &calls
}

func postBond(t *testing.T, host string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"host": host, "bond": 1})
	w := httptest.NewRecorder()
	PostDcrdexBondHandler(w, httptest.NewRequest(http.MethodPost, "/api/dcrdex/postbond", bytes.NewReader(body)))
	return w
}

func bondStatus(t *testing.T, host string) services.DexBondPost {
	t.Helper()
	w := httptest.NewRecorder()
	PostDcrdexBondStatusHandler(w, httptest.NewRequest(http.MethodGet, "/api/dcrdex/postbond/status?host="+host, nil))
	var s services.DexBondPost
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("status body %q: %v", w.Body.String(), err)
	}
	return s
}

// waitBondPhase polls the status endpoint until the phase is want. The end of a
// post is recorded by the handler's goroutine, so an immediate read after the
// post returns can still see "submitting".
func waitBondPhase(t *testing.T, host, want string) services.DexBondPost {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s := bondStatus(t, host)
		if s.Phase == want {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("phase = %q, want %q", s.Phase, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPostBondRefusesWhileOneIsInFlight(t *testing.T) {
	const host = "inflight.test:7232"
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	calls := bondPostSeams(t, func(context.Context, string) error {
		started <- struct{}{}
		<-release
		return nil
	})

	if w := postBond(t, host); w.Code != http.StatusAccepted {
		t.Fatalf("first post: %d %s", w.Code, w.Body.String())
	}
	<-started

	w := postBond(t, host)
	if w.Code != http.StatusConflict {
		t.Fatalf("second post while the first is in flight: %d, want 409; body %s", w.Code, w.Body.String())
	}
	var refusal struct {
		Phase   string `json:"phase"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &refusal); err != nil || refusal.Phase != "submitting" {
		t.Fatalf("the refusal must carry phase submitting so the client can tell it from the locked 409: %s", w.Body.String())
	}
	if !strings.Contains(refusal.Message, host) {
		t.Fatalf("refusal message %q does not name the host", refusal.Message)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("the bond was posted %d times, want 1", got)
	}

	close(release)
	waitBondPhase(t, host, "broadcast")

	// A further bond after a broadcast is legitimate; the guard is not a one-shot.
	if w := postBond(t, host); w.Code != http.StatusAccepted {
		t.Fatalf("post after broadcast: %d %s", w.Code, w.Body.String())
	}
	waitBondPhase(t, host, "broadcast")
	if got := calls.Load(); got != 2 {
		t.Fatalf("the bond was posted %d times, want 2", got)
	}
}

func TestPostBondGuardIsPerHost(t *testing.T) {
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	bondPostSeams(t, func(_ context.Context, host string) error {
		started <- struct{}{}
		if host == "a.test:7232" {
			<-release
		}
		return nil
	})
	t.Cleanup(func() { close(release) })

	if w := postBond(t, "a.test:7232"); w.Code != http.StatusAccepted {
		t.Fatalf("post to a: %d", w.Code)
	}
	<-started
	if w := postBond(t, "b.test:7232"); w.Code != http.StatusAccepted {
		t.Fatalf("a post in flight to one host blocked another: %d %s", w.Code, w.Body.String())
	}
	waitBondPhase(t, "b.test:7232", "broadcast")
}

func TestPostBondFailureIsReportedAndRetryable(t *testing.T) {
	const host = "fails.test:7232"
	bondPostSeams(t, func(context.Context, string) error { return errors.New("insufficient funds for bond") })

	if w := postBond(t, host); w.Code != http.StatusAccepted {
		t.Fatalf("post: %d", w.Code)
	}
	if s := waitBondPhase(t, host, "error"); s.Error != "insufficient funds for bond" {
		t.Fatalf("error = %q, want bisonw's own text", s.Error)
	}
	if w := postBond(t, host); w.Code != http.StatusAccepted {
		t.Fatalf("retry after a failure: %d %s", w.Code, w.Body.String())
	}
	waitBondPhase(t, host, "error")
}

// bisonw's postbond runs without a request context, so when our ceiling fires
// the bond may still go out. A plain "failed" would invite a second post.
func TestPostBondTimeoutSaysTheBondMayStillBroadcast(t *testing.T) {
	const host = "slow.test:7232"
	bondPostSeams(t, func(context.Context, string) error {
		return fmt.Errorf("bisonw web: %w", context.DeadlineExceeded)
	})

	if w := postBond(t, host); w.Code != http.StatusAccepted {
		t.Fatalf("post: %d", w.Code)
	}
	if s := waitBondPhase(t, host, "error"); !strings.Contains(s.Error, "may still broadcast") {
		t.Fatalf("error = %q, want the may-still-broadcast warning", s.Error)
	}
}

func TestPostBondRefusedSessionChangesNoState(t *testing.T) {
	const host = "locked.test:7232"
	calls := bondPostSeams(t, func(context.Context, string) error { return nil })
	dexBondSession = func(w http.ResponseWriter) (*bisonw.WebClient, bool) {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return nil, false
	}

	if w := postBond(t, host); w.Code != http.StatusConflict {
		t.Fatalf("locked post: %d", w.Code)
	}
	if s := bondStatus(t, host); s.Phase != "none" {
		t.Fatalf("a refused request left the host in phase %q", s.Phase)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("the bond was posted %d times through a refused session", got)
	}
}
