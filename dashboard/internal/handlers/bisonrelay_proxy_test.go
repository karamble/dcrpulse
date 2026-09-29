// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// zeros yields n zero bytes without holding them.
type zeros struct{ n int64 }

func (z *zeros) Read(p []byte) (int, error) {
	if z.n <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > z.n {
		p = p[:z.n]
	}
	clear(p)
	z.n -= int64(len(p))
	return len(p), nil
}

func brReply(status int, contentLength int64, body io.Reader, header ...string) *http.Response {
	h := http.Header{}
	for i := 0; i+1 < len(header); i += 2 {
		h.Set(header[i], header[i+1])
	}
	return &http.Response{StatusCode: status, ContentLength: contentLength, Header: h, Body: io.NopCloser(body)}
}

func TestProxyBRBytes(t *testing.T) {
	t.Run("a failed reply passes its status and text on", func(t *testing.T) {
		rec := httptest.NewRecorder()
		proxyBRBytes(rec, brReply(http.StatusNotFound, -1, strings.NewReader("no such file")), 0)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "no such file") {
			t.Fatalf("got %d %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("the type is clamped and named headers are forwarded", func(t *testing.T) {
		rec := httptest.NewRecorder()
		proxyBRBytes(rec, brReply(http.StatusOK, 4, strings.NewReader("<b>x"),
			"Content-Type", "text/html", "Content-Length", "4", "Cache-Control", "max-age=60"), 0, "Content-Length")
		if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
			t.Errorf("Content-Type = %q, want the opaque download type", got)
		}
		if rec.Header().Get("Content-Length") != "4" || rec.Header().Get("Cache-Control") != "" {
			t.Errorf("forwarded headers = %v", rec.Header())
		}
		if rec.Body.String() != "<b>x" {
			t.Errorf("body = %q", rec.Body.String())
		}
	})

	t.Run("a declared length over the cap is refused", func(t *testing.T) {
		rec := httptest.NewRecorder()
		proxyBRBytes(rec, brReply(http.StatusOK, 11, strings.NewReader("hello world")), 10)
		if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "hello") {
			t.Fatalf("got %d %q, want 502 without the bytes", rec.Code, rec.Body.String())
		}
	})

	t.Run("an undeclared stream is cut at the cap", func(t *testing.T) {
		rec := httptest.NewRecorder()
		proxyBRBytes(rec, brReply(http.StatusOK, -1, strings.NewReader("hello world")), 5)
		if rec.Body.String() != "hello" {
			t.Fatalf("body = %q, want the first 5 bytes", rec.Body.String())
		}
	})

	t.Run("the body streams instead of being held", func(t *testing.T) {
		const size = 64 << 20
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		proxyBRBytes(discardWriter{http.Header{}}, brReply(http.StatusOK, size, &zeros{size}), 256<<20)
		runtime.ReadMemStats(&after)
		if grew := after.TotalAlloc - before.TotalAlloc; grew > 8<<20 {
			t.Fatalf("allocated %d bytes to pass %d through", grew, size)
		}
	})
}

// discardWriter is a ResponseWriter that keeps nothing, so only the proxy's own
// allocations are measured.
type discardWriter struct{ h http.Header }

func (d discardWriter) Header() http.Header         { return d.h }
func (d discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (d discardWriter) WriteHeader(int)             {}
