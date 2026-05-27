// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode, createContext, useCallback, useContext, useEffect, useRef } from 'react';
import type { DexNote } from '../../services/dcrdexApi';

type NoteListener = (note: DexNote) => void;

interface DexLiveCtx {
  addNoteListener: (fn: NoteListener) => () => void;
}

const Ctx = createContext<DexLiveCtx | null>(null);

// DexLiveProvider owns a single WebSocket to the dashboard's DCRDEX notification
// relay (/api/dcrdex/notify -> bisonw's webserver /ws). bisonw broadcasts its
// whole notification feed to every connected client, so a bare connection
// receives order, match, balance, bond and other notes without subscribing to a
// market. Panels register listeners (typically via useDexRefreshOnNotes) and
// refresh from live notes instead of polling, sharing this one connection. The
// order book uses a separate connection on the RPC server (see useDexFeed).
export const DexLiveProvider = ({ children }: { children: ReactNode }) => {
  const listenersRef = useRef<Set<NoteListener>>(new Set());

  const addNoteListener = useCallback((fn: NoteListener) => {
    listenersRef.current.add(fn);
    return () => {
      listenersRef.current.delete(fn);
    };
  }, []);

  useEffect(() => {
    let ws: WebSocket | null = null;
    let cancelled = false;
    let retry = 1000;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      if (cancelled) return;
      const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
      ws = new WebSocket(`${proto}://${window.location.host}/api/dcrdex/notify`);
      ws.onopen = () => {
        retry = 1000;
      };
      ws.onmessage = (e) => {
        let msg: any;
        try {
          msg = JSON.parse(e.data);
        } catch {
          return;
        }
        if (msg?.route !== 'notify' || !msg.payload) return;
        const note = msg.payload as DexNote;
        listenersRef.current.forEach((fn) => {
          try {
            fn(note);
          } catch {
            /* ignore */
          }
        });
      };
      ws.onclose = () => {
        if (cancelled) return;
        retryTimer = setTimeout(connect, retry);
        retry = Math.min(retry * 2, 30000);
      };
      ws.onerror = () => {
        try {
          ws?.close();
        } catch {
          /* ignore */
        }
      };
    };
    connect();

    return () => {
      cancelled = true;
      if (retryTimer) clearTimeout(retryTimer);
      try {
        ws?.close();
      } catch {
        /* ignore */
      }
    };
  }, []);

  return <Ctx.Provider value={{ addNoteListener }}>{children}</Ctx.Provider>;
};

// useDexRefreshOnNotes calls refresh (debounced) whenever a notification whose
// type is in `types` arrives on the shared notify socket. Bursts are coalesced
// so a flurry of notes triggers a single refresh. It is a no-op outside the
// provider (e.g. the preview view), where panels keep their initial fetch.
export function useDexRefreshOnNotes(types: string[], refresh: () => void) {
  const ctx = useContext(Ctx);
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  const typesKey = types.join(',');

  useEffect(() => {
    if (!ctx) return;
    const want = new Set(typesKey ? typesKey.split(',') : []);
    let timer: ReturnType<typeof setTimeout> | null = null;
    const off = ctx.addNoteListener((note) => {
      if (!want.has(note.type)) return;
      if (timer) return;
      timer = setTimeout(() => {
        timer = null;
        refreshRef.current();
      }, 400);
    });
    return () => {
      off();
      if (timer) clearTimeout(timer);
    };
  }, [ctx, typesKey]);
}
