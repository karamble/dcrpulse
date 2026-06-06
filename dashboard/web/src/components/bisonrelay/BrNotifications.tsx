// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { Bell } from 'lucide-react';
import {
  BisonrelayLiveEvent,
  BisonrelayNote,
  getBisonrelayNotifications,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';

const SEEN_KEY = 'brNotesSeen';

// Live event types that correspond to persisted notes and warrant an
// immediate refetch of the bell list.
const REFRESH_EVENT_TYPES = new Set([
  'file-invoice-capacity-low',
  'invoice-gen-failed',
  'offline-too-long',
  'server-unwelcome',
  'posts-subscribe-error',
  'blocked-by-user',
  'tip-received',
]);

const sevDot = (s: string) =>
  s === 'error' ? 'bg-destructive' : s === 'warn' ? 'bg-warning' : 'bg-muted-foreground/50';

// BrNotifications is the bell + dropdown in the BR tab bar, mirroring the
// DEX bell: it polls brclientd's persisted notes (which survive the browser
// being closed) and tracks unread by timestamp, no ack round-trip.
export const BrNotifications = () => {
  const [notes, setNotes] = useState<BisonrelayNote[]>([]);
  const [open, setOpen] = useState(false);
  const [seen, setSeen] = useState<number>(() => Number(localStorage.getItem(SEEN_KEY) || 0));
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const { addListener } = useBisonrelayLive();

  const refresh = () => getBisonrelayNotifications(50).then(setNotes).catch(() => {});

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
        <div className="absolute right-0 top-full mt-1 z-40 w-80 max-h-96 overflow-y-auto rounded-xl bg-card border border-border/50 shadow-xl">
          {notes.length === 0 ? (
            <p className="text-xs text-muted-foreground p-4 text-center">No notifications yet.</p>
          ) : (
            notes.map((n) => (
              <div key={n.id} className="px-3 py-2.5 border-b border-border/30 last:border-b-0">
                <div className="flex items-center gap-2">
                  <span className={`inline-block h-2 w-2 rounded-full shrink-0 ${sevDot(n.severity)}`} />
                  <span className="text-xs font-medium text-foreground truncate">{n.subject}</span>
                  <span className="ml-auto shrink-0 text-[10px] text-muted-foreground">
                    {new Date(n.ts).toLocaleString()}
                  </span>
                </div>
                <p className="text-[11px] text-muted-foreground mt-1 break-words">{n.detail}</p>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
};
