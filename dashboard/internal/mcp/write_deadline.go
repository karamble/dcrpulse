// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"net/http"
	"time"
)

// listenWriteTimeout bounds one write to an agent. Notifications are a couple of
// hundred bytes, so this is only ever reached by a peer that has stopped reading
// long enough to fill its receive window. A var so tests can shorten it.
var listenWriteTimeout = 10 * time.Second

// writeDeadlineRate extends the bound for large bodies, so a file or a long
// transaction listing is not cut off just for being big on a slow link. At this
// rate 16 MiB gets a little over two minutes.
const writeDeadlineRate = 128 << 10

// boundedWriter puts a deadline on the agent's socket. Without one, a client
// that stops reading parks the goroutine that is writing to it forever, and that
// goroutine is shared: the resource feeds walk every agent in turn, so one
// unread stream stops notifications for all of them, and the write holds a lock
// the surface needs to shut the stream down afterwards.
type boundedWriter struct {
	http.ResponseWriter
	rc     *http.ResponseController
	failed bool
}

// boundWrites installs the deadline for every response on the agent listener.
func boundWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&boundedWriter{ResponseWriter: w, rc: http.NewResponseController(w)}, r)
	})
}

// Unwrap keeps the rest of the ResponseController surface working, since it
// reaches the real writer by unwrapping.
func (b *boundedWriter) Unwrap() http.ResponseWriter { return b.ResponseWriter }

func (b *boundedWriter) deadline(n int) time.Time {
	return time.Now().Add(listenWriteTimeout + time.Duration(n/writeDeadlineRate)*time.Second)
}

// Write arms the deadline and leaves it armed. A small body only reaches
// net/http's buffer here; the socket write happens in the flush below, and that
// is the one that blocks on a peer which has stopped reading. The flush arms its
// own deadline as well, so either alone bounds the write; keeping both means a
// later edit to one of them cannot quietly leave the socket unbounded.
func (b *boundedWriter) Write(p []byte) (int, error) {
	if b.failed {
		return 0, http.ErrHandlerTimeout
	}
	_ = b.rc.SetWriteDeadline(b.deadline(len(p)))
	n, err := b.ResponseWriter.Write(p)
	if err != nil {
		b.failed = true
	}
	return n, err
}

// FlushError is where a stalled peer is actually caught. It has to exist on this
// type: a ResponseController walks past a wrapper that only unwraps, and would
// flush the real writer with no deadline at all.
func (b *boundedWriter) FlushError() error {
	if b.failed {
		return http.ErrHandlerTimeout
	}
	_ = b.rc.SetWriteDeadline(b.deadline(0))
	if err := b.rc.Flush(); err != nil {
		// Once a write has timed out the peer is gone and the connection is
		// being torn down. Staying failed matters: the stream's own response and
		// the next feed event both still come through here, and re-arming would
		// park each of them for another full deadline.
		b.failed = true
		mcpLog.Warnf("Dropping an agent stream: it stopped reading and the write timed out: %v", err)
		return err
	}
	_ = b.rc.SetWriteDeadline(time.Time{})
	return nil
}

func (b *boundedWriter) Flush() { _ = b.FlushError() }
