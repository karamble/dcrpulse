// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode, createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import {
  ARCHIVED_GROUP_ID,
  BisonrelayContactGroups,
  BisonrelayLiveEvent,
  getBisonrelayContactGroups,
} from '../../services/bisonrelayApi';
import { useWalletReady } from '../../hooks/useWalletReady';

type Listener = (evt: BisonrelayLiveEvent) => void;

interface BisonrelayLiveCtx {
  unread: Record<string, number>;
  totalUnread: number;
  clearUnread: (uid: string) => void;
  setActiveUid: (uid: string) => void;
  gcUnread: Record<string, number>;
  totalGCUnread: number;
  clearGCUnread: (gcid: string) => void;
  // Drop unread for any GC not in liveGcids - reconciles entries orphaned when a
  // GC we were kicked from / that was dissolved disappears from the list.
  pruneGCUnread: (liveGcids: string[]) => void;
  setActiveGCID: (gcid: string) => void;
  addListener: (fn: Listener) => () => void;
  contactGroups: BisonrelayContactGroups | null;
  refreshContactGroups: () => void;
}

const Ctx = createContext<BisonrelayLiveCtx | null>(null);

export const useBisonrelayLive = (): BisonrelayLiveCtx => {
  const c = useContext(Ctx);
  if (!c) throw new Error('useBisonrelayLive must be used inside BisonrelayLiveProvider');
  return c;
};

export const BisonrelayLiveProvider = ({ children }: { children: ReactNode }) => {
  // A watch-only wallet has no Bison Relay daemon; the live socket is skipped for it.
  const { isWatchOnly } = useWalletReady();
  const [unread, setUnread] = useState<Record<string, number>>({});
  const [gcUnread, setGCUnread] = useState<Record<string, number>>({});
  const [contactGroups, setContactGroups] = useState<BisonrelayContactGroups | null>(null);
  const listenersRef = useRef<Set<Listener>>(new Set());
  const activeUidRef = useRef<string>('');
  const activeGCIDRef = useRef<string>('');
  // Archived contacts (by uid) whose DMs must not badge; a ref so the
  // long-lived websocket handler sees the current set.
  const archivedRef = useRef<Set<string>>(new Set());

  const refreshContactGroups = useCallback(() => {
    getBisonrelayContactGroups()
      .then((g) => {
        setContactGroups(g);
        const next = new Set<string>();
        Object.entries(g.contacts ?? {}).forEach(([uid, a]) => {
          if (a.group === ARCHIVED_GROUP_ID) next.add(uid);
        });
        archivedRef.current = next;
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    refreshContactGroups();
  }, [refreshContactGroups]);

  const addListener = useCallback((fn: Listener) => {
    listenersRef.current.add(fn);
    return () => {
      listenersRef.current.delete(fn);
    };
  }, []);

  const clearUnread = useCallback((uid: string) => {
    if (!uid) return;
    setUnread((prev) => {
      if (!prev[uid]) return prev;
      const next = { ...prev };
      delete next[uid];
      return next;
    });
  }, []);

  const setActiveUid = useCallback((uid: string) => {
    activeUidRef.current = uid;
  }, []);

  const clearGCUnread = useCallback((gcid: string) => {
    if (!gcid) return;
    setGCUnread((prev) => {
      if (!prev[gcid]) return prev;
      const next = { ...prev };
      delete next[gcid];
      return next;
    });
  }, []);

  const pruneGCUnread = useCallback((liveGcids: string[]) => {
    const live = new Set(liveGcids);
    setGCUnread((prev) => {
      const keys = Object.keys(prev);
      if (keys.every((k) => live.has(k))) return prev;
      const next: Record<string, number> = {};
      for (const k of keys) if (live.has(k)) next[k] = prev[k];
      return next;
    });
  }, []);

  const setActiveGCID = useCallback((gcid: string) => {
    activeGCIDRef.current = gcid;
  }, []);

  useEffect(() => {
    // A watch-only wallet has no Bison Relay daemon; skip the live socket (the
    // backend notifications loop also idles for watch-only).
    if (isWatchOnly) return;
    let ws: WebSocket | null = null;
    let cancelled = false;
    let retry = 1000;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      if (cancelled) return;
      const url = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/api/br/events`;
      ws = new WebSocket(url);
      ws.onopen = () => {
        retry = 1000;
      };
      ws.onmessage = (e) => {
        try {
          const evt: BisonrelayLiveEvent = JSON.parse(e.data);
          listenersRef.current.forEach((fn) => {
            try { fn(evt); } catch { /* ignore */ }
          });
          if (evt.type === 'pm') {
            const uid = String((evt.payload as Record<string, unknown>)?.from ?? '');
            if (uid && uid !== activeUidRef.current && !archivedRef.current.has(uid)) {
              setUnread((prev) => ({ ...prev, [uid]: (prev[uid] ?? 0) + 1 }));
            }
          }
          // Daemon system lines logged into a user's thread (blocked sales,
          // failed invoices, subscription errors, idle unsubscribes) badge
          // that conversation like a PM would.
          if (
            evt.type === 'file-invoice-capacity-low' ||
            evt.type === 'invoice-gen-failed' ||
            evt.type === 'posts-subscribe-error' ||
            evt.type === 'idle-unsubscribing'
          ) {
            const uid = String((evt.payload as Record<string, unknown>)?.uid ?? '');
            if (uid && uid !== activeUidRef.current && !archivedRef.current.has(uid)) {
              setUnread((prev) => ({ ...prev, [uid]: (prev[uid] ?? 0) + 1 }));
            }
          }
          if (evt.type === 'contact-groups-changed') {
            refreshContactGroups();
          }
          if (evt.type === 'gc-message') {
            const gcid = String((evt.payload as Record<string, unknown>)?.gcid ?? '');
            if (gcid && gcid !== activeGCIDRef.current) {
              setGCUnread((prev) => ({ ...prev, [gcid]: (prev[gcid] ?? 0) + 1 }));
            }
          }
          // A GC we were removed from / that was dissolved leaves no sidebar row
          // to open and clear, so release its unread (and the nav dot) here.
          if (evt.type === 'gc-killed') {
            clearGCUnread(String((evt.payload as Record<string, unknown>)?.gcid ?? ''));
          } else if (evt.type === 'gc-parted' || evt.type === 'gc-members-removed') {
            const p = (evt.payload ?? {}) as Record<string, unknown>;
            if (p.self === true) clearGCUnread(String(p.gcid ?? ''));
          }
        } catch {
          /* ignore */
        }
      };
      ws.onclose = () => {
        if (cancelled) return;
        retryTimer = setTimeout(connect, retry);
        retry = Math.min(retry * 2, 30000);
      };
      ws.onerror = () => {
        try { ws?.close(); } catch { /* ignore */ }
      };
    };
    connect();

    return () => {
      cancelled = true;
      if (retryTimer) clearTimeout(retryTimer);
      try { ws?.close(); } catch { /* ignore */ }
    };
  }, [isWatchOnly]);

  const totalUnread = Object.values(unread).reduce((a, b) => a + b, 0);
  const totalGCUnread = Object.values(gcUnread).reduce((a, b) => a + b, 0);

  return (
    <Ctx.Provider
      value={{
        unread,
        totalUnread,
        clearUnread,
        setActiveUid,
        gcUnread,
        totalGCUnread,
        clearGCUnread,
        pruneGCUnread,
        setActiveGCID,
        addListener,
        contactGroups,
        refreshContactGroups,
      }}
    >
      {children}
    </Ctx.Provider>
  );
};
