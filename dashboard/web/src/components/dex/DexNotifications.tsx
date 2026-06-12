// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { toYMDTime } from '../../utils/date';
import { Bell } from 'lucide-react';
import { getDexNotifications, type DexNote } from '../../services/dcrdexApi';
import { loadNotifPrefs, shouldNotify } from './dexNotifPrefs';
import { useDexOnNotes, useDexRefreshOnNotes } from './DexLiveProvider';

const SEEN_KEY = 'dexNotesSeen';
const FIRED_KEY = 'dexNotesFired';

// Wallet connection failures (e.g. on a stack restart, before dcrwallet has
// loaded) are persisted error notes, but bisonw emits no persisted note when the
// wallet later reconnects (the walletstate "connected" note is data-severity and
// is not saved). So these warnings linger in the bell. We watch the live
// walletstate feed and, once the dcr wallet is running again, hide the stale
// warnings and surface a recovery note.
const WALLET_WARNING_TOPICS = new Set(['WalletConnectionWarning', 'BondWalletNotConnected']);
const RECONNECT_NOTE_ID = 'dcr-wallet-reconnected';

// User-facing note types that warrant refetching the persisted list. High-rate
// transient notes (spots, fiatrateupdate, epoch, walletsync) are excluded.
const REFRESH_NOTE_TYPES = [
  'order',
  'match',
  'bondpost',
  'bondrefund',
  'conn',
  'security',
  'dex_auth',
  'feepayment',
  'send',
  'reputation',
  'actionrequired',
];

const sevDot = (s: number) =>
  s >= 4 ? 'bg-destructive' : s === 3 ? 'bg-warning' : s === 2 ? 'bg-success' : 'bg-muted-foreground/50';

// DexNotifications is the bell + dropdown in the DEX sub-nav. It polls bisonw's
// recent notifications and tracks unread by timestamp (no ack round-trip).
export const DexNotifications = () => {
  const [notes, setNotes] = useState<DexNote[]>([]);
  const [open, setOpen] = useState(false);
  const [seen, setSeen] = useState<number>(() => Number(localStorage.getItem(SEEN_KEY) || 0));
  // Stamp at/below which wallet-connection warnings are treated as resolved, and
  // a synthesized recovery note to show in their place.
  const [resolvedBefore, setResolvedBefore] = useState(0);
  const [reconnectNote, setReconnectNote] = useState<DexNote | null>(null);

  const refresh = () => getDexNotifications(50).then(setNotes).catch(() => {});
  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useDexRefreshOnNotes(REFRESH_NOTE_TYPES, refresh);

  // When the dcr bond wallet reconnects, clear the lingering connection warnings
  // and show a one-line recovery note (bisonw never persists this transition).
  const notesRef = useRef<DexNote[]>(notes);
  notesRef.current = notes;
  useDexOnNotes(['walletstate'], (note) => {
    if (note.wallet?.symbol !== 'dcr' || !note.wallet?.running) return;
    const pending = notesRef.current.filter(
      (n) => WALLET_WARNING_TOPICS.has(n.topic) && n.stamp > resolvedBefore,
    );
    if (pending.length === 0) return;
    const latest = pending.reduce((m, n) => Math.max(m, n.stamp), 0);
    const stamp = note.stamp && note.stamp > latest ? note.stamp : latest + 1;
    setResolvedBefore(latest);
    setReconnectNote({
      type: 'walletstate',
      topic: 'WalletReconnected',
      subject: 'DCR wallet connected',
      details: 'The Decred bond wallet reconnected. You can post bonds and trade again.',
      severity: 2,
      stamp,
      acked: true,
      id: RECONNECT_NOTE_ID,
    });
  });

  // Fire desktop notifications for newly-arrived notes that match the user's
  // enabled categories. The first batch only seeds the cursor (no burst of OS
  // notifications for pre-existing notes).
  useEffect(() => {
    if (notes.length === 0) return;
    const maxStamp = notes.reduce((m, n) => Math.max(m, n.stamp), 0);
    const stored = localStorage.getItem(FIRED_KEY);
    if (stored === null) {
      localStorage.setItem(FIRED_KEY, String(maxStamp));
      return;
    }
    const lastFired = Number(stored);
    const prefs = loadNotifPrefs();
    if (prefs.desktop && typeof Notification !== 'undefined' && Notification.permission === 'granted') {
      notes
        .filter((n) => n.stamp > lastFired)
        .sort((a, b) => a.stamp - b.stamp)
        .forEach((n) => {
          if (shouldNotify(prefs, n.type)) {
            try {
              new Notification(n.subject || 'DCRDEX', { body: n.details || '' });
            } catch {
              /* ignore */
            }
          }
        });
    }
    if (maxStamp > lastFired) localStorage.setItem(FIRED_KEY, String(maxStamp));
  }, [notes]);

  const sorted = useMemo(() => {
    const merged = reconnectNote ? [reconnectNote, ...notes] : notes;
    return merged
      .filter((n) => !(WALLET_WARNING_TOPICS.has(n.topic) && n.stamp <= resolvedBefore))
      .sort((a, b) => b.stamp - a.stamp);
  }, [notes, reconnectNote, resolvedBefore]);
  const unread = useMemo(() => sorted.filter((n) => n.stamp > seen).length, [sorted, seen]);

  const toggle = () => {
    if (!open && sorted.length) {
      const max = sorted[0].stamp;
      setSeen(max);
      localStorage.setItem(SEEN_KEY, String(max));
    }
    setOpen((o) => !o);
  };

  return (
    <div className="relative">
      <button
        type="button"
        onClick={toggle}
        title="Notifications"
        className="relative p-2 rounded-lg text-muted-foreground hover:text-foreground hover:bg-background/50 transition-colors"
      >
        <Bell className="h-4 w-4" />
        {unread > 0 && (
          <span className="absolute -top-0.5 -right-0.5 min-w-4 h-4 px-1 rounded-full bg-primary text-[10px] font-semibold text-white flex items-center justify-center">
            {unread > 9 ? '9+' : unread}
          </span>
        )}
      </button>

      {open && (
        <>
          <button type="button" className="fixed inset-0 z-10 cursor-default" onClick={() => setOpen(false)} aria-hidden />
          <div className="absolute right-0 mt-1 w-80 max-h-96 overflow-auto z-20 rounded-xl border border-border/60 bg-card shadow-lg">
            <div className="px-3 py-2 text-[10px] uppercase tracking-wider text-muted-foreground/60 border-b border-border/40 sticky top-0 bg-card">
              Notifications
            </div>
            {sorted.length === 0 ? (
              <div className="px-3 py-6 text-xs text-muted-foreground">No notifications.</div>
            ) : (
              sorted.map((n) => (
                <div key={n.id || `${n.stamp}-${n.subject}`} className="px-3 py-2 border-b border-border/30 last:border-0">
                  <div className="flex items-center gap-1.5">
                    <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${sevDot(n.severity)}`} />
                    <span className="text-xs font-medium truncate">{n.subject}</span>
                  </div>
                  {n.details && <p className="text-[11px] text-muted-foreground mt-0.5 break-words">{n.details}</p>}
                  <p className="text-[10px] text-muted-foreground/60 mt-0.5">
                    {n.stamp ? toYMDTime(new Date(n.stamp)) : ''}
                  </p>
                </div>
              ))
            )}
          </div>
        </>
      )}
    </div>
  );
};
