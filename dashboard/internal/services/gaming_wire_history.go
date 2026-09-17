package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
)

var gamingHistoryRecovery sync.Mutex

type gamingHistoryPage struct {
	Entries []struct {
		Message string `json:"message"`
		From    string `json:"from"`
	} `json:"entries"`
}

func gamingFrameInHistory(ctx context.Context, gcid rpc.ShortIDHex, frame string) (bool, error) {
	for page := 0; page < 10000; page++ {
		raw, err := rpc.BrclientdGCHistory(ctx, gcid, page, 500)
		if err != nil {
			return false, err
		}
		var got gamingHistoryPage
		if err := json.Unmarshal(raw, &got); err != nil {
			return false, err
		}
		for _, entry := range got.Entries {
			if strings.TrimSpace(entry.Message) == strings.TrimSpace(frame) {
				return true, nil
			}
		}
		if len(got.Entries) < 500 {
			return false, nil
		}
	}
	return false, nil
}

// claimOrReconcileGamingFrame claims the one permitted physical send. A claim
// left uncertain by a crash is resolved against BR's own local history and is
// never retried blindly.
func claimOrReconcileGamingFrame(ctx context.Context, game, gcid string, parsed gamingFrame, frame string) (bool, error) {
	fresh, err := claimGamingFrameSend(game, gcid, parsed, frame)
	if !errors.Is(err, errGamingSendUncertain) {
		return fresh, err
	}
	id, parseErr := parseGamingGCID(gcid)
	if parseErr != nil {
		return false, parseErr
	}
	found, historyErr := gamingFrameInHistory(ctx, id, frame)
	if historyErr != nil || !found {
		return false, errGamingSendUncertain
	}
	if markErr := markGamingFrameSent(game, gcid, parsed, frame); markErr != nil {
		return false, markErr
	}
	return false, nil
}

// knownGamingGCIDs returns chats the bridge has either sent a gaming frame to
// or persisted one from. A local participant emits its own seat event, so an
// active table enters this set before later formation traffic can matter.
func (b *GamingBus) knownGamingGCIDs() map[string]struct{} {
	out := make(map[string]struct{})
	b.wireMu.Lock()
	if _, err := b.loadGamingFramesLocked("", 0); err == nil {
		for _, records := range b.wireRecords {
			for _, ev := range records {
				out[ev.GCID] = struct{}{}
			}
		}
	}
	b.wireMu.Unlock()

	gamingOutbox.Lock()
	if loadGamingSendClaimsLocked() == nil {
		for key := range gamingOutbox.claims {
			parts := strings.Split(key, "\x00")
			if len(parts) == 4 {
				out[parts[1]] = struct{}{}
			}
		}
	}
	gamingOutbox.Unlock()
	return out
}

// RecoverHistory rebuilds the durable inbox from BR's local group-chat
// history after a notification sequence gap. It never sends a peer message.
// Exact duplicates disappear in persistGamingFrame.
func (b *GamingBus) RecoverHistory() {
	if !gamingHistoryRecovery.TryLock() {
		return
	}
	defer gamingHistoryRecovery.Unlock()
	for gcid := range b.knownGamingGCIDs() {
		id, err := parseGamingGCID(gcid)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		var pages []gamingHistoryPage
		for page := 0; page < 10000; page++ {
			raw, err := rpc.BrclientdGCHistory(ctx, id, page, 500)
			if err != nil {
				gameLog.Warnf("recover gaming history for %s: %v", gcid, err)
				break
			}
			var got gamingHistoryPage
			if err := json.Unmarshal(raw, &got); err != nil {
				gameLog.Warnf("decode gaming history for %s: %v", gcid, err)
				break
			}
			if len(got.Entries) == 0 {
				break
			}
			pages = append(pages, got)
			if len(got.Entries) < 500 {
				break
			}
		}
		cancel()
		// Page zero is newest. Apply oldest pages first and preserve the
		// oldest-first order within each page.
		for page := len(pages) - 1; page >= 0; page-- {
			for _, entry := range pages[page].Entries {
				b.deliverGamingMessage(gcid, entry.From, entry.Message)
			}
		}
	}
}
