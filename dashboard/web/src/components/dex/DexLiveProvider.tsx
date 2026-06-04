// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode, createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import { getDexExchanges, getMMStatus } from '../../services/dcrdexApi';
import type { DexNote, MMStatus, MMBotStatus } from '../../services/dcrdexApi';
import type { MarketSpot } from './useDexFeed';

type NoteListener = (note: DexNote) => void;

// LiveNote is a notify-feed note plus the extra fields the accumulated notes
// carry beyond the base DexNote (decred.org/dcrdex/client/core/notification.go):
// spots (SpotPriceNote), host/connectionStatus (ConnEventNote),
// host/authenticated (DEXAuthNote), and the BridgeNote fields.
export type LiveNote = DexNote & {
  spots?: Record<string, MarketSpot>;
  host?: string;
  connectionStatus?: number;
  authenticated?: boolean;
  sourceAssetID?: number;
  destAssetID?: number;
  txID?: string;
  completionTxIDs?: string[];
  amount?: number;
  complete?: boolean;
  // walletstate notes carry the wallet's live state (WalletState).
  wallet?: { symbol?: string; assetID?: number; running?: boolean; synced?: boolean };
};

// DexConn is a DEX server's live connection state: connectionStatus (1 ==
// connected, per comms.ConnectionStatus) and whether the account is
// authenticated. Fed by `conn` and `dex_auth` notes.
export interface DexConn {
  status: number;
  authed: boolean;
}

// DexBridge is the live state of a cross-chain bridge transaction, from `bridge`
// notes. Ingested now for a future bridge view; no UI consumes it yet.
export interface DexBridge {
  sourceAssetID: number;
  destAssetID: number;
  txID: string;
  completionTxIDs: string[];
  amount: number;
  complete: boolean;
  stamp: number;
}

// spotKey indexes spots by base/quote asset id, matching the markets list's
// `${baseID}-${quoteID}` row keys (avoids symbol-casing mismatches).
const spotKey = (s: MarketSpot) => `${s.baseID}-${s.quoteID}`;

// MM_NOTE_TYPES are the market-maker notifications. They carry only the market,
// so their arrival triggers a debounced refetch of the full market-making status
// rather than being accumulated directly.
const MM_NOTE_TYPES = new Set(['runstats', 'runevent', 'epochreport', 'cexproblems']);

interface DexLiveCtx {
  addNoteListener: (fn: NoteListener) => () => void;
  spots: Record<string, MarketSpot>;
  seedSpots: (snapshot: Record<string, MarketSpot>) => void;
  conns: Record<string, DexConn>;
  bridges: Record<string, DexBridge>;
  mmStatus: MMStatus | null;
  refreshMMStatus: () => void;
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
  // conns is the live per-host connection/auth state, fed by `conn` and
  // `dex_auth` notes; bridges is the live per-tx bridge state from `bridge`
  // notes (no UI consumes bridges yet, ingested for a future bridge view).
  const [conns, setConns] = useState<Record<string, DexConn>>({});
  const [bridges, setBridges] = useState<Record<string, DexBridge>>({});
  // mmStatus is the market-making status (bots + CEX state), refetched on MM
  // notes. Fetched lazily when a MM consumer mounts (refreshMMStatus); the fetch
  // 409s and is ignored while the DEX is locked.
  const [mmStatus, setMMStatus] = useState<MMStatus | null>(null);
  const mmFetching = useRef(false);
  const refreshMMStatus = useCallback(() => {
    if (mmFetching.current) return;
    mmFetching.current = true;
    getMMStatus()
      .then((s) => setMMStatus(s))
      .catch(() => {})
      .finally(() => {
        mmFetching.current = false;
      });
  }, []);
  const refreshMMRef = useRef(refreshMMStatus);
  refreshMMRef.current = refreshMMStatus;

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

  // Seed conns from the exchanges snapshot so an already-down server is known
  // at mount, before any live `conn` note arrives. Only disconnected hosts are
  // seeded: a seeded entry carries authed=false, and injecting one for a
  // healthy host would flip consumers that require status===1 && authed (the
  // stats-bar dot) until the first dex_auth note. Live state wins the merge.
  useEffect(() => {
    let cancelled = false;
    getDexExchanges()
      .then((xcs) => {
        if (cancelled) return;
        // The exchange entries carry no host field over the RPC; the host is
        // the map key.
        const seeded: Record<string, DexConn> = {};
        Object.entries(xcs).forEach(([host, x]) => {
          if (x && x.connectionStatus !== 1) {
            seeded[host] = { status: x.connectionStatus, authed: false };
          }
        });
        if (Object.keys(seeded).length) {
          setConns((prev) => ({ ...seeded, ...prev }));
        }
      })
      .catch(() => {
        /* best-effort snapshot; live notes still feed conns */
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let ws: WebSocket | null = null;
    let cancelled = false;
    let retry = 1000;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let mmTimer: ReturnType<typeof setTimeout> | null = null;

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
        const note = msg.payload as LiveNote;
        // spots is high-frequency and has no refresh listeners, so accumulate and
        // swallow it. conn/dex_auth/bridge update live state but still fall
        // through to listeners (e.g. the notifications bell refreshes on conn).
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
        if (note.type === 'conn' && note.host) {
          const host = note.host;
          setConns((prev) => ({
            ...prev,
            [host]: { status: note.connectionStatus ?? 0, authed: prev[host]?.authed ?? false },
          }));
        } else if (note.type === 'dex_auth' && note.host) {
          const host = note.host;
          setConns((prev) => ({
            ...prev,
            [host]: { status: prev[host]?.status ?? 0, authed: !!note.authenticated },
          }));
        } else if (note.type === 'bridge' && note.txID) {
          const txID = note.txID;
          setBridges((prev) => ({
            ...prev,
            [txID]: {
              sourceAssetID: note.sourceAssetID ?? 0,
              destAssetID: note.destAssetID ?? 0,
              txID,
              completionTxIDs: note.completionTxIDs ?? [],
              amount: note.amount ?? 0,
              complete: !!note.complete,
              stamp: note.stamp ?? Date.now(),
            },
          }));
        } else if (MM_NOTE_TYPES.has(note.type)) {
          // Coalesce a burst of MM notes into one status refetch.
          if (!mmTimer) {
            mmTimer = setTimeout(() => {
              mmTimer = null;
              refreshMMRef.current();
            }, 500);
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
      if (mmTimer) clearTimeout(mmTimer);
      try {
        ws?.close();
      } catch {
        /* ignore */
      }
    };
  }, []);

  return (
    <Ctx.Provider value={{ addNoteListener, spots, seedSpots, conns, bridges, mmStatus, refreshMMStatus }}>
      {children}
    </Ctx.Provider>
  );
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

// useDexConn returns a DEX server's live connection/auth state (from `conn` and
// `dex_auth` notes), or null before any note has arrived (callers fall back to
// the REST connectionStatus snapshot). A no-op null outside the provider.
export function useDexConn(host: string): DexConn | null {
  return useContext(Ctx)?.conns[host] ?? null;
}

// useMMStatus returns the shared market-making status, triggering an initial
// fetch on mount. It then refreshes automatically on MM notifications. Null
// outside the provider or before the first fetch resolves.
export function useMMStatus(): MMStatus | null {
  const ctx = useContext(Ctx);
  useEffect(() => {
    ctx?.refreshMMStatus();
  }, [ctx]);
  return ctx?.mmStatus ?? null;
}

// useMMRefresh returns the callback that refetches the market-making status,
// for use after a start/stop/config action. A no-op outside the provider.
export function useMMRefresh(): () => void {
  return useContext(Ctx)?.refreshMMStatus ?? noop;
}

// useMMBotRun returns the live status of the bot configured for the given market,
// or null. Used by the trade view to show activity for the selected market.
export function useMMBotRun(host: string, baseID: number, quoteID: number): MMBotStatus | null {
  const status = useMMStatus();
  return (
    status?.bots.find((b) => b.config.host === host && b.config.baseID === baseID && b.config.quoteID === quoteID) ??
    null
  );
}

// useDexBridges returns the live bridge-transaction map keyed by txID. No UI
// consumes it yet; ingested for a future bridge view. Empty outside the provider.
export function useDexBridges(): Record<string, DexBridge> {
  return useContext(Ctx)?.bridges ?? EMPTY_BRIDGES;
}

const EMPTY_SPOTS: Record<string, MarketSpot> = {};
const EMPTY_BRIDGES: Record<string, DexBridge> = {};
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

// useDexOnNotes calls handler with each notify-feed note whose type is in
// `types` (or every note when `types` is empty). Unlike useDexRefreshOnNotes it
// passes the note payload through (not debounced), for callers that need to
// inspect note contents. A no-op outside the provider.
export function useDexOnNotes(types: string[], handler: (note: LiveNote) => void) {
  const ctx = useContext(Ctx);
  const handlerRef = useRef(handler);
  handlerRef.current = handler;
  const typesKey = types.join(',');

  useEffect(() => {
    if (!ctx) return;
    const want = new Set(typesKey ? typesKey.split(',') : []);
    const off = ctx.addNoteListener((note) => {
      if (want.size && !want.has(note.type)) return;
      handlerRef.current(note);
    });
    return off;
  }, [ctx, typesKey]);
}
