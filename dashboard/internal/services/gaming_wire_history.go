package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
)

var gamingHistoryRecovery sync.Mutex

// Settable so the recovery and prune rules run without a live brclientd.
// Production sets none of them.
var (
	gamingHistoryFetch = rpc.BrclientdGamingHistory
	gamingHistoryPrune = rpc.BrclientdGamingHistoryPrune
	gamingSelfUID      = localGamingUID
)

type gamingHistoryPage struct {
	Entries []struct {
		Message string `json:"message"`
		From    string `json:"from"`
		Sent    bool   `json:"sent"`
	} `json:"entries"`
}

// isGamingUID reports whether from is a 64-hex BR UID in the form brclientd
// writes, rather than a nick from an older history page.
func isGamingUID(from string) bool {
	if len(from) != 64 {
		return false
	}
	for _, c := range from {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func gamingFrameInHistory(ctx context.Context, gcid rpc.ShortIDHex, frame string) (bool, error) {
	for page := 0; page < 10000; page++ {
		raw, err := gamingHistoryFetch(ctx, gcid, page, 500)
		if err != nil {
			return false, err
		}
		var got gamingHistoryPage
		if err := json.Unmarshal(raw, &got); err != nil {
			return false, err
		}
		for _, entry := range got.Entries {
			if entry.Sent && strings.TrimSpace(entry.Message) == strings.TrimSpace(frame) {
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

	// A table whose frames all arrived while the dashboard was down is known
	// only from the ledger. Read an existing ledger only; opening creates one.
	if _, err := os.Stat(filepath.Join(GamingStateDir, "financial-authority", "authority.json")); err == nil {
		if store, err := gamingFundsStore(); err == nil {
			if tables, err := store.Tables(); err == nil {
				for _, t := range tables {
					if t.Group != "" {
						out[t.Group] = struct{}{}
					}
				}
			}
		}
	}

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

// RecoverHistory rebuilds the durable inbox from brclientd's gaming journal
// after a notification sequence gap. It never sends a peer message. Exact
// duplicates disappear in persistGamingFrame; our own sends and entries without
// an authenticated UID are skipped.
func (b *GamingBus) RecoverHistory() {
	if !gamingHistoryRecovery.TryLock() {
		return
	}
	defer gamingHistoryRecovery.Unlock()
	uidCtx, uidCancel := context.WithTimeout(context.Background(), 10*time.Second)
	self, err := gamingSelfUID(uidCtx)
	uidCancel()
	if err != nil {
		gameLog.Warnf("recover gaming history: %v", err)
		return
	}
	for gcid := range b.knownGamingGCIDs() {
		id, err := parseGamingGCID(gcid)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		var pages []gamingHistoryPage
		for page := 0; page < 10000; page++ {
			raw, err := gamingHistoryFetch(ctx, id, page, 500)
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
				if entry.Sent || entry.From == self || !isGamingUID(entry.From) {
					continue
				}
				b.deliverGamingMessage(gcid, entry.From, entry.Message)
			}
		}
	}
}

// pruneSettledGamingHistory drops the protocol history of group chats whose
// funds are all paid out, once per group per run. brclientd's journal goes
// first: history recovery reads it, so the inbox is only pruned once nothing
// can replay into it.
func (b *GamingBus) pruneSettledGamingHistory(ctx context.Context, groups []string) {
	gamingHistoryRecovery.Lock()
	defer gamingHistoryRecovery.Unlock()
	for _, group := range groups {
		if _, done := b.prunedGroups[group]; done {
			continue
		}
		id, err := parseGamingGCID(group)
		if err != nil {
			continue
		}
		if err := gamingHistoryPrune(ctx, id); err != nil {
			gameLog.Warnf("prune gaming journal of %s: %v", group, err)
			continue
		}
		removed, err := b.pruneGamingGroup(group)
		if err != nil {
			gameLog.Warnf("prune gaming inbox of %s: %v", group, err)
			continue
		}
		if b.prunedGroups == nil {
			b.prunedGroups = make(map[string]struct{})
		}
		b.prunedGroups[group] = struct{}{}
		if removed > 0 {
			gameLog.Infof("pruned %d gaming frames of settled group %s", removed, group)
		}
	}
}
