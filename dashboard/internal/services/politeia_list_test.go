// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A record that failed to load carries an empty censorship token, so the name
// has to come from the token the caller already holds.
func TestProposalNameFallsBackToCallerToken(t *testing.T) {
	const token = "aaaabbbbccccdddd"

	meta := piRecord{
		CensorshipRecord: piCensorship{Token: token},
		Files: []piFile{{
			Name: "proposalmetadata.json",
			// {"name":"A real proposal"}
			Payload: "eyJuYW1lIjoiQSByZWFsIHByb3Bvc2FsIn0=",
		}},
	}
	if got := proposalName(token, meta); got != "A real proposal" {
		t.Errorf("named proposal = %q, want the metadata name", got)
	}

	noMeta := piRecord{CensorshipRecord: piCensorship{Token: token}}
	if got := proposalName(token, noMeta); got != token {
		t.Errorf("nameless record = %q, want the censorship token", got)
	}

	// The case that matters: the chunk failed, so the record is zero-valued.
	if got := proposalName(token, piRecord{}); got != token {
		t.Errorf("missing record = %q, want the caller's token", got)
	}
}

// piRouter points piHTTPClient at a server that dispatches on the request path,
// reusing the piRedirect transport from politeia_timeout_test.go.
func piRouter(t *testing.T, routes map[string]http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, ok := routes[r.URL.Path]
		if !ok {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)

	prev := piHTTPClient
	piHTTPClient = &http.Client{Transport: piRedirect{to: srv.URL, rt: srv.Client().Transport}}
	t.Cleanup(func() { piHTTPClient = prev })
}

func resetPiListCache(t *testing.T) {
	t.Helper()
	clear := func() {
		piCacheMu.Lock()
		piCachedLists = map[string]piListCacheEntry{}
		piCacheMu.Unlock()
		piListFetchMu.Lock()
		piListFailAt = map[string]time.Time{}
		piListFetchMu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

// Seven tokens chunk into two requests, [t1..t5] and [t6 t7], so one chunk can
// fail while the other succeeds. That is the case the guards missed: with a
// single chunk, a summaries failure empties the map and the pre-existing
// len(summaries) == 0 guard catches it, proving nothing about the new one.
var piTestTokens = []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7"}

// failChunkToken marks the second chunk; a request carrying it is refused.
const failChunkToken = "t6"

func piTestInventoryJSON() string {
	return fmt.Sprintf(`{"vetted":{"approved":[%q,%q,%q,%q,%q,%q,%q]},"bestblock":100}`,
		"t1", "t2", "t3", "t4", "t5", "t6", "t7")
}

// piRecordsHandler answers with a record per requested token, or 500 when the
// chunk contains failToken.
func piRecordsHandler(failToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if failToken != "" && strings.Contains(string(body), `"`+failToken+`"`) {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		var req struct {
			Requests []struct {
				Token string `json:"token"`
			} `json:"requests"`
		}
		_ = json.Unmarshal(body, &req)
		out := piRecordsResp{Records: map[string]piRecord{}}
		for _, q := range req.Requests {
			out.Records[q.Token] = piRecord{
				Username:         "alice",
				CensorshipRecord: piCensorship{Token: q.Token},
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}

// piSummariesHandler answers with a summary per requested token, or 500 when the
// chunk contains failToken.
func piSummariesHandler(failToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if failToken != "" && strings.Contains(string(body), `"`+failToken+`"`) {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		var req struct {
			Tokens []string `json:"tokens"`
		}
		_ = json.Unmarshal(body, &req)
		out := piSummariesResp{Summaries: map[string]piSummary{}}
		for _, tok := range req.Tokens {
			out.Summaries[tok] = piSummary{Status: 5, EndBlockHeight: 10, BestBlock: 100}
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}

func piInventoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(piTestInventoryJSON()))
	}
}

// Each chunk fires two independent requests and only logs their errors. piPost
// gives every request its own timeout, so one chunk can fail while the parent
// context stays healthy - and the list cache has no TTL, so a partial result
// would be served until someone forced a refresh.
func TestPartialRecordsFetchIsNotCached(t *testing.T) {
	resetPiListCache(t)
	piRouter(t, map[string]http.HandlerFunc{
		"/api/ticketvote/v1/inventory": piInventoryHandler(),
		"/api/ticketvote/v1/summaries": piSummariesHandler(""),
		"/api/records/v1/records":      piRecordsHandler(failChunkToken),
	})

	_, _, err := fetchAndCacheProposals(context.Background(), "finished")
	if err == nil {
		t.Fatal("a partly failed records fetch was reported as success")
	}

	piCacheMu.RLock()
	_, cached := piCachedLists["finished"]
	piCacheMu.RUnlock()
	if cached {
		t.Error("a partial fetch was written to the cache, where it would persist")
	}
}

// The mirror case the finding did not mention: a zero-valued summary yields an
// "unknown" status with no vote counts, cached just as indefinitely. Only one
// chunk fails here, so the summaries map is non-empty and the older guard does
// not fire.
func TestPartialSummariesFetchIsNotCached(t *testing.T) {
	resetPiListCache(t)
	piRouter(t, map[string]http.HandlerFunc{
		"/api/ticketvote/v1/inventory": piInventoryHandler(),
		"/api/ticketvote/v1/summaries": piSummariesHandler(failChunkToken),
		"/api/records/v1/records":      piRecordsHandler(""),
	})

	_, _, err := fetchAndCacheProposals(context.Background(), "finished")
	if err == nil {
		t.Fatal("a partly failed summaries fetch was reported as success")
	}

	piCacheMu.RLock()
	_, cached := piCachedLists["finished"]
	piCacheMu.RUnlock()
	if cached {
		t.Error("a partial fetch was written to the cache, where it would persist")
	}
}

// A clean fetch must still cache, or the guard above would have broken the
// feature rather than the bug.
func TestCompleteFetchIsCached(t *testing.T) {
	resetPiListCache(t)
	piRouter(t, map[string]http.HandlerFunc{
		"/api/ticketvote/v1/inventory": piInventoryHandler(),
		"/api/ticketvote/v1/summaries": piSummariesHandler(""),
		"/api/records/v1/records":      piRecordsHandler(""),
	})

	list, _, err := fetchAndCacheProposals(context.Background(), "finished")
	if err != nil {
		t.Fatalf("clean fetch: %v", err)
	}
	if len(list) != len(piTestTokens) {
		t.Fatalf("got %d proposals, want %d", len(list), len(piTestTokens))
	}
	piCacheMu.RLock()
	_, cached := piCachedLists["finished"]
	piCacheMu.RUnlock()
	if !cached {
		t.Error("a complete fetch was not cached")
	}
}
