package services

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const gamingInboxFile = "gaming-wire-inbox-v2.jsonl"

func gamingFrameKey(ev GamingFrameEvent) string {
	h := sha256.New()
	for _, part := range []string{ev.Game, ev.GCID, ev.From, ev.Frame} {
		var n [8]byte
		v := uint64(len(part))
		for i := 7; i >= 0; i-- {
			n[i] = byte(v)
			v >>= 8
		}
		_, _ = h.Write(n[:])
		_, _ = io.WriteString(h, part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// loadGamingFramesLocked loads the append-only inbox and returns the requested
// replay. b.wireMu must be held by the caller.
func (b *GamingBus) loadGamingFramesLocked(game string, after uint64) ([]GamingFrameEvent, error) {
	dir := GamingStateDir
	if b.wireDir != dir {
		b.wireDir = dir
		b.wireNext = make(map[string]uint64)
		b.wireRecords = make(map[string][]GamingFrameEvent)
		b.wireSeen = make(map[string]struct{})
		path := filepath.Join(dir, gamingInboxFile)
		f, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		defer f.Close()
		scan := bufio.NewScanner(f)
		scan.Buffer(make([]byte, 64*1024), 2<<20)
		for line := 1; scan.Scan(); line++ {
			var ev GamingFrameEvent
			if err := json.Unmarshal(scan.Bytes(), &ev); err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			if ev.Seq == 0 || ev.Game == "" || ev.GCID == "" || ev.Frame == "" {
				return nil, fmt.Errorf("line %d: incomplete gaming frame", line)
			}
			if ev.Seq <= b.wireNext[ev.Game] {
				return nil, fmt.Errorf("line %d: non-increasing sequence for %q", line, ev.Game)
			}
			b.wireNext[ev.Game] = ev.Seq
			b.wireRecords[ev.Game] = append(b.wireRecords[ev.Game], ev)
			b.wireSeen[gamingFrameKey(ev)] = struct{}{}
		}
		if err := scan.Err(); err != nil {
			return nil, err
		}
	}
	records := b.wireRecords[game]
	out := make([]GamingFrameEvent, 0, len(records))
	for _, ev := range records {
		if ev.Seq > after && !ev.Financial {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (b *GamingBus) financialReplay() []GamingFrameEvent {
	b.wireMu.Lock()
	defer b.wireMu.Unlock()
	if _, err := b.loadGamingFramesLocked("", 0); err != nil {
		gameLog.Errorf("load durable financial inbox: %v", err)
		return nil
	}
	var out []GamingFrameEvent
	for _, records := range b.wireRecords {
		for _, ev := range records {
			if ev.Financial {
				out = append(out, ev)
			}
		}
	}
	return out
}

// persistGamingFrame writes and syncs a frame before it can reach a game.
// Exact BR history duplicates are ignored, so reconnecting a notification
// source cannot replay the same protocol event into the game.
func (b *GamingBus) persistGamingFrame(ev GamingFrameEvent) (uint64, bool, error) {
	b.wireMu.Lock()
	defer b.wireMu.Unlock()
	if _, err := b.loadGamingFramesLocked(ev.Game, 0); err != nil {
		return 0, false, err
	}
	key := gamingFrameKey(ev)
	if _, exists := b.wireSeen[key]; exists {
		return 0, false, nil
	}
	if err := os.MkdirAll(GamingStateDir, 0o700); err != nil {
		return 0, false, err
	}
	ev.Seq = b.wireNext[ev.Game] + 1
	raw, err := json.Marshal(ev)
	if err != nil {
		return 0, false, err
	}
	path := filepath.Join(GamingStateDir, gamingInboxFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, false, err
	}
	if _, err = f.Write(append(raw, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return 0, false, err
	}
	if closeErr != nil {
		return 0, false, closeErr
	}
	b.wireNext[ev.Game] = ev.Seq
	b.wireRecords[ev.Game] = append(b.wireRecords[ev.Game], ev)
	b.wireSeen[key] = struct{}{}
	return ev.Seq, true, nil
}
