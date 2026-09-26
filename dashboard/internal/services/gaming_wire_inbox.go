package services

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// gamingWireMaxLine is the longest line either wire journal writes or reads.
// A 1 MiB frame whose every byte JSON escapes to six fits with room to spare.
const gamingWireMaxLine = 8 << 20

// healTornTail drops a final line with no newline: an append that never
// completed, so nothing acted on it. It returns how many bytes it dropped.
func healTornTail(path string) (int64, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()
	if size == 0 {
		return 0, nil
	}
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, size-1); err != nil {
		return 0, err
	}
	if last[0] == '\n' {
		return 0, nil
	}
	// Walk back to the previous newline; everything after it is the torn write.
	keep := int64(0)
	buf := make([]byte, 64*1024)
	for end := size; end > 0 && keep == 0; {
		start := end - int64(len(buf))
		if start < 0 {
			start = 0
		}
		chunk := buf[:end-start]
		if _, err := f.ReadAt(chunk, start); err != nil {
			return 0, err
		}
		if i := bytes.LastIndexByte(chunk, '\n'); i >= 0 {
			keep = start + int64(i) + 1
		}
		end = start
	}
	if err := f.Truncate(keep); err != nil {
		return 0, err
	}
	if err := f.Sync(); err != nil {
		return 0, err
	}
	return size - keep, nil
}

// loadGamingFramesLocked loads the append-only inbox and returns the requested
// replay. The file is installed only once it has read cleanly, so a bad file
// fails every call instead of leaving part of it in place. b.wireMu must be
// held by the caller.
func (b *GamingBus) loadGamingFramesLocked(game string, after uint64) ([]GamingFrameEvent, error) {
	dir := GamingStateDir
	if b.wireDir != dir {
		next := make(map[string]uint64)
		records := make(map[string][]GamingFrameEvent)
		seen := make(map[string]struct{})
		path := filepath.Join(dir, gamingInboxFile)
		if err := readGamingInbox(path, next, records, seen); err != nil {
			return nil, fmt.Errorf("gaming inbox %s: %w", path, err)
		}
		b.wireDir, b.wireNext, b.wireRecords, b.wireSeen = dir, next, records, seen
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

// readGamingInbox reads the inbox file into the given maps. A missing file is
// an empty inbox.
func readGamingInbox(path string, next map[string]uint64, records map[string][]GamingFrameEvent, seen map[string]struct{}) error {
	dropped, err := healTornTail(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if dropped > 0 {
		gameLog.Warnf("gaming inbox %s: dropped an unfinished last record (%d bytes)", path, dropped)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), gamingWireMaxLine)
	for line := 1; scan.Scan(); line++ {
		var rec gamingInboxLine
		if err := json.Unmarshal(scan.Bytes(), &rec); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		ev := rec.GamingFrameEvent
		if ev.Seq == 0 || ev.Game == "" || (!rec.Mark && (ev.GCID == "" || ev.Frame == "")) {
			return fmt.Errorf("line %d: incomplete gaming frame", line)
		}
		if ev.Seq <= next[ev.Game] {
			return fmt.Errorf("line %d: non-increasing sequence for %q", line, ev.Game)
		}
		next[ev.Game] = ev.Seq
		if rec.Mark {
			continue
		}
		records[ev.Game] = append(records[ev.Game], ev)
		seen[gamingFrameKey(ev)] = struct{}{}
	}
	return scan.Err()
}

// gamingInboxLine is one inbox line. A mark carries only a game's highest seq
// from before a prune.
type gamingInboxLine struct {
	GamingFrameEvent
	Mark bool `json:"mark,omitempty"`
}

// pruneGamingGroup removes one group chat's frames from the inbox. Each game
// keeps a mark with the highest seq it was issued, so no cursor sees a seq
// twice after a restart.
func (b *GamingBus) pruneGamingGroup(gcid string) (int, error) {
	b.wireMu.Lock()
	defer b.wireMu.Unlock()
	if _, err := b.loadGamingFramesLocked("", 0); err != nil {
		return 0, err
	}
	removed := 0
	for _, records := range b.wireRecords {
		for _, ev := range records {
			if ev.GCID == gcid {
				removed++
			}
		}
	}
	if removed == 0 {
		return 0, nil
	}
	games := make([]string, 0, len(b.wireNext))
	for game := range b.wireNext {
		games = append(games, game)
	}
	sort.Strings(games)
	var buf bytes.Buffer
	for _, game := range games {
		var last uint64
		for _, ev := range b.wireRecords[game] {
			if ev.GCID == gcid {
				continue
			}
			raw, err := json.Marshal(ev)
			if err != nil {
				return 0, err
			}
			buf.Write(append(raw, '\n'))
			last = ev.Seq
		}
		if last < b.wireNext[game] {
			raw, err := json.Marshal(gamingInboxLine{GamingFrameEvent{Seq: b.wireNext[game], Game: game}, true})
			if err != nil {
				return 0, err
			}
			buf.Write(append(raw, '\n'))
		}
	}
	if err := writeFileSynced(filepath.Join(GamingStateDir, gamingInboxFile), buf.Bytes(), 0o600); err != nil {
		return 0, err
	}
	// Reload from the new file on next use.
	b.wireDir = ""
	return removed, nil
}

// financialReplay returns the stored financial frames still to apply: those of
// accepted tables' groups not yet applied by this process.
func (b *GamingBus) financialReplay() []GamingFrameEvent {
	groups := gamingAcceptedGroups()
	b.wireMu.Lock()
	defer b.wireMu.Unlock()
	if _, err := b.loadGamingFramesLocked("", 0); err != nil {
		gameLog.Errorf("load durable financial inbox: %v", err)
		return nil
	}
	var out []GamingFrameEvent
	for _, records := range b.wireRecords {
		for _, ev := range records {
			if _, ok := groups[ev.GCID]; !ok || !ev.Financial {
				continue
			}
			if _, done := b.financialDone[financialDoneKey(ev)]; done {
				continue
			}
			out = append(out, ev)
		}
	}
	return out
}

func financialDoneKey(ev GamingFrameEvent) string {
	return fmt.Sprintf("%s\x00%d", ev.Game, ev.Seq)
}

// markFinancialApplied keeps an applied financial frame out of later replays.
func (b *GamingBus) markFinancialApplied(ev GamingFrameEvent) {
	b.wireMu.Lock()
	defer b.wireMu.Unlock()
	if b.financialDone == nil {
		b.financialDone = make(map[string]struct{})
	}
	b.financialDone[financialDoneKey(ev)] = struct{}{}
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
	if len(raw)+1 > gamingWireMaxLine {
		return 0, false, fmt.Errorf("gaming frame of %d bytes is longer than the inbox keeps", len(raw))
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
