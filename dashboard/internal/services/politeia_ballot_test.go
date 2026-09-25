// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// socksRecorder is a minimal SOCKS5 proxy that records the username of every
// connection it accepts and forwards CONNECT requests.
type socksRecorder struct {
	mu    sync.Mutex
	users []string
}

func (s *socksRecorder) serve(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *socksRecorder) handle(c net.Conn) {
	defer c.Close()
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return
	}
	methods := make([]byte, hdr[1])
	io.ReadFull(c, methods)
	user := ""
	if strings.IndexByte(string(methods), 2) >= 0 {
		c.Write([]byte{5, 2})
		var b [1]byte
		io.ReadFull(c, b[:]) // auth version
		io.ReadFull(c, b[:])
		u := make([]byte, b[0])
		io.ReadFull(c, u)
		io.ReadFull(c, b[:])
		io.ReadFull(c, make([]byte, b[0]))
		c.Write([]byte{1, 0})
		user = string(u)
	} else {
		c.Write([]byte{5, 0})
	}
	s.mu.Lock()
	s.users = append(s.users, user)
	s.mu.Unlock()

	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return
	}
	var host string
	switch req[3] {
	case 1:
		ip := make([]byte, 4)
		io.ReadFull(c, ip)
		host = net.IP(ip).String()
	case 3:
		var n [1]byte
		io.ReadFull(c, n[:])
		name := make([]byte, n[0])
		io.ReadFull(c, name)
		host = string(name)
	default:
		return
	}
	var port [2]byte
	io.ReadFull(c, port[:])
	up, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port[:])))))
	if err != nil {
		return
	}
	defer up.Close()
	c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	go io.Copy(up, c)
	io.Copy(c, up)
}

// Two ballots of a trickle must not share a connection or a Tor circuit, even
// with stream isolation switched off in the settings.
func TestBallotsTakeSeparateTorCircuits(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "tor.json")
	if err := os.WriteFile(settings, []byte(`{"enabled":true,"isolation":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := torSettingsPath
	torSettingsPath = func() string { return settings }
	t.Cleanup(func() { torSettingsPath = prev })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	rec := &socksRecorder{}
	go rec.serve(ln)
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	t.Setenv("TOR_PROXY_IP", host)
	t.Setenv("TOR_PROXY_PORT", port)

	politeia := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"receipts":[]}`))
	}))
	t.Cleanup(politeia.Close)

	client := &http.Client{Transport: newBallotTransport()}
	for i := 0; i < 2; i++ {
		resp, err := client.Post(politeia.URL+"/ticketvote/v1/castballot", "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatalf("ballot %d: %v", i, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.users) != 2 {
		t.Fatalf("proxy saw %d connections for 2 ballots, want 2", len(rec.users))
	}
	if rec.users[0] == "" || rec.users[0] == rec.users[1] {
		t.Errorf("ballot proxy credentials %q, want two distinct isolation credentials", rec.users)
	}
}

type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("%s went over the shared Politeia client", r.URL.Path)
	return nil, http.ErrHandlerTimeout
}

// The trickle hands each ballot to the ballot client, never the shared one.
func TestTrickledBallotUsesTheBallotClient(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(`{"receipts":[{"ticket":"ab"}]}`))
	}))
	t.Cleanup(srv.Close)
	prevShared, prevBallot := piHTTPClient, piBallotClient
	piHTTPClient = &http.Client{Transport: failTransport{t}}
	piBallotClient = &http.Client{Transport: piRedirect{to: srv.URL, rt: srv.Client().Transport}}
	t.Cleanup(func() { piHTTPClient, piBallotClient = prevShared, prevBallot })

	st := &vtRunState{token: "ballot-client-test", total: 1}
	trickleOneVote(context.Background(), st, piBallotVote{Ticket: "ab"}, time.Now())
	if hits.Load() != 1 || st.cast.Load() != 1 {
		t.Fatalf("ballot server hits = %d, cast = %d, want 1 and 1", hits.Load(), st.cast.Load())
	}
}
