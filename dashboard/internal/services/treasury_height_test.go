// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"context"
	"encoding/hex"
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
	"github.com/decred/dcrd/wire"
)

type treasuryScanSnapshot struct {
	Running                     bool
	Start, Current, Total, Safe int64
	Found, TAddFound, Failed    int
	Results, Buffer             []types.TSpendHistory
	TAdds                       []types.TreasuryTAdd
	TBase                       map[string]int64
}

func scanSnapshot() treasuryScanSnapshot {
	scanMutex.RLock()
	defer scanMutex.RUnlock()
	tbase := make(map[string]int64, len(scanTBase))
	for k, v := range scanTBase {
		tbase[k] = v
	}
	return treasuryScanSnapshot{isScanRunning, scanStartHeight, currentScanHeight, totalScanHeight, scanSafeHeight,
		tspendFoundCount, taddFoundCount, scanFailedCount,
		append([]types.TSpendHistory(nil), scanResults...), append([]types.TSpendHistory(nil), newTSpendBuffer...),
		append([]types.TreasuryTAdd(nil), scanTAdds...), tbase}
}
func setScanSnapshot(s treasuryScanSnapshot) {
	scanMutex.Lock()
	defer scanMutex.Unlock()
	isScanRunning, scanStartHeight, currentScanHeight, totalScanHeight, scanSafeHeight = s.Running, s.Start, s.Current, s.Total, s.Safe
	tspendFoundCount, taddFoundCount, scanFailedCount = s.Found, s.TAddFound, s.Failed
	scanResults, newTSpendBuffer, scanTAdds, scanTBase = s.Results, s.Buffer, s.TAdds, s.TBase
}
func seedScan(t *testing.T) treasuryScanSnapshot {
	t.Helper()
	old := scanSnapshot()
	t.Cleanup(func() { setScanSnapshot(old) })
	networkMu.Lock()
	oldNet := networkVal
	networkVal = "mainnet"
	networkMu.Unlock()
	t.Cleanup(func() { networkMu.Lock(); networkVal = oldNet; networkMu.Unlock() })
	s := treasuryScanSnapshot{Start: 599990, Current: 600000, Total: 600100, Safe: 599999, Found: 1, TAddFound: 1, Failed: 1,
		Results: []types.TSpendHistory{{TxHash: "prior"}}, Buffer: []types.TSpendHistory{{TxHash: "buffered"}},
		TAdds: []types.TreasuryTAdd{{TxHash: "prior add"}}, TBase: map[string]int64{"2021-05": 7}}
	setScanSnapshot(s)
	return s
}

// scanTBaseAtoms is the treasurybase every fake block pays; scanBlockTime puts
// block h at a minute past activation per height, from 2021-05-26 01:00 UTC.
const scanTBaseAtoms = 54683468

func scanBlockTime(h int64) int64 {
	return time.Date(2021, time.May, 26, 1, 0, 0, 0, time.UTC).Unix() + 60*(h-TreasuryActivationHeight)
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
		if q.Method == "gettreasurybalance" || q.Method == "getblockheader" {
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
			case "gettreasurybalance":
				result = map[string]any{"hash": fmt.Sprintf("%064x", h), "height": h, "balance": 0, "updates": []int64{scanTBaseAtoms}}
			case "getblockheader":
				hdr := wire.BlockHeader{Height: uint32(h), Timestamp: time.Unix(scanBlockTime(h), 0)}
				b, _ := hdr.Bytes()
				result = hex.EncodeToString(b)
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
	act := int64(TreasuryActivationHeight)
	for _, tc := range []struct {
		name       string
		start, tip int64
		want       []int64
	}{
		{"default", 0, act + 2, []int64{act, act + 1, act + 2}}, {"negative", -10, act + 1, []int64{act, act + 1}},
		{"below activation", 1, act, []int64{act}}, {"inside", act + 5, act + 7, []int64{act + 5, act + 6, act + 7}},
		{"tip", act + 9, act + 9, []int64{act + 9}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seedScan(t)
			reads := treasuryRPC(t, tc.tip, nil)
			for i := 0; i < 2; i++ {
				if err := TriggerHistoricalScan(context.Background(), tc.start); err != nil {
					t.Fatal(err)
				}
				s := awaitTreasuryScan(t)
				if s.Current != tc.tip || s.Safe != tc.tip || s.Total != tc.tip || s.Failed != 0 {
					t.Fatalf("bad progress %+v", s)
				}
				wantTBase := map[string]int64{"2021-05": scanTBaseAtoms * int64(len(tc.want))}
				if s.Found != 0 || len(s.Results) != 0 || len(s.TAdds) != 0 || !reflect.DeepEqual(s.TBase, wantTBase) {
					t.Fatalf("results not replaced correctly: %+v", s)
				}
				r := GetScanResults()
				if r.FromHeight != tc.want[0] || r.ToHeight != tc.tip {
					t.Fatalf("results cover %d-%d, want %d-%d", r.FromHeight, r.ToHeight, tc.want[0], tc.tip)
				}
			}
			want := append(append([]int64(nil), tc.want...), tc.want...)
			if !reflect.DeepEqual(reads(), want) {
				t.Fatalf("reads %v want %v", reads(), want)
			}
		})
	}
}
func TestTreasuryScanCountsEachBlockInItsOwnMonth(t *testing.T) {
	seedScan(t)
	// Block times advance a minute per height, so June 2021 starts at this height.
	june := TreasuryActivationHeight + (time.Date(2021, time.June, 1, 0, 0, 0, 0, time.UTC).Unix()-scanBlockTime(TreasuryActivationHeight))/60
	treasuryRPC(t, june+1, nil)
	if err := TriggerHistoricalScan(context.Background(), june-2); err != nil {
		t.Fatal(err)
	}
	awaitTreasuryScan(t)
	got := GetScanResults().TBaseByMonth
	want := map[string]int64{"2021-05": 2 * scanTBaseAtoms, "2021-06": 2 * scanTBaseAtoms}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("block reward by month = %v, want %v", got, want)
	}
}
func TestTreasuryScanStopsAtAnUnreadableBlock(t *testing.T) {
	seedScan(t)
	first := int64(TreasuryActivationHeight + 10)
	reads := treasuryRPC(t, first+5, func(m string, h int64) (any, bool) {
		if m == "getblockhash" && h == first+1 {
			return errors.New("unread block"), true
		}
		return nil, false
	})
	if err := TriggerHistoricalScan(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	s := awaitTreasuryScan(t)
	want := []int64{first, first + 1, first + 1, first + 1}
	if !reflect.DeepEqual(reads(), want) || s.Failed != 1 || s.Safe != first {
		t.Fatalf("reads=%v state=%+v", reads(), s)
	}
	r := GetScanResults()
	if r.ToHeight != first || r.TBaseByMonth["2021-05"] != scanTBaseAtoms {
		t.Fatalf("results %+v, want only block %d", r, first)
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
	if len(reads()) != 2 || s.Failed != 0 || s.Safe != 552672 || s.TBase["2021-05"] != scanTBaseAtoms {
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
	if s.Failed != 0 || s.TBase["2021-05"] != scanTBaseAtoms {
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
