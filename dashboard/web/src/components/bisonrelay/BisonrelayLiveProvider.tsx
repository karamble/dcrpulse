// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode, createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import { BisonrelayLiveEvent } from '../../services/bisonrelayApi';

type Listener = (evt: BisonrelayLiveEvent) => void;

interface BisonrelayLiveCtx {
  unread: Record<string, number>;
  totalUnread: number;
  clearUnread: (uid: string) => void;
  setActiveUid: (uid: string) => void;
  gcUnread: Record<string, number>;
  totalGCUnread: number;
  clearGCUnread: (gcid: string) => void;
  setActiveGCID: (gcid: string) => void;
  addListener: (fn: Listener) => () => void;
}

const Ctx = createContext<BisonrelayLiveCtx | null>(null);

export const useBisonrelayLive = (): BisonrelayLiveCtx => {
  const c = useContext(Ctx);
  if (!c) throw new Error('useBisonrelayLive must be used inside BisonrelayLiveProvider');
  return c;
};

export const BisonrelayLiveProvider = ({ children }: { children: ReactNode }) => {
  const [unread, setUnread] = useState<Record<string, number>>({});
  const [gcUnread, setGCUnread] = useState<Record<string, number>>({});
  const listenersRef = useRef<Set<Listener>>(new Set());
  const activeUidRef = useRef<string>('');
  const activeGCIDRef = useRef<string>('');

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

  const setActiveGCID = useCallback((gcid: string) => {
    activeGCIDRef.current = gcid;
  }, []);

  useEffect(() => {
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
            if (uid && uid !== activeUidRef.current) {
              setUnread((prev) => ({ ...prev, [uid]: (prev[uid] ?? 0) + 1 }));
            }
          }
          if (evt.type === 'gc-message') {
            const gcid = String((evt.payload as Record<string, unknown>)?.gcid ?? '');
            if (gcid && gcid !== activeGCIDRef.current) {
              setGCUnread((prev) => ({ ...prev, [gcid]: (prev[gcid] ?? 0) + 1 }));
            }
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
  }, []);

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
        setActiveGCID,
        addListener,
      }}
    >
      {children}
    </Ctx.Provider>
  );
};
