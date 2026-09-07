// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"

	"dcrpulse/internal/types"
)

// A synthetic chain behind the three dcrd seams. Every call is counted, and the
// verbose replies carry Confirmations 0, so a confirmation count that comes
// back correct was derived from the tip, not copied from dcrd.
type fakeChain struct {
	mu     sync.Mutex
	tip    int64
	hashes map[int64]string
	counts struct{ count, hash, block int }
}

func chainHash(label string) string {
	h := sha256.Sum256([]byte(label))
	return hex.EncodeToString(h[:])
}

func newFakeChain(t *testing.T, tip int64) *fakeChain {
	t.Helper()
	c := &fakeChain{tip: tip, hashes: map[int64]string{}}
	for h := int64(0); h <= tip; h++ {
		c.hashes[h] = chainHash(fmt.Sprintf("block-%d", h))
	}
	prevCount, prevHash, prevVerbose := blockCountSeam, blockHashSeam, blockVerboseSeam
	blockCountSeam = func(context.Context) (int64, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.counts.count++
		return c.tip, nil
	}
	blockHashSeam = func(_ context.Context, height int64) (*chainhash.Hash, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.counts.hash++
		hs, ok := c.hashes[height]
		if !ok {
			return nil, fmt.Errorf("no block at height %d", height)
		}
		return chainhash.NewHashFromStr(hs)
	}
	blockVerboseSeam = func(_ context.Context, hash *chainhash.Hash) (*chainjson.GetBlockVerboseResult, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.counts.block++
		want := hash.String()
		for h, hs := range c.hashes {
			if hs != want {
				continue
			}
			prev := ""
			if h > 0 {
				prev = c.hashes[h-1]
			}
			return &chainjson.GetBlockVerboseResult{
				Hash: hs, PreviousHash: prev, Height: h, Time: 1_700_000_000 + h*300,
				Confirmations: 0, Tx: []string{"t"}, Size: 1000,
			}, nil
		}
		return nil, fmt.Errorf("unknown block %s", want)
	}
	t.Cleanup(func() {
		blockCountSeam, blockHashSeam, blockVerboseSeam = prevCount, prevHash, prevVerbose
		blockSummaryCache.RetainWhere(func(string, types.BlockSummary) bool { return false })
	})
	return c
}

// spent returns the call counts since the last call and clears them.
func (c *fakeChain) spent() (count, hash, block int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	count, hash, block = c.counts.count, c.counts.hash, c.counts.block
	c.counts.count, c.counts.hash, c.counts.block = 0, 0, 0
	return
}

func (c *fakeChain) setTip(height int64, label string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hashes[height] = chainHash(label)
	c.tip = height
}

func firstPage(t *testing.T) []types.BlockSummary {
	t.Helper()
	resp, err := FetchRecentBlocksPaginated(context.Background(), 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Blocks) != 20 {
		t.Fatalf("page holds %d blocks, want 20", len(resp.Blocks))
	}
	return resp.Blocks
}

func TestRecentBlocksAreFetchedOnce(t *testing.T) {
	c := newFakeChain(t, 100)
	first := firstPage(t)
	if count, hash, block := c.spent(); count != 1 || hash != 1 || block != 20 {
		t.Fatalf("a cold page cost %d count, %d hash, %d block calls; want 1, 1, 20", count, hash, block)
	}
	second := firstPage(t)
	if count, hash, block := c.spent(); count != 1 || hash != 1 || block != 0 {
		t.Fatalf("a warm page cost %d count, %d hash, %d block calls; want 1, 1, 0", count, hash, block)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("block %d differs between the two pages: %+v vs %+v", i, first[i], second[i])
		}
		if want := int64(100) - first[i].Height + 1; first[i].Confirmations != want {
			t.Fatalf("block %d has %d confirmations, want %d (dcrd reported 0)", first[i].Height, first[i].Confirmations, want)
		}
	}
}

func TestANewTipCostsOneBlock(t *testing.T) {
	c := newFakeChain(t, 100)
	firstPage(t)
	c.spent()
	c.setTip(101, "block-101")
	page := firstPage(t)
	if count, hash, block := c.spent(); count != 1 || hash != 1 || block != 1 {
		t.Fatalf("a new tip cost %d count, %d hash, %d block calls; want 1, 1, 1", count, hash, block)
	}
	if page[0].Height != 101 || page[0].Confirmations != 1 {
		t.Fatalf("the page leads with %+v, want the new tip at 1 confirmation", page[0])
	}
}

// A block that fell off the main chain sits in the cache under its own hash;
// the walk from the tip must never reach it.
func TestAReorgedBlockIsNeverServed(t *testing.T) {
	c := newFakeChain(t, 100)
	before := firstPage(t)
	old := before[0].Hash
	c.spent()
	c.setTip(100, "fork-100")
	after := firstPage(t)
	if _, _, block := c.spent(); block != 1 {
		t.Fatalf("the reorg cost %d block calls, want 1", block)
	}
	if after[0].Hash != chainHash("fork-100") || after[0].Height != 100 {
		t.Fatalf("the page leads with %+v, want the replacement tip", after[0])
	}
	for _, b := range after {
		if b.Hash == old {
			t.Fatal("the reorged-out block is still being served")
		}
	}
}

func TestTheBlockCacheStaysBounded(t *testing.T) {
	newFakeChain(t, 1500)
	for page := 1; page <= 61; page++ {
		if _, err := FetchRecentBlocksPaginated(context.Background(), page, 20); err != nil {
			t.Fatal(err)
		}
	}
	if n := blockSummaryCache.Len(); n > blockCacheDepth {
		t.Fatalf("the cache holds %d summaries, more than its depth of %d", n, blockCacheDepth)
	}
}
