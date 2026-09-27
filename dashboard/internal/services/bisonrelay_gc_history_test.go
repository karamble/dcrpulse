// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"dcrpulse/internal/rpc"
)

// rawGCLog serves log (oldest first) the way brclientd pages it: page 0 is the
// newest size entries, and a page past the oldest is empty. It counts reads.
func rawGCLog(log ...string) (func(context.Context, rpc.ShortIDHex, int, int) (json.RawMessage, error), *int) {
	calls := new(int)
	return func(_ context.Context, _ rpc.ShortIDHex, page, size int) (json.RawMessage, error) {
		*calls++
		var entries []GCLogEntry
		if end := len(log) - page*size; end > 0 {
			for _, m := range log[max(0, end-size):end] {
				entries = append(entries, GCLogEntry{Message: m, From: "alice"})
			}
		}
		return json.Marshal(map[string]any{"entries": entries})
	}, calls
}

func withRawGCLog(t *testing.T, fetchSize int, log ...string) *int {
	t.Helper()
	fetch, calls := rawGCLog(log...)
	restore := SetGCHistoryFetch(fetch)
	oldSize := gcHistoryFetchSize
	gcHistoryFetchSize = fetchSize
	t.Cleanup(func() { restore(); gcHistoryFetchSize = oldSize })
	return calls
}

func chatPage(t *testing.T, page, size int) string {
	t.Helper()
	id, _ := rpc.ParseShortIDHex(strings.Repeat("ab", 32))
	res, err := GCChatHistory(context.Background(), id, page, size)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range res.Entries {
		out = append(out, e.Message)
	}
	return strings.Join(out, ",")
}

func TestGCHistoryHidesFramesAndStillFillsPages(t *testing.T) {
	withRawGCLog(t, 3, "c1", testFrame, "c2", testFrame, testFrame, testFrame, "c3")
	for page, want := range []string{"c2,c3", "c1", ""} {
		if got := chatPage(t, page, 2); got != want {
			t.Fatalf("page %d = %q, want %q", page, got, want)
		}
	}
}

func TestGCHistoryPagesAWideRead(t *testing.T) {
	withRawGCLog(t, 500, "c1", "c2", testFrame, "c3", "c4", "c5")
	for page, want := range []string{"c4,c5", "c2,c3", "c1", ""} {
		if got := chatPage(t, page, 2); got != want {
			t.Fatalf("page %d = %q, want %q", page, got, want)
		}
	}
}

func TestGCHistoryReadsOnlyAsFarAsThePageNeeds(t *testing.T) {
	calls := withRawGCLog(t, 3, "c1", testFrame, "c2", testFrame, testFrame, testFrame, "c3")
	if got := chatPage(t, 0, 1); got != "c3" {
		t.Fatalf("page 0 = %q", got)
	}
	if *calls != 1 {
		t.Fatalf("read %d raw pages for one entry", *calls)
	}
}
