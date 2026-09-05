// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

// fakeConnWriter records every deadline the wrapper sets. httptest's recorder
// cannot be used here: it has no SetWriteDeadline, so a ResponseController
// returns ErrNotSupported and a test built on it asserts nothing at all.
type fakeConnWriter struct {
	deadlines []time.Time
	writes    []int
	flushes   int
	writeErr  error
	flushErr  error
}

func (f *fakeConnWriter) Header() http.Header { return http.Header{} }
func (f *fakeConnWriter) WriteHeader(int)     {}

func (f *fakeConnWriter) Write(p []byte) (int, error) {
	f.writes = append(f.writes, len(p))
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}

func (f *fakeConnWriter) FlushError() error {
	f.flushes++
	return f.flushErr
}

func (f *fakeConnWriter) SetWriteDeadline(t time.Time) error {
	f.deadlines = append(f.deadlines, t)
	return nil
}

func (f *fakeConnWriter) SetReadDeadline(t time.Time) error { return nil }

func wrapFake(f *fakeConnWriter) *boundedWriter {
	return &boundedWriter{ResponseWriter: f, rc: http.NewResponseController(f)}
}

// The socket write happens in the flush, not in Write: a notification is a few
// hundred bytes and only reaches net/http's buffer. So the deadline has to still
// be armed when the flush runs, and may only be cleared once that flush has
// succeeded. Clearing it on the way out of Write would leave the write that
// actually blocks completely unbounded.
func TestBoundedWriterArmsAcrossFlush(t *testing.T) {
	f := &fakeConnWriter{}
	b := wrapFake(f)

	if _, err := b.Write([]byte("event")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(f.deadlines) != 1 {
		t.Fatalf("Write set %d deadlines, want 1", len(f.deadlines))
	}
	if f.deadlines[0].IsZero() {
		t.Fatal("Write armed no deadline")
	}
	if len(f.deadlines) > 1 {
		t.Fatal("Write cleared the deadline; the flush that follows would be unbounded")
	}

	if err := http.NewResponseController(b).Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if f.flushes != 1 {
		t.Fatalf("the flush reached the inner writer %d times, want 1", f.flushes)
	}
	if len(f.deadlines) != 3 {
		t.Fatalf("deadlines set: %d, want 3 (write, flush, clear)", len(f.deadlines))
	}
	if f.deadlines[1].IsZero() {
		t.Error("the flush ran with no deadline armed, which is the whole point of the wrapper")
	}
	if !f.deadlines[2].IsZero() {
		t.Error("the deadline was not cleared after a successful flush; a later idle stream would be cut off")
	}
}

// A ResponseController walks past a wrapper that only unwraps, so the wrapper
// has to answer Flush itself. This pins that a controller built on the wrapper
// reaches the wrapper's own flush rather than the writer underneath it.
func TestBoundedWriterInterceptsControllerFlush(t *testing.T) {
	f := &fakeConnWriter{}
	b := wrapFake(f)

	if err := http.NewResponseController(b).Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(f.deadlines) == 0 {
		t.Fatal("the controller flushed the inner writer directly, with no deadline armed")
	}
}

// Unwrap has to stay so the rest of the controller surface still reaches the
// real connection.
func TestBoundedWriterUnwraps(t *testing.T) {
	f := &fakeConnWriter{}
	if err := http.NewResponseController(wrapFake(f)).SetReadDeadline(time.Now()); err != nil {
		t.Fatalf("SetReadDeadline did not reach the inner writer: %v", err)
	}
}

// After a write times out the peer is gone and the connection is being torn
// down, but the stream's own response and the next feed event still come through
// here. Re-arming would park each of them for another full deadline.
func TestBoundedWriterGoesSticky(t *testing.T) {
	f := &fakeConnWriter{flushErr: os.ErrDeadlineExceeded}
	b := wrapFake(f)

	if err := b.FlushError(); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("first flush returned %v, want the deadline error", err)
	}
	armed := len(f.deadlines)
	if armed == 0 {
		t.Fatal("the first flush armed no deadline")
	}

	if _, err := b.Write([]byte("next")); err == nil {
		t.Error("a write after the timeout was attempted again instead of failing fast")
	}
	if err := b.FlushError(); err == nil {
		t.Error("a flush after the timeout was attempted again instead of failing fast")
	}
	if f.flushes != 1 {
		t.Errorf("the inner writer was flushed %d times, want 1: the wrapper retried a dead connection", f.flushes)
	}
	if len(f.deadlines) != armed {
		t.Errorf("deadlines set: %d, want %d: re-arming parks the unwind writes for another full timeout",
			len(f.deadlines), armed)
	}
}

// A flat bound would cut off a legitimately large body on a slow link, so the
// deadline grows with the payload.
func TestBoundedWriterScalesWithSize(t *testing.T) {
	small := &fakeConnWriter{}
	if _, err := wrapFake(small).Write([]byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	big := &fakeConnWriter{}
	if _, err := wrapFake(big).Write(make([]byte, 16<<20)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	smallD := time.Until(small.deadlines[0])
	bigD := time.Until(big.deadlines[0])
	if bigD <= smallD+time.Second {
		t.Fatalf("16 MiB got %v and one byte got %v; a flat bound cuts off large bodies", bigD, smallD)
	}
	if smallD > time.Duration(listenWriteTimeout.Load())+time.Second {
		t.Errorf("a one-byte write got %v, want about %v", smallD, time.Duration(listenWriteTimeout.Load()))
	}
}
