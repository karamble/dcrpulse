// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"

	gamingwire "github.com/karamble/dcrgaming-sdk/pkg/gaming/wire"

	"dcrpulse/internal/rpc"
)

// GCLogEntry is one entry of a group's history, as brclientd logs it.
type GCLogEntry struct {
	Message   string `json:"message"`
	From      string `json:"from"`
	Timestamp int64  `json:"timestamp"`
	Internal  bool   `json:"internal"`
}

// GCHistoryPage is one page of a group's chat history.
type GCHistoryPage struct {
	GCID     string       `json:"gcid"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Entries  []GCLogEntry `json:"entries"`
}

// Settable so the paging runs without a live brclientd.
var (
	gcHistoryFetch     = rpc.BrclientdGCHistory
	gcHistoryFetchSize = 500
)

// SetGCHistoryFetch replaces brclientd's raw history read and returns a func
// putting it back. It lets tests in other packages pin that a caller goes
// through GCChatHistory; nothing in production calls it.
func SetGCHistoryFetch(f func(context.Context, rpc.ShortIDHex, int, int) (json.RawMessage, error)) (restore func()) {
	prev := gcHistoryFetch
	gcHistoryFetch = f
	return func() { gcHistoryFetch = prev }
}

// gcChatTail returns a group's newest chat entries, oldest first: at least
// want of them, or all there are. Gaming frames are protocol traffic and are
// dropped, which is why the raw log is read here rather than paged by
// brclientd: a group whose newest stretch is all frames would otherwise show
// empty pages.
func gcChatTail(ctx context.Context, gcid rpc.ShortIDHex, want int) ([]GCLogEntry, error) {
	var pages [][]GCLogEntry
	count := 0
	for page := 0; count < want && page < 100000; page++ {
		raw, err := gcHistoryFetch(ctx, gcid, page, gcHistoryFetchSize)
		if err != nil {
			return nil, err
		}
		var got struct {
			Entries []GCLogEntry `json:"entries"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			return nil, err
		}
		kept := got.Entries[:0]
		for _, e := range got.Entries {
			if !gamingwire.IsEnvelope(e.Message) {
				kept = append(kept, e)
			}
		}
		pages = append(pages, kept)
		count += len(kept)
		if len(got.Entries) < gcHistoryFetchSize {
			break
		}
	}
	out := make([]GCLogEntry, 0, count)
	for i := len(pages) - 1; i >= 0; i-- {
		out = append(out, pages[i]...)
	}
	return out, nil
}

// GCChatHistory pages a group's chat the way brclientd pages its log: entries
// oldest first, page 0 the newest pageSize of them, and a page past the oldest
// entry empty.
func GCChatHistory(ctx context.Context, gcid rpc.ShortIDHex, page, pageSize int) (GCHistoryPage, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	pageSize = min(pageSize, 500)
	page = min(max(page, 0), 100000)
	out := GCHistoryPage{GCID: gcid.String(), Page: page, PageSize: pageSize, Entries: []GCLogEntry{}}
	tail, err := gcChatTail(ctx, gcid, (page+1)*pageSize)
	if err != nil {
		return GCHistoryPage{}, err
	}
	if end := len(tail) - page*pageSize; end > 0 {
		out.Entries = tail[max(0, end-pageSize):end]
	}
	return out, nil
}
