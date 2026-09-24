// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"dcrpulse/internal/types"
	"github.com/decred/dcrd/rpcclient/v8"
)

type treasuryScanSnapshot struct {
	Running              bool
	Current, Total, Safe int64
	Found, Failed        int
	Results, Buffer      []types.TSpendHistory
}

func scanSnapshot() treasuryScanSnapshot {
	scanMutex.RLock()
	defer scanMutex.RUnlock()
	return treasuryScanSnapshot{isScanRunning, currentScanHeight, totalScanHeight, scanSafeHeight, tspendFoundCount, scanFailedCount, append([]types.TSpendHistory(nil), scanResults...), append([]types.TSpendHistory(nil), newTSpendBuffer...)}
}
func setScanSnapshot(s treasuryScanSnapshot) {
	scanMutex.Lock()
	defer scanMutex.Unlock()
	isScanRunning, currentScanHeight, totalScanHeight, scanSafeHeight = s.Running, s.Current, s.Total, s.Safe
	tspendFoundCount, scanFailedCount = s.Found, s.Failed
	scanResults, newTSpendBuffer = s.Results, s.Buffer
}
func seedScan(t *testing.T) treasuryScanSnapshot {
	t.Helper()
	old := scanSnapshot()
	t.Cleanup(func() { setScanSnapshot(old) })
	s := treasuryScanSnapshot{Current: 600000, Total: 600100, Safe: 599999, Found: 1, Failed: 1, Results: []types.TSpendHistory{{TxHash: "prior"}}, Buffer: []types.TSpendHistory{{TxHash: "buffered"}}}
	setScanSnapshot(s)
	return s
}
func awaitTreasuryScan(t *testing.T) treasuryScanSnapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s := scanSnapshot()
		if !s.Running {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("scan did not release its running flag")
	return treasuryScanSnapshot{}
}

// hook can hold admission/block reads, fail selected requests, or override a tip.
func treasuryRPC(t *testing.T, tip int64, hook func(string, int64) (any, bool)) func() []int64 {
	t.Helper()
	var mu sync.Mutex
	var heights []int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     json.RawMessage
			Method string
			Params []json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
			return
		}
		var h int64
		if q.Method == "getblockhash" {
			json.Unmarshal(q.Params[0], &h)
			mu.Lock()
			heights = append(heights, h)
			mu.Unlock()
		}
		if q.Method == "getblock" {
			var hash string
			json.Unmarshal(q.Params[0], &hash)
			h, _ = strconv.ParseInt(strings.TrimLeft(hash, "0"), 16, 64)
		}
		var result any
		var rpcErr any
		override := false
		if hook != nil {
			result, override = hook(q.Method, h)
		}
		if err, ok := result.(error); ok {
			rpcErr = map[string]any{"code": -1, "message": err.Error()}
			result = nil
		}
		if !override {
			switch q.Method {
			case "getblockcount":
				result = tip
			case "getblockhash":
				result = fmt.Sprintf("%064x", h)
			case "getblock":
				tx := map[string]any{"txid": "spend", "version": 3, "vout": []any{
					map[string]any{"value": 1, "scriptPubKey": map[string]any{"type": "treasurygen"}},
				}}
				result = map[string]any{"height": h, "hash": fmt.Sprintf("%064x", h), "rawstx": []any{tx}}
			default:
				t.Errorf("unexpected RPC %s", q.Method)
			}
		}

		json.NewEncoder(w).Encode(map[string]any{"id": q.ID, "result": result, "error": rpcErr})
	}))
	t.Cleanup(srv.Close)
	c, err := rpcclient.New(&rpcclient.ConnConfig{Host: strings.TrimPrefix(srv.URL, "http://"), User: "u", Pass: "p", HTTPPostMode: true, DisableTLS: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Shutdown)
	withDcrdClient(t, c)
	return func() []int64 { mu.Lock(); defer mu.Unlock(); return append([]int64(nil), heights...) }
}
func TestTreasuryScanRejectsOversizedWithoutMutation(t *testing.T) {
	before := seedScan(t)
	reads := treasuryRPC(t, 600000, nil)
	for _, h := range []int64{600001, math.MaxInt64 - 287, math.MaxInt64 - 1, math.MaxInt64} {
		err := TriggerHistoricalScan(context.Background(), h)
		if !errors.Is(err, ErrInvalidScanHeight) {
			t.Fatalf("height %d: %v", h, err)
		}
		if got := scanSnapshot(); !reflect.DeepEqual(got, before) {
			t.Fatalf("rejection changed state: %+v", got)
		}
	}
	if got := reads(); len(got) != 0 {
		t.Fatalf("invalid scan read blocks %v", got)
	}
}
func TestTreasuryScanAdmissionFailuresPreserveState(t *testing.T) {
	for _, mode := range []string{"nil client", "RPC error", "negative tip", "tip below activation", "cancelled", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			before := seedScan(t)
			ctx := context.Background()
			var cancel context.CancelFunc
			switch mode {
			case "nil client":
				withDcrdClient(t, nil)
			case "RPC error":
				treasuryRPC(t, 600000, func(m string, _ int64) (any, bool) { return errors.New("offline"), true })
			case "negative tip":
				treasuryRPC(t, -1, nil)
			case "tip below activation":
				treasuryRPC(t, TreasuryActivationHeight-1, nil)
			case "cancelled":
				treasuryRPC(t, 600000, nil)
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "deadline":
				treasuryRPC(t, 600000, nil)
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			}
			if err := TriggerHistoricalScan(ctx, 0); err == nil {
				t.Fatal("expected admission failure")
			}
			if got := scanSnapshot(); !reflect.DeepEqual(got, before) {
				t.Fatalf("failure changed state: %+v", got)
			}
		})
	}
}
func TestTreasuryScanRanges(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start, tip int64
		want       []int64
	}{
		{"default", 0, 552700, []int64{552672}}, {"negative", -10, 552700, []int64{552672}}, {"below activation", 1, 552700, []int64{552672}},
		{"activation", 552448, 552700, []int64{552672}}, {"aligned", 552672, 552960, []int64{552672, 552960}},
		{"unaligned", 552673, 552960, []int64{552960}}, {"tip aligned", 552672, 552672, []int64{552672}},
		{"tip unaligned", 552700, 552700, nil}, {"no boundary", 552673, 552700, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seedScan(t)
			reads := treasuryRPC(t, tc.tip, nil)
			for i := 0; i < 2; i++ {
				if err := TriggerHistoricalScan(context.Background(), tc.start); err != nil {
					t.Fatal(err)
				}
				s := awaitTreasuryScan(t)
				if s.Current > tc.tip || s.Safe > tc.tip || s.Total != tc.tip || s.Failed != 0 {
					t.Fatalf("bad progress %+v", s)
				}
				if s.Found != len(tc.want) || len(s.Results) != len(tc.want) || len(s.Buffer) != len(tc.want) {
					t.Fatalf("results not replaced correctly: %+v", s)
				}
			}
			want := append(append([]int64(nil), tc.want...), tc.want...)
			if !reflect.DeepEqual(reads(), want) {
				t.Fatalf("reads %v want %v", reads(), want)
			}
		})
	}
}
func TestTreasuryScanArithmetic(t *testing.T) {
	last := int64(math.MaxInt64) - int64(math.MaxInt64)%TreasuryVoteInterval
	for _, tc := range []struct {
		start, tip, want int64
		ok               bool
	}{
		{last - 1, math.MaxInt64, last, true}, {last, math.MaxInt64, last, true}, {last + 1, math.MaxInt64, 0, false}, {math.MaxInt64, math.MaxInt64, 0, false},
		{-1, math.MaxInt64, 0, false}, {1, -1, 0, false}, {552673, 552700, 0, false},
	} {
		h, ok := firstScanHeight(tc.start, tc.tip)
		if h != tc.want || ok != tc.ok {
			t.Fatalf("first(%d,%d)=(%d,%v)", tc.start, tc.tip, h, ok)
		}
	}
	if _, ok := nextScanHeight(last, math.MaxInt64); ok {
		t.Fatal("wrapped final increment")
	}
	if h, ok := nextScanHeight(last-TreasuryVoteInterval, math.MaxInt64); !ok || h != last {
		t.Fatal("lost valid last interval")
	}
}
func TestTreasuryScanFailedReadStillAdvancesSafely(t *testing.T) {
	for _, last := range []int64{552960, int64(math.MaxInt64) - int64(math.MaxInt64)%TreasuryVoteInterval} {
		t.Run(fmt.Sprint(last), func(t *testing.T) {
			seedScan(t)
			first := last - TreasuryVoteInterval
			reads := treasuryRPC(t, last, func(m string, h int64) (any, bool) {
				if m == "getblockhash" && h == first {
					return errors.New("unread block"), true
				}
				return nil, false
			})
			if err := TriggerHistoricalScan(context.Background(), first); err != nil {
				t.Fatal(err)
			}
			s := awaitTreasuryScan(t)
			want := []int64{first, first, first, last}
			if !reflect.DeepEqual(reads(), want) || s.Failed != 1 || s.Safe != first-1 || s.Found != 1 {
				t.Fatalf("reads=%v state=%+v", reads(), s)
			}
		})
	}
}
func TestTreasuryScanTransientRetry(t *testing.T) {
	seedScan(t)
	attempt := 0
	reads := treasuryRPC(t, 552672, func(m string, _ int64) (any, bool) {
		if m == "getblockhash" {
			attempt++
			if attempt == 1 {
				return errors.New("transient"), true
			}
		}
		return nil, false
	})
	if err := TriggerHistoricalScan(context.Background(), 552672); err != nil {
		t.Fatal(err)
	}
	s := awaitTreasuryScan(t)
	if len(reads()) != 2 || s.Failed != 0 || s.Safe != 552672 || s.Found != 1 {
		t.Fatalf("retry failed: %+v", s)
	}
}
func TestTreasuryScanConcurrentAdmissionAndDetachedWorker(t *testing.T) {
	seedScan(t)
	arrived := make(chan struct{}, 2)
	releaseTip := make(chan struct{})
	releaseBlock := make(chan struct{})
	blockEntered := make(chan struct{}, 1)
	var tipOnce, blockOnce sync.Once
	defer tipOnce.Do(func() { close(releaseTip) })
	defer blockOnce.Do(func() { close(releaseBlock) })
	treasuryRPC(t, 552672, func(m string, _ int64) (any, bool) {
		switch m {
		case "getblockcount":
			arrived <- struct{}{}
			<-releaseTip
		case "getblockhash":
			blockEntered <- struct{}{}
			<-releaseBlock
		}
		return nil, false
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- TriggerHistoricalScan(ctx, 552672) }()
	}
	select {
	case <-arrived:
	case <-time.After(time.Second):
		t.Fatal("tip lookup did not start")
	}
	// rpcclient serializes HTTP requests. While its first lookup is held,
	// progress must remain readable and the second admission may queue.
	readable := make(chan struct{})
	go func() { scanSnapshot(); close(readable) }()
	select {
	case <-readable:
	case <-time.After(time.Second):
		t.Fatal("admission held the state mutex during RPC")
	}

	tipOnce.Do(func() { close(releaseTip) })
	successes := 0
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err == nil {
				successes++
			} else if !strings.Contains(err.Error(), "already in progress") {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("admission blocked")
		}
	}
	if successes != 1 {
		t.Fatalf("%d scans admitted", successes)
	}
	<-blockEntered
	cancel()
	before := scanSnapshot()
	if err := TriggerHistoricalScan(context.Background(), 552672); err == nil {
		t.Fatal("accepted during active scan")
	}
	if !reflect.DeepEqual(scanSnapshot(), before) {
		t.Fatal("busy request changed state")
	}
	blockOnce.Do(func() { close(releaseBlock) })
	s := awaitTreasuryScan(t)
	if s.Failed != 0 || s.Found != 1 {
		t.Fatalf("request cancellation killed accepted worker: %+v", s)
	}
}

func TestTreasuryScanPendingTipDeadline(t *testing.T) {
	before := seedScan(t)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	entered := make(chan struct{})
	treasuryRPC(t, 600000, func(m string, _ int64) (any, bool) {
		if m == "getblockcount" {
			close(entered)
			<-release
		}
		return nil, false
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- TriggerHistoricalScan(ctx, 552672) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("tip lookup missing")
	}
	select {
	case err := <-done:
		if !errors.Is(err, rpcclient.ErrRequestCanceled) || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("tip lookup ignored deadline")
	}
	if !reflect.DeepEqual(scanSnapshot(), before) {
		t.Fatal("timeout changed scan state")
	}
	once.Do(func() { close(release) })
}
