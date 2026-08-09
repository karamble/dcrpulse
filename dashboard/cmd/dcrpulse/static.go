// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// compressibleExts lists the asset types worth gzipping. Fonts and images are
// already compressed, so re-encoding them only spends memory.
var compressibleExts = map[string]bool{
	".js":   true,
	".css":  true,
	".html": true,
	".svg":  true,
	".json": true,
	".txt":  true,
}

// staticServer serves the embedded frontend bundle. The embedded filesystem is
// immutable for the lifetime of the process, so every file is compressed and
// hashed once at startup instead of per request. embed.FS reports a zero
// ModTime, which leaves net/http without a Last-Modified to emit and revalidation
// with nothing to compare; the synthesized ETags stand in for it.
type staticServer struct {
	fsys  fs.FS
	files http.Handler
	gz    map[string][]byte
	etag  map[string]string
}

func newStaticServer(fsys fs.FS) *staticServer {
	s := &staticServer{
		fsys:  fsys,
		files: http.FileServer(http.FS(fsys)),
		gz:    make(map[string][]byte),
		etag:  make(map[string]string),
	}

	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil
		}

		sum := sha256.Sum256(raw)
		s.etag[p] = `W/"` + hex.EncodeToString(sum[:8]) + `"`

		if !compressibleExts[strings.ToLower(path.Ext(p))] {
			return nil
		}
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return nil
		}
		if _, err := zw.Write(raw); err != nil {
			return nil
		}
		if err := zw.Close(); err != nil {
			return nil
		}
		if buf.Len() < len(raw) {
			s.gz[p] = buf.Bytes()
		}
		return nil
	})

	return s
}

func (s *staticServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" || p == "." {
		p = "index.html"
	}

	// Vite content-hashes every filename it emits under assets/, so those URLs
	// can never name a different body than the one served now.
	if strings.HasPrefix(p, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	etag, ok := s.etag[p]
	if !ok {
		// Directories never serve: without this, http.FileServer would render
		// a listing for any directory path that reached here.
		if fi, err := fs.Stat(s.fsys, p); err == nil && fi.IsDir() {
			http.NotFound(w, r)
			return
		}
		s.files.ServeHTTP(w, r)
		return
	}
	w.Header().Set("ETag", etag)

	gz, hasGz := s.gz[p]
	if hasGz {
		w.Header().Set("Vary", "Accept-Encoding")
	}

	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if !hasGz || !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		s.files.ServeHTTP(w, r)
		return
	}

	ct := mime.TypeByExtension(path.Ext(p))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Length", strconv.Itoa(len(gz)))
	if r.Method == http.MethodHead {
		return
	}
	w.Write(gz)
}
