package gamingcore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var gamingHistoryRecovery sync.Mutex

// Settable so these rules run without a live brclientd.
// Production sets none of them.
var (
	gamingGCHistoryFetch = func(ctx context.Context, gcid [32]byte, page, pageSize int) ([]GroupEntry, error) {
		return hostRelay().GroupHistory(ctx, gcid, page, pageSize)
	}
	gamingSelfNick = localGamingNick
	gamingSelfUID  = localGamingUID
)

// localGamingNick is the nick BR logs this client's own messages under.
func localGamingNick(ctx context.Context) (string, error) {
	_, nick, err := hostRelay().Identity(ctx)
	if err != nil {
		return "", err
	}
	if nick == "" {
		return "", fmt.Errorf("BR nick unavailable")
	}
	return nick, nil
}

// gamingFrameInHistory reports whether BR's own log of the group holds frame as
// a message from this client. BR logs a group message under the sender's nick
// before it queues the send, so an entry here means BR took it.
func gamingFrameInHistory(ctx context.Context, gcid [32]byte, frame string) (bool, error) {
	nick, err := gamingSelfNick(ctx)
	if err != nil {
		return false, err
	}
	for page := 0; page < 10000; page++ {
		entries, err := gamingGCHistoryFetch(ctx, gcid, page, 500)
		if err != nil {
			return false, err
		}
		for _, entry := range entries {
			if entry.From == nick && strings.TrimSpace(entry.Message) == strings.TrimSpace(frame) {
				return true, nil
			}
		}
		if len(entries) < 500 {
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
	// only from the ledger.
	for group := range gamingAcceptedGroups() {
		out[group] = struct{}{}
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

// RecoverHistory delivers the journaled frames of every group the bridge knows,
// oldest first. It never sends a peer message; exact duplicates disappear in
// persistGamingFrame.
func (b *GamingBus) RecoverHistory() {
	if !gamingHistoryRecovery.TryLock() {
		return
	}
	defer gamingHistoryRecovery.Unlock()
	for gcid := range b.knownGamingGCIDs() {
		records, err := gamingJournalHistory(gcid)
		if err != nil {
			gameLog.Warnf("recover gaming history for %s: %v", gcid, err)
			continue
		}
		for _, rec := range records {
			b.deliverGamingMessage(gcid, rec.From, rec.Message)
		}
	}
}

// pruneSettledGamingHistory drops the protocol history of group chats whose
// funds are all paid out, once per group per run. The journal goes first:
// history recovery reads it, so the inbox is only pruned once nothing can
// replay into it.
func (b *GamingBus) pruneSettledGamingHistory(ctx context.Context, groups []string) {
	gamingHistoryRecovery.Lock()
	defer gamingHistoryRecovery.Unlock()
	for _, group := range groups {
		if _, done := b.prunedGroups[group]; done {
			continue
		}
		if _, err := pruneGamingJournal(group); err != nil {
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

// gamingAcceptedGroups is the group chat of every table in the payout ledger,
// closed ones included, since those still settle and recover. Only an existing
// ledger is read; opening one would create it.
func gamingAcceptedGroups() map[string]struct{} {
	out := make(map[string]struct{})
	if _, err := os.Stat(filepath.Join(GamingStateDir, "financial-authority", "authority.json")); err != nil {
		return out
	}
	store, err := gamingFundsStore()
	if err != nil {
		return out
	}
	tables, err := store.Tables()
	if err != nil {
		return out
	}
	for _, t := range tables {
		if t.Group != "" {
			out[t.Group] = struct{}{}
		}
	}
	return out
}

// gamingTableGroup reports whether gcid is the group of one of game's tables.
func gamingTableGroup(game, gcid string) bool {
	_, ok := gamingGameGroups()[game+"\x00"+gcid]
	return ok
}

// gamingGameGroups is every accepted table's game and group chat, keyed
// game + NUL + gcid.
func gamingGameGroups() map[string]struct{} {
	out := make(map[string]struct{})
	if _, err := os.Stat(filepath.Join(GamingStateDir, "financial-authority", "authority.json")); err != nil {
		return out
	}
	store, err := gamingFundsStore()
	if err != nil {
		return out
	}
	tables, err := store.Tables()
	if err != nil {
		return out
	}
	for _, t := range tables {
		if t.Group != "" {
			out[t.Scope.Game+"\x00"+t.Group] = struct{}{}
		}
	}
	return out
}

// gamingTableSenders reports whether game has an accepted table sid in group
// gcid, and once its roster is complete, the seated players who may send.
func gamingTableSenders(game, sid, gcid string) (map[string]bool, bool) {
	if _, err := os.Stat(filepath.Join(GamingStateDir, "financial-authority", "authority.json")); err != nil {
		return nil, false
	}
	store, err := gamingFundsStore()
	if err != nil {
		return nil, false
	}
	seated, found, err := store.TableSenders(game, sid, gcid)
	if err != nil {
		return nil, false
	}
	return seated, found
}
