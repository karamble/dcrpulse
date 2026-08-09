// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testDist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":     {Data: []byte("<html>app</html>")},
		"assets/app.js":  {Data: []byte("console.log(1)")},
		"assets/app.css": {Data: []byte("body{}")},
	}
}

// A directory path must never render http.FileServer's listing.
func TestStaticServerRefusesDirectories(t *testing.T) {
	srv := newStaticServer(testDist())
	for _, p := range []string{"/assets", "/assets/"} {
		req := httptest.NewRequest("GET", p, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code == 200 && strings.Contains(rec.Body.String(), "app.js") {
			t.Fatalf("%s served a directory listing", p)
		}
		if rec.Code != 404 && rec.Code != 301 {
			t.Fatalf("%s = %d, want 404 (or FileServer's own 301)", p, rec.Code)
		}
	}
}

// Known files keep serving, with their ETag.
func TestStaticServerServesFiles(t *testing.T) {
	srv := newStaticServer(testDist())
	req := httptest.NewRequest("GET", "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("asset = %d, want 200", rec.Code)
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("asset served without an ETag")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("asset Cache-Control = %q, want immutable", cc)
	}
}
