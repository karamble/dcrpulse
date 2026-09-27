// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/companyzero/bisonrelay/clientrpc/types"
	"google.golang.org/protobuf/encoding/protojson"
)

// The acker exists because acknowledging from the stream callback deadlocked
// the WebSocket client: the callback runs on the read goroutine, and an ack
// blocks there waiting for a response only that goroutine can deliver. These
// tests pin the behaviour the replacement relies on, which is that brclientd's
// acks are cumulative, so one call for the highest id stands in for a burst.

func TestAckTrackerCoalescesABurst(t *testing.T) {
	a := newAckTracker()

	for _, seq := range []int64{1, 2, 3, 4, 5} {
		a.record(seq)
	}

	// One wake-up for the whole burst, not five.
	if got := len(a.pending); got != 1 {
		t.Errorf("pending wake-ups = %d, want 1", got)
	}
	seq, ok := a.next()
	if !ok {
		t.Fatal("next() reported nothing to ack after a burst")
	}
	if seq != 5 {
		t.Errorf("ack id = %d, want the highest of the burst, 5", seq)
	}

	// Acking the high-water mark settles the whole burst.
	a.confirm(seq)
	if _, ok := a.next(); ok {
		t.Error("next() still wants an ack after the highest id was confirmed")
	}
}

func TestAckTrackerMarkNeverRegresses(t *testing.T) {
	a := newAckTracker()

	a.record(10)
	a.record(4) // out of order, must not pull the mark back
	a.record(7)

	seq, ok := a.next()
	if !ok {
		t.Fatal("next() reported nothing to ack")
	}
	if seq != 10 {
		t.Errorf("ack id = %d, want 10; a lower id must not regress the mark", seq)
	}
}

func TestAckTrackerRetriesAfterFailure(t *testing.T) {
	a := newAckTracker()
	a.record(3)

	seq, ok := a.next()
	if !ok || seq != 3 {
		t.Fatalf("next() = (%d, %v), want (3, true)", seq, ok)
	}

	// A failed ack is not confirmed, so the same id is still owed.
	if seq, ok := a.next(); !ok || seq != 3 {
		t.Errorf("after a failed ack next() = (%d, %v), want (3, true)", seq, ok)
	}

	a.confirm(3)
	if _, ok := a.next(); ok {
		t.Error("next() still wants an ack after 3 was confirmed")
	}
}

func TestAckTrackerConfirmIgnoresStaleAck(t *testing.T) {
	a := newAckTracker()
	a.record(9)
	a.confirm(9)

	// A late confirmation for an older id must not reopen settled ground.
	a.confirm(2)
	if _, ok := a.next(); ok {
		t.Error("a stale confirm reopened an already acked range")
	}
}

// The recorder runs on the client's read goroutine while the acker reads it
// from another. Run with -race.
func TestAckTrackerConcurrentRecordAndDrain(t *testing.T) {
	a := newAckTracker()

	const events = 500
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := int64(1); i <= events; i++ {
			a.record(i)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < events; i++ {
			if seq, ok := a.next(); ok {
				a.confirm(seq)
			}
		}
	}()

	wg.Wait()

	// Whatever the interleaving, the mark must have reached the last event and
	// must never exceed it.
	a.record(events)
	seq, ok := a.next()
	if ok && seq > events {
		t.Errorf("ack id = %d, want no more than %d", seq, events)
	}
	a.mu.Lock()
	highest := a.highest
	a.mu.Unlock()
	if highest != events {
		t.Errorf("highest = %d, want %d", highest, events)
	}
}

func TestSequenceIDOf(t *testing.T) {
	if seq, ok := sequenceIDOf(map[string]int64{"sequenceId": 42}); !ok || seq != 42 {
		t.Errorf("sequenceIDOf = (%d, %v), want (42, true)", seq, ok)
	}
	if _, ok := sequenceIDOf(map[string]int64{"other": 1}); ok {
		t.Error("sequenceIDOf accepted a map without a sequenceId")
	}
	if _, ok := sequenceIDOf("not a map"); ok {
		t.Error("sequenceIDOf accepted a non-map")
	}
}

// The ack parameters come from events exactly as Bison Relay's clientrpc
// writes them, where a 64-bit id is a JSON string.
func TestAckBySequenceIDReadsBisonRelayEvents(t *testing.T) {
	kx, err := protojson.Marshal(&types.KXCompleted{SequenceId: 7})
	if err != nil {
		t.Fatal(err)
	}
	dl, err := protojson.Marshal(&types.DownloadCompletedResponse{SequenceId: 9})
	if err != nil {
		t.Fatal(err)
	}
	for payload, want := range map[string]int64{
		string(kx):         7,
		string(dl):         9,
		`{"sequenceId":5}`: 5,
	} {
		p, ok := ackBySequenceID(json.RawMessage(payload))
		if seq, got := sequenceIDOf(p); !ok || !got || seq != want {
			t.Errorf("%s: ack = %v, %v; want sequenceId %d", payload, p, ok, want)
		}
	}
	for _, payload := range []string{`{}`, `{"sequenceId":"0"}`, `{"sequenceId":"x"}`, `not json`} {
		if p, ok := ackBySequenceID(json.RawMessage(payload)); ok {
			t.Errorf("%s: acked %v", payload, p)
		}
	}
}

const testFrame = `--gaming[v=2,game=poker,gv=1,sid=0123456789abcdef,mid=5736684151c34f0a17823de6822769dfafeb3170477c2079dec9d72e35aa5c5f,seq=1/1,exp=1783000000]--eyJhY3Rpb24iOiJmb2xkIn0=`

func TestGCMessageGamingFramesStayOutOfTheBrowser(t *testing.T) {
	frame, err := json.Marshal(map[string]string{"gcid": "aa", "from": "bb", "message": testFrame})
	if err != nil {
		t.Fatal(err)
	}
	// An unknown game's envelope is still protocol traffic, not chat.
	if !gcMessageIsGamingFrame(frame) {
		t.Fatal("a gaming envelope in a gc-message would reach the browser")
	}
	chat, err := json.Marshal(map[string]string{"gcid": "aa", "from": "bb", "message": "ordinary chat"})
	if err != nil {
		t.Fatal(err)
	}
	if gcMessageIsGamingFrame(chat) {
		t.Fatal("ordinary chat was taken for a gaming frame")
	}
	if gcMessageIsGamingFrame(json.RawMessage(`{`)) {
		t.Fatal("an undecodable event was taken for a gaming frame")
	}
}
