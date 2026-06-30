// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { toYMDTime } from '../../utils/date';
import { Bell, Trash2 } from 'lucide-react';
import {
  BisonrelayLiveEvent,
  BisonrelayNote,
  getBisonrelayNotifications,
  deleteBisonrelayNotification,
  clearBisonrelayNotifications,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';

const SEEN_KEY = 'brNotesSeen';

// Live event types that correspond to persisted notes and warrant an
// immediate refetch of the bell list.
const REFRESH_EVENT_TYPES = new Set([
  'file-download-completed',
  'file-invoice-capacity-low',
  'invoice-gen-failed',
  'offline-too-long',
  'server-unwelcome',
  'posts-subscribe-error',
  'blocked-by-user',
  'tip-received',
  'post-heart-received',
  'post-status-received',
  'store-order-placed',
  'store-order-status',
  // Group-chat removal / dissolve persist a bell note in brclientd; refetch
  // the recent list as soon as the live event arrives.
  'gc-parted',
  'gc-killed',
  'gc-members-removed',
]);

const sevDot = (s: string) =>
  s === 'error' ? 'bg-destructive' : s === 'warn' ? 'bg-warning' : 'bg-muted-foreground/50';

// BrNotifications is the bell + dropdown in the BR tab bar, mirroring the
// DEX bell: it polls brclientd's persisted notes (which survive the browser
// being closed) and tracks unread by timestamp, no ack round-trip.
export const BrNotifications = () => {
  const [notes, setNotes] = useState<BisonrelayNote[]>([]);
  const [open, setOpen] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);
  const [seen, setSeen] = useState<number>(() => Number(localStorage.getItem(SEEN_KEY) || 0));
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const { addListener } = useBisonrelayLive();
  const navigate = useNavigate();

  const refresh = () => getBisonrelayNotifications(50).then(setNotes).catch(() => {});

  // Optimistically drop the row, then tell the daemon; resync on failure.
  const removeOne = async (id: number) => {
    setNotes((prev) => prev.filter((x) => x.id !== id));
    try {
      await deleteBisonrelayNotification(id);
    } catch {
      refresh();
    }
  };

  const clearAll = async () => {
    setConfirmClear(false);
    setNotes([]);
    try {
      await clearBisonrelayNotifications();
    } catch {
      refresh();
    }
  };

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    return addListener((evt: BisonrelayLiveEvent) => {
      if (REFRESH_EVENT_TYPES.has(evt.type)) refresh();
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [addListener]);

  useEffect(() => {
    if (!open) return undefined;
    const onClick = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, [open]);

  useEffect(() => {
    if (!open) setConfirmClear(false);
  }, [open]);

  const unread = useMemo(
    () => notes.filter((n) => new Date(n.ts).getTime() > seen).length,
    [notes, seen],
  );

  const toggle = () => {
    const next = !open;
    setOpen(next);
    if (next && notes.length > 0) {
      const newest = Math.max(...notes.map((n) => new Date(n.ts).getTime()));
      setSeen(newest);
      localStorage.setItem(SEEN_KEY, String(newest));
    }
  };

  return (
    <div ref={wrapRef} className="relative ml-auto shrink-0 self-center">
      <button
        type="button"
        onClick={toggle}
        aria-label="Notifications"
        className="relative p-2 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
      >
        <Bell className="h-4 w-4" />
        {unread > 0 && (
          <span className="absolute -top-0.5 -right-0.5 min-w-[16px] h-4 px-1 rounded-full bg-primary text-white text-[10px] font-semibold flex items-center justify-center">
            {unread > 99 ? '99+' : unread}
          </span>
        )}
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 z-40 w-80 max-h-96 flex flex-col overflow-hidden rounded-xl bg-card border border-border/50 shadow-xl">
          <div className="overflow-y-auto">
            {notes.length === 0 ? (
              <p className="text-xs text-muted-foreground p-4 text-center">No notifications yet.</p>
            ) : (
              notes.map((n) => {
                const link = n.link;
                return (
                  <div
                    key={n.id}
                    onClick={link ? () => { setOpen(false); navigate(link); } : undefined}
                    className={`px-3 py-2.5 border-b border-border/30 last:border-b-0${
                      link ? ' cursor-pointer hover:bg-muted/30' : ''
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      <span className={`inline-block h-2 w-2 rounded-full shrink-0 ${sevDot(n.severity)}`} />
                      <span className="text-xs font-medium text-foreground truncate">{n.subject}</span>
                      <span className="ml-auto shrink-0 text-[10px] text-muted-foreground">
                        {toYMDTime(new Date(n.ts))}
                      </span>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          e.preventDefault();
                          removeOne(n.id);
                        }}
                        aria-label="Delete notification"
                        title="Delete"
                        className="shrink-0 p-0.5 rounded text-muted-foreground/50 hover:text-destructive hover:bg-destructive/10 transition-colors"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    <p className="text-[11px] text-muted-foreground mt-1 break-words">{n.detail}</p>
                  </div>
                );
              })
            )}
          </div>
          {notes.length > 0 && (
            <div className="shrink-0 border-t border-border/40 p-2">
              {confirmClear ? (
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={clearAll}
                    className="flex-1 inline-flex items-center justify-center gap-1.5 px-2 py-1.5 rounded-md bg-destructive/15 text-destructive border border-destructive/30 text-xs hover:bg-destructive/25 transition-colors"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    Confirm clear all
                  </button>
                  <button
                    type="button"
                    onClick={() => setConfirmClear(false)}
                    className="px-2 py-1.5 rounded-md border border-border/60 text-xs text-muted-foreground hover:bg-muted/20 transition-colors"
                  >
                    Cancel
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmClear(true)}
                  className="w-full inline-flex items-center justify-center gap-1.5 px-2 py-1.5 rounded-md text-xs text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  Clear all
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
