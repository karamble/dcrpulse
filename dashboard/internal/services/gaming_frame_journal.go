// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dcrpulse/internal/fsutil"
)

// gamingJournalFile keeps every gaming frame the bridge received, with the
// sender's authenticated UID, so a table accepted after its first frames
// arrived can still read them. Records leave it only through prune, once the
// group's tables are settled.
const gamingJournalFile = "gaming-frames.jsonl"

// gamingJournalRecord is one received frame. A mark record carries only the
// highest seq issued before a prune.
type gamingJournalRecord struct {
	Seq     uint64 `json:"seq"`
	GCID    string `json:"gcid,omitempty"`
	From    string `json:"from,omitempty"`
	Message string `json:"message,omitempty"`
	TS      int64  `json:"ts,omitempty"`
	Mark    bool   `json:"mark,omitempty"`
}

// gamingFrameJournal is loaded for one directory at a time. A file that does
// not read cleanly keeps it unloaded, so every call fails until it is repaired.
var gamingFrameJournal struct {
	sync.Mutex
	dir  string
	next uint64
	seen map[string]struct{}
}

func gamingJournalKey(gcid, from, message string) string {
	return gamingFrameKey(GamingFrameEvent{GCID: gcid, From: from, Frame: message})
}

func loadGamingJournalLocked() error {
	j := &gamingFrameJournal
	if j.seen != nil && j.dir == GamingStateDir {
		return nil
	}
	path := filepath.Join(GamingStateDir, gamingJournalFile)
	seen := make(map[string]struct{})
	var last uint64
	dropped, err := healTornTail(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if dropped > 0 {
			gameLog.Warnf("gaming journal %s: dropped an unfinished last record (%d bytes)", path, dropped)
		}
		err = scanGamingJournal(path, func(rec gamingJournalRecord) {
			last = rec.Seq
			if !rec.Mark {
				seen[gamingJournalKey(rec.GCID, rec.From, rec.Message)] = struct{}{}
			}
		})
		if err != nil {
			return fmt.Errorf("gaming journal %s: %w", path, err)
		}
	}
	j.dir, j.next, j.seen = GamingStateDir, last, seen
	return nil
}

// scanGamingJournal reads every record in order, rejecting any damage.
func scanGamingJournal(path string, fn func(gamingJournalRecord)) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), gamingWireMaxLine)
	var prev uint64
	for line := 1; scan.Scan(); line++ {
		var rec gamingJournalRecord
		if err := json.Unmarshal(scan.Bytes(), &rec); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if rec.Seq <= prev {
			return fmt.Errorf("line %d: non-increasing sequence", line)
		}
		if !rec.Mark && (rec.GCID == "" || rec.From == "" || rec.Message == "") {
			return fmt.Errorf("line %d: incomplete gaming frame", line)
		}
		prev = rec.Seq
		fn(rec)
	}
	return scan.Err()
}

// appendGamingJournal writes and syncs one received frame. A frame already in
// the journal is not written again and reports false.
func appendGamingJournal(gcid, from, message string, ts time.Time) (bool, error) {
	j := &gamingFrameJournal
	j.Lock()
	defer j.Unlock()
	if err := loadGamingJournalLocked(); err != nil {
		return false, err
	}
	key := gamingJournalKey(gcid, from, message)
	if _, dup := j.seen[key]; dup {
		return false, nil
	}
	rec := gamingJournalRecord{Seq: j.next + 1, GCID: gcid, From: from, Message: message, TS: ts.Unix()}
	raw, err := json.Marshal(rec)
	if err != nil {
		return false, err
	}
	if len(raw)+1 > gamingWireMaxLine {
		return false, fmt.Errorf("gaming frame of %d bytes is longer than the journal keeps", len(raw))
	}
	if err := os.MkdirAll(GamingStateDir, 0o700); err != nil {
		return false, err
	}
	f, err := os.OpenFile(filepath.Join(GamingStateDir, gamingJournalFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	if _, err = f.Write(append(raw, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	j.next = rec.Seq
	j.seen[key] = struct{}{}
	return true, nil
}

// gamingJournalHistory returns one group's frames, oldest first.
func gamingJournalHistory(gcid string) ([]gamingJournalRecord, error) {
	j := &gamingFrameJournal
	j.Lock()
	defer j.Unlock()
	if err := loadGamingJournalLocked(); err != nil {
		return nil, err
	}
	var out []gamingJournalRecord
	err := scanGamingJournal(filepath.Join(GamingStateDir, gamingJournalFile), func(rec gamingJournalRecord) {
		if !rec.Mark && rec.GCID == gcid {
			out = append(out, rec)
		}
	})
	return out, err
}

// pruneGamingJournal removes one group's frames. The rewrite keeps a mark
// with the highest seq issued, so seq never repeats after a restart.
func pruneGamingJournal(gcid string) (int, error) {
	j := &gamingFrameJournal
	j.Lock()
	defer j.Unlock()
	if err := loadGamingJournalLocked(); err != nil {
		return 0, err
	}
	var buf bytes.Buffer
	var last uint64
	removed := 0
	kept := make(map[string]struct{})
	err := scanGamingJournal(filepath.Join(GamingStateDir, gamingJournalFile), func(rec gamingJournalRecord) {
		if rec.Mark {
			return
		}
		if rec.GCID == gcid {
			removed++
			return
		}
		raw, _ := json.Marshal(rec)
		buf.Write(raw)
		buf.WriteByte('\n')
		last = rec.Seq
		kept[gamingJournalKey(rec.GCID, rec.From, rec.Message)] = struct{}{}
	})
	if err != nil {
		return 0, err
	}
	if removed == 0 {
		return 0, nil
	}
	if last < j.next {
		raw, _ := json.Marshal(gamingJournalRecord{Seq: j.next, Mark: true})
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	if err := fsutil.AtomicWriteJSON(filepath.Join(GamingStateDir, gamingJournalFile), buf.Bytes()); err != nil {
		return 0, err
	}
	j.seen = kept
	return removed, nil
}
