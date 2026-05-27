// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode, createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import type { DexNote } from '../../services/dcrdexApi';
import type { MarketSpot } from './useDexFeed';

type NoteListener = (note: DexNote) => void;

// SpotsNote is the bisonw `spots` notification: a map of marketID -> spot. The
// initial subPriceFeed sends the full map; later price_update notes carry one
// market each, so listeners merge rather than replace.
type SpotsNote = DexNote & { spots?: Record<string, MarketSpot> };

// spotKey indexes spots by base/quote asset id, matching the markets list's
// `${baseID}-${quoteID}` row keys (avoids symbol-casing mismatches).
const spotKey = (s: MarketSpot) => `${s.baseID}-${s.quoteID}`;

interface DexLiveCtx {
  addNoteListener: (fn: NoteListener) => () => void;
  spots: Record<string, MarketSpot>;
  seedSpots: (snapshot: Record<string, MarketSpot>) => void;
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
  // spots is the live last/24h price map for every market, fed by `spots` notes
  // (and seeded from the config snapshot). Shared so the markets list renders
  // all rows without subscribing to each market.
  const [spots, setSpots] = useState<Record<string, MarketSpot>>({});

  const seedSpots = useCallback((snapshot: Record<string, MarketSpot>) => {
    if (!Object.keys(snapshot).length) return;
    // Merge so live updates already received are not clobbered by the snapshot.
    setSpots((prev) => ({ ...snapshot, ...prev }));
  }, []);

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
        const note = msg.payload as SpotsNote;
        if (note.type === 'spots' && note.spots) {
          const incoming = Object.values(note.spots).filter((s): s is MarketSpot => !!s);
          if (incoming.length) {
            setSpots((prev) => {
              const next = { ...prev };
              incoming.forEach((s) => {
                next[spotKey(s)] = s;
              });
              return next;
            });
          }
          return;
        }
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

  return <Ctx.Provider value={{ addNoteListener, spots, seedSpots }}>{children}</Ctx.Provider>;
};

// useDexSpots returns the live last/24h price map for all markets, keyed by
// `${baseID}-${quoteID}`. Empty outside the provider (e.g. the preview view).
export function useDexSpots(): Record<string, MarketSpot> {
  return useContext(Ctx)?.spots ?? EMPTY_SPOTS;
}

// useSeedDexSpots returns the callback that seeds the spots map from the config
// snapshot. A no-op outside the provider.
export function useSeedDexSpots(): (snapshot: Record<string, MarketSpot>) => void {
  return useContext(Ctx)?.seedSpots ?? noop;
}

const EMPTY_SPOTS: Record<string, MarketSpot> = {};
const noop = () => {};

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
