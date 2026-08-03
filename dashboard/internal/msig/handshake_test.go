// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The harness runs several "nodes" (dcrpulse wallets with their own BR
// identities) inside one process by swapping the package seams per
// delivery. Frames queue through a captured transport and are pumped
// synchronously, so every assertion sees a settled state. Key material
// is layered on top by hdHarness, which gives every node a
// deterministic master key and dedicated-account xpubs.

type hsNode struct {
	wallet  string
	uid     string
	nick    string
	imports []string // scriptHex per importScriptSeam call
}

type hsHarness struct {
	t       *testing.T
	nodes   map[string]*hsNode // by uid
	current *hsNode
	queue   []hsFrame
	ctx     context.Context
}

type hsFrame struct {
	from *hsNode
	to   string
	body string
}

func newHsHarness(t *testing.T, names ...string) *hsHarness {
	t.Helper()
	h := &hsHarness{t: t, nodes: make(map[string]*hsNode), ctx: context.Background()}
	base := t.TempDir()

	origWalletDir, origWalletsDir := walletDirFn, walletsDirFn
	origImport, origTip := importScriptSeam, tipHeightSeam
	origSend, origHist := sendPMSeam, msigHistorySeam
	origActive, origNetwork := activeWalletSeam, networkSeam
	t.Cleanup(func() {
		walletDirFn, walletsDirFn = origWalletDir, origWalletsDir
		importScriptSeam, tipHeightSeam = origImport, origTip
		sendPMSeam, msigHistorySeam = origSend, origHist
		activeWalletSeam, networkSeam = origActive, origNetwork
		mgrMu.Lock()
		mgr = nil
		mgrMu.Unlock()
		ladderMu.Lock()
		ladderViews = map[string]*ladderView{}
		ladderMu.Unlock()
	})

	walletDirFn = func(network, walletName string) string {
		return filepath.Join(base, network, walletName)
	}
	walletsDirFn = func(network string) string {
		return filepath.Join(base, network)
	}
	networkSeam = func(context.Context) (string, error) { return "simnet", nil }
	activeWalletSeam = func() string { return h.current.wallet }
	importScriptSeam = func(ctx context.Context, scriptHex string, rescan bool, scanFrom int64) error {
		h.current.imports = append(h.current.imports, scriptHex)
		return nil
	}
	tipHeightSeam = func(context.Context) (int64, error) { return 4242, nil }
	sendPMSeam = func(ctx context.Context, user, msg string) error {
		h.queue = append(h.queue, hsFrame{from: h.current, to: user, body: msg})
		return nil
	}
	msigHistorySeam = func(ctx context.Context, uid string, limit int, since int64) (json.RawMessage, error) {
		return json.RawMessage(`{"local_nick":"","entries":[]}`), nil
	}

	mgrMu.Lock()
	mgr = nil
	mgrMu.Unlock()

	for i, name := range names {
		n := &hsNode{
			wallet: "w-" + name,
			uid:    strings.Repeat(fmt.Sprintf("%02x", i+1), 16),
			nick:   name,
		}
		h.nodes[n.uid] = n
	}
	h.current = h.nodeByNick(names[0])
	return h
}

func (h *hsHarness) nodeByNick(nick string) *hsNode {
	for _, n := range h.nodes {
		if n.nick == nick {
			return n
		}
	}
	h.t.Fatalf("no node %q", nick)
	return nil
}

func (h *hsHarness) as(nick string) *hsNode {
	h.current = h.nodeByNick(nick)
	return h.current
}

func (h *hsHarness) store(nick string) *Store {
	n := h.nodeByNick(nick)
	s, err := manager("simnet").StoreFor(n.wallet)
	if err != nil {
		h.t.Fatalf("store for %s: %v", nick, err)
	}
	return s
}

// pump delivers queued frames until quiet, switching the active node per
// delivery like a wallet switch would.
func (h *hsHarness) pump() {
	for len(h.queue) > 0 {
		f := h.queue[0]
		h.queue = h.queue[1:]
		to, ok := h.nodes[f.to]
		if !ok {
			h.t.Fatalf("frame to unknown uid %s", f.to)
		}
		prev := h.current
		h.current = to
		handleInbound(f.from.uid, f.from.nick, f.body, time.Now())
		h.current = prev
	}
}

// pumpTo delivers only the frames addressed to nick, including ones
// enqueued during delivery; everything else stays queued so a caller can
// drop it to simulate lost frames.
func (h *hsHarness) pumpTo(nick string) {
	target := h.nodeByNick(nick)
	for i := 0; i < len(h.queue); {
		f := h.queue[i]
		if f.to != target.uid {
			i++
			continue
		}
		h.queue = append(h.queue[:i], h.queue[i+1:]...)
		prev := h.current
		h.current = target
		handleInbound(f.from.uid, f.from.nick, f.body, time.Now())
		h.current = prev
	}
}

func (h *hsHarness) record(nick, id string) *WalletRecord {
	rec, ok := h.store(nick).Wallet(id)
	if !ok {
		h.t.Fatalf("%s has no record %s", nick, id)
	}
	return rec
}
