// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  AlertCircle,
  Coins,
  Loader2,
  Mic,
  MicOff,
  Phone,
  PhoneOff,
  Plus,
  Radio,
  RefreshCw,
  Send,
  UserMinus,
  UserPlus,
  Users,
  Volume2,
} from 'lucide-react';
import {
  BisonrelayLiveEvent,
  RTDTSession,
  RTDTSessionPublisher,
  dissolveRTDTSession,
  getBisonrelayRTDTMessages,
  joinRTDTSession,
  kickRTDTPeer,
  leaveRTDTSession,
  listRTDTSessions,
  rotateRTDTCookies,
  sendBisonrelayRTDTChat,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';
import { RealtimeAudioPipeline, supportsWebCodecsAudio } from './realtime/AudioPipeline';
import { IncomingInviteBanner } from './realtime/IncomingInviteBanner';
import { InstantCallModal } from './realtime/InstantCallModal';
import { InviteToRoomModal } from './realtime/InviteToRoomModal';
import { NewRoomModal } from './realtime/NewRoomModal';

const readHashRoom = (): string | null => {
  const h = window.location.hash.replace(/^#/, '');
  if (!h.startsWith('realtime')) return null;
  const rest = h.slice('realtime'.length);
  const m = rest.match(/^\/room\/([0-9a-fA-F]{64})$/);
  return m ? m[1] : null;
};

const navigateTo = (hash: string): void => {
  window.location.hash = hash;
};

export const BisonrelayRealtime = () => {
  const [activeRV, setActiveRV] = useState<string | null>(readHashRoom);

  useEffect(() => {
    const onHashChange = () => setActiveRV(readHashRoom());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  if (!supportsWebCodecsAudio()) {
    return <ChromeRequired />;
  }

  const goToRoom = (rv: string) => navigateTo(`realtime/room/${rv}`);

  return (
    <div className="space-y-4">
      <IncomingInviteBanner activeRV={activeRV} onAccepted={goToRoom} />
      {activeRV ? (
        <ActiveCallView rv={activeRV} onLeave={() => navigateTo('realtime')} />
      ) : (
        <RoomList onOpen={goToRoom} />
      )}
    </div>
  );
};

const ChromeRequired = () => (
  <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 flex items-start gap-3">
    <div className="p-2 rounded-lg bg-amber-500/15 border border-amber-500/30 shrink-0">
      <Radio className="h-5 w-5 text-amber-400" />
    </div>
    <div className="space-y-1">
      <h3 className="text-sm font-semibold">Voice calls require Chrome 130+ or Firefox 130+</h3>
      <p className="text-xs text-muted-foreground">
        This feature uses the WebCodecs AudioEncoder/AudioDecoder API for
        Opus-frame encoding. Safari support is not yet available.
      </p>
    </div>
  </div>
);

const RoomList = ({ onOpen }: { onOpen: (rv: string) => void }) => {
  const [sessions, setSessions] = useState<RTDTSession[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [showInstant, setShowInstant] = useState(false);
  const [showNewRoom, setShowNewRoom] = useState(false);
  const { addListener } = useBisonrelayLive();

  const reload = useCallback(async () => {
    try {
      setSessions(await listRTDTSessions());
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not list sessions');
    }
  }, []);

  useEffect(() => {
    reload();
    return addListener((evt) => {
      if (evt.type.startsWith('rtdt-')) {
        reload();
      }
    });
  }, [addListener, reload]);

  const handleJoin = async (rv: string) => {
    if (busy) return;
    setBusy(true);
    try {
      await joinRTDTSession(rv);
      onOpen(rv);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Join failed');
    } finally {
      setBusy(false);
    }
  };

  // Split sessions into "active calls" (live, currently in audio) and
  // "rooms" (created or accepted but not live). bruig groups them the
  // same way on its Realtime tab.
  const live = sessions?.filter((s) => s.live) ?? [];
  const idle = sessions?.filter((s) => !s.live) ?? [];

  return (
    <div className="space-y-4">
      <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-4 flex items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">Realtime calls</h3>
          <p className="text-[11px] text-muted-foreground mt-0.5">
            Voice calls over RTDT. Start a 1:1 instant call or create a group
            room to host up to 32 peers.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setShowInstant(true)}
            className="px-3 py-1.5 rounded-lg text-sm bg-gradient-primary text-white font-semibold inline-flex items-center gap-1.5"
          >
            <Phone className="h-4 w-4" /> Instant call
          </button>
          <button
            type="button"
            onClick={() => setShowNewRoom(true)}
            className="px-3 py-1.5 rounded-lg text-sm border border-border/50 text-foreground hover:bg-muted/30 inline-flex items-center gap-1.5"
          >
            <Plus className="h-4 w-4" /> New room
          </button>
        </div>
      </div>
      {err && <ErrorBanner msg={err} />}
      {sessions === null && !err && <Loading />}
      {sessions && sessions.length === 0 && (
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
          <p className="text-sm text-muted-foreground">
            No realtime sessions yet. Start an instant call or create a group
            room above.
          </p>
        </div>
      )}
      {live.length > 0 && (
        <SectionList
          title="Active calls"
          items={live}
          onOpen={onOpen}
          onJoin={handleJoin}
          busy={busy}
        />
      )}
      {idle.length > 0 && (
        <SectionList
          title="Rooms"
          items={idle}
          onOpen={onOpen}
          onJoin={handleJoin}
          busy={busy}
        />
      )}
      {showInstant && (
        <InstantCallModal
          onClose={() => setShowInstant(false)}
          onJoined={(rv) => {
            setShowInstant(false);
            onOpen(rv);
          }}
        />
      )}
      {showNewRoom && (
        <NewRoomModal
          onClose={() => setShowNewRoom(false)}
          onJoined={(rv) => {
            setShowNewRoom(false);
            onOpen(rv);
          }}
        />
      )}
    </div>
  );
};

const SectionList = ({
  title,
  items,
  onOpen,
  onJoin,
  busy,
}: {
  title: string;
  items: RTDTSession[];
  onOpen: (rv: string) => void;
  onJoin: (rv: string) => void;
  busy: boolean;
}) => (
  <div className="space-y-2">
    <div className="text-[10px] uppercase tracking-wide text-muted-foreground px-1">
      {title}
    </div>
    <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden divide-y divide-border/30">
      {items.map((s) => (
        <div key={s.rv} className="px-4 py-3 flex items-center gap-3">
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium truncate flex items-center gap-2">
              <span>{s.description || '(no description)'}</span>
              {s.is_instant && (
                <span className="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded bg-primary/15 text-primary">
                  Instant
                </span>
              )}
              {s.live && (
                <span className="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-400">
                  Live
                </span>
              )}
            </div>
            <div className="text-[10px] text-muted-foreground mt-0.5 font-mono break-all">
              {s.rv.slice(0, 16)}… · capacity {s.size} · {s.members.length} members
            </div>
          </div>
          {s.live ? (
            <button
              type="button"
              onClick={() => onOpen(s.rv)}
              className="px-3 py-1.5 rounded-md text-xs bg-gradient-primary text-white font-semibold inline-flex items-center gap-1.5"
            >
              <Phone className="h-3 w-3" /> Open
            </button>
          ) : (
            <button
              type="button"
              onClick={() => onJoin(s.rv)}
              disabled={busy}
              className="px-3 py-1.5 rounded-md text-xs border border-border/50 text-foreground hover:bg-muted/30 inline-flex items-center gap-1.5 disabled:opacity-50"
            >
              <Phone className="h-3 w-3" /> Join
            </button>
          )}
        </div>
      ))}
    </div>
  </div>
);

const ActiveCallView = ({ rv, onLeave }: { rv: string; onLeave: () => void }) => {
  const [pipeline, setPipeline] = useState<RealtimeAudioPipeline | null>(null);
  const [connected, setConnected] = useState(false);
  const [muted, setMuted] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [sendErr, setSendErr] = useState<string | null>(null);
  const [packetsSent, setPacketsSent] = useState(0);
  const [packetsInbound, setPacketsInbound] = useState(0);
  const [session, setSession] = useState<RTDTSession | null>(null);
  // Per-peer ticker state: regenerated every 250ms so the speaking
  // indicator + buffer-depth UI stay live without a heavier polling loop.
  const [livePeerIDs, setLivePeerIDs] = useState<number[]>([]);
  const [speakingMap, setSpeakingMap] = useState<Record<number, boolean>>({});
  const [bufferDepthMap, setBufferDepthMap] = useState<Record<number, number>>({});
  const [peerGains, setPeerGains] = useState<Record<number, number>>({});
  // Allowance ledger: count refresh events + cumulative matoms added.
  // BR doesn't expose the current remaining balance through the public
  // API, so a true progress bar is not possible from v1. We surface the
  // last refresh delta + time-since instead.
  const [allowanceRefreshes, setAllowanceRefreshes] = useState(0);
  const [allowanceAddedMatoms, setAllowanceAddedMatoms] = useState(0);
  const [lastRefreshAt, setLastRefreshAt] = useState<number | null>(null);
  const [showInvite, setShowInvite] = useState(false);
  const [busyAdmin, setBusyAdmin] = useState<string | null>(null);
  const [reconnecting, setReconnecting] = useState<{ attempt: number; nextMs: number } | null>(null);
  const { addListener } = useBisonrelayLive();

  // The AudioPipeline starts immediately on mount. If brclientd's live
  // UDP session isn't up yet (BR's POST /join returns before the live
  // session is registered, typically a 2-5s gap), the first WS attempt
  // 409s and the pipeline retries with the PRECONNECT backoff ladder.
  // No UI gate needed - the badge shows "Reconnecting (try N)..." until
  // the live session is ready and the WS upgrades cleanly.
  useEffect(() => {
    const p = new RealtimeAudioPipeline({
      rv,
      callbacks: {
        onConnected: () => {
          setConnected(true);
          setReconnecting(null);
        },
        onDisconnected: () => setConnected(false),
        onReconnecting: (attempt, nextMs) =>
          setReconnecting({ attempt, nextMs }),
        onError: (msg) => setErr(msg),
        onInboundFrame: () => setPacketsInbound((n) => n + 1),
      },
    });
    p.start().catch((e) => setErr(e?.message ?? String(e)));
    setPipeline(p);
    return () => {
      p.stop();
    };
  }, [rv]);

  // Tick outbound packet counter + per-peer activity / buffer state.
  useEffect(() => {
    if (!pipeline) return;
    const id = setInterval(() => {
      const c = pipeline.outboundCounters();
      setPacketsSent(c.sent);
      const ids = pipeline.livePeerIDs();
      setLivePeerIDs(ids);
      const speaking: Record<number, boolean> = {};
      const depth: Record<number, number> = {};
      for (const id of ids) {
        speaking[id] = pipeline.hasRecentSpeech(id, 250);
        const s = pipeline.peerStats(id);
        if (s) depth[id] = s.bufferDepthMs;
      }
      setSpeakingMap(speaking);
      setBufferDepthMap(depth);
    }, 250);
    return () => clearInterval(id);
  }, [pipeline]);

  // Pull session metadata so we can map peerID -> nick/alias. Refresh on
  // every rtdt-* event for our session.
  const reloadSession = useCallback(async () => {
    try {
      const all = await listRTDTSessions();
      const ours = all.find((s) => s.rv === rv) ?? null;
      setSession(ours);
    } catch {
      /* leave previous */
    }
  }, [rv]);

  useEffect(() => {
    reloadSession();
    return addListener((evt) => {
      if (!evt.type.startsWith('rtdt-')) return;
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      const sessRV = payload.sessRV ? String(payload.sessRV) : undefined;
      if (sessRV && sessRV !== rv) return;
      // Specific event handlers for the active call.
      switch (evt.type) {
        case 'rtdt-allowance-refreshed': {
          const added = Number(payload.addAllowance ?? 0);
          setAllowanceRefreshes((n) => n + 1);
          setAllowanceAddedMatoms((m) => m + added);
          setLastRefreshAt(Date.now());
          break;
        }
        case 'rtdt-send-error':
          setSendErr(String(payload.error ?? 'send error'));
          break;
        case 'rtdt-kicked':
        case 'rtdt-removed':
        case 'rtdt-dissolved':
          // Server-side teardown for our session: bounce back to list.
          onLeave();
          return;
      }
      reloadSession();
    });
  }, [addListener, onLeave, reloadSession, rv]);

  // Best-effort leave on tab close so other peers see us drop.
  useEffect(() => {
    const onBeforeUnload = () => {
      try {
        navigator.sendBeacon(`/api/br/rtdt/sessions/${rv}/leave`);
      } catch {
        /* ignore */
      }
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, [rv]);

  const handleHangup = async () => {
    if (pipeline) pipeline.stop();
    try {
      await leaveRTDTSession(rv);
    } catch {
      /* ignore */
    }
    onLeave();
  };

  const handleDissolve = async () => {
    if (pipeline) pipeline.stop();
    try {
      await dissolveRTDTSession(rv);
    } catch {
      /* ignore */
    }
    onLeave();
  };

  const handleMuteToggle = () => {
    if (!pipeline) return;
    const next = !muted;
    pipeline.setMuted(next);
    setMuted(next);
  };

  const handlePeerGainChange = (peerID: number, gain: number) => {
    if (!pipeline) return;
    pipeline.setPeerGain(peerID, gain);
    setPeerGains((prev) => ({ ...prev, [peerID]: gain }));
  };

  const handleKickPeer = async (peerID: number) => {
    if (busyAdmin) return;
    setBusyAdmin(`kick-${peerID}`);
    try {
      // 2-hour default ban; bruig uses the same default.
      await kickRTDTPeer(rv, peerID, 7200);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Kick failed');
    } finally {
      setBusyAdmin(null);
    }
  };

  const handleRotateCookies = async () => {
    if (busyAdmin) return;
    setBusyAdmin('rotate');
    try {
      await rotateRTDTCookies(rv);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Rotate failed');
    } finally {
      setBusyAdmin(null);
    }
  };

  // Build a peerID -> display label map from session metadata. Falls
  // back to the raw peer id when we have no nick (joined peer not yet
  // in the publisher list, or unknown user).
  const peerLabel = (peerID: number): string => {
    if (!session) return `peer ${peerID}`;
    const pub = session.publishers.find((p) => p.peer_id === peerID);
    if (pub && pub.alias) return pub.alias;
    return `peer ${peerID}`;
  };

  return (
    <div className="space-y-4">
      <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5 space-y-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-base font-semibold flex items-center gap-2">
              <Phone className="h-4 w-4 text-primary" /> In call
            </h3>
            <div className="text-[10px] text-muted-foreground font-mono break-all mt-1">
              {rv}
            </div>
          </div>
          <div
            className={`shrink-0 inline-flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-full border ${
              connected
                ? 'border-emerald-500/40 text-emerald-400 bg-emerald-500/10'
                : reconnecting
                  ? 'border-rose-500/40 text-rose-400 bg-rose-500/10'
                  : 'border-amber-500/40 text-amber-400 bg-amber-500/10'
            }`}
          >
            {connected
              ? 'Connected'
              : reconnecting
                ? `Reconnecting (try ${reconnecting.attempt})…`
                : 'Connecting…'}
          </div>
        </div>

        {err && <ErrorBanner msg={err} />}
        <RTDTChatPanel rv={rv} peerLabel={peerLabel} />
        {sendErr && (
          <div className="rounded-lg bg-amber-500/10 border border-amber-500/30 p-2.5 flex items-center justify-between gap-2 text-xs text-amber-300">
            <span className="break-words">RTDT send error: {sendErr}</span>
            <button
              type="button"
              onClick={() => setSendErr(null)}
              className="text-amber-300/80 hover:text-amber-200 shrink-0"
            >
              Dismiss
            </button>
          </div>
        )}

        <div className="grid grid-cols-3 gap-3">
          <CounterTile label="Sent (out)" value={packetsSent} hint="opus packets" />
          <CounterTile
            label="Received"
            value={packetsInbound}
            hint={`${livePeerIDs.length} active peer${livePeerIDs.length === 1 ? '' : 's'}`}
          />
          <AllowanceTile
            refreshes={allowanceRefreshes}
            addedMatoms={allowanceAddedMatoms}
            lastRefreshAt={lastRefreshAt}
          />
        </div>

        <div className="flex items-center justify-end gap-2 pt-1 flex-wrap">
          {session?.is_admin && (
            <>
              <button
                type="button"
                onClick={() => setShowInvite(true)}
                className="px-3 py-1.5 rounded-md text-xs border border-border/50 text-foreground hover:bg-muted/30 inline-flex items-center gap-1.5"
              >
                <UserPlus className="h-3 w-3" /> Invite
              </button>
              <button
                type="button"
                onClick={handleRotateCookies}
                disabled={busyAdmin === 'rotate'}
                title="Invalidate appointment cookies; kicked peers cannot rejoin with old cookies"
                className="px-3 py-1.5 rounded-md text-xs border border-border/50 text-foreground hover:bg-muted/30 inline-flex items-center gap-1.5 disabled:opacity-50"
              >
                {busyAdmin === 'rotate' ? (
                  <Loader2 className="h-3 w-3 animate-spin" />
                ) : (
                  <RefreshCw className="h-3 w-3" />
                )}
                Rotate cookies
              </button>
            </>
          )}
          <button
            type="button"
            onClick={handleMuteToggle}
            disabled={!connected}
            className={`px-3 py-1.5 rounded-md text-xs border inline-flex items-center gap-1.5 ${
              muted
                ? 'border-amber-500/40 text-amber-400 bg-amber-500/10'
                : 'border-border/50 text-foreground hover:bg-muted/30'
            } disabled:opacity-50 disabled:cursor-not-allowed`}
          >
            {muted ? <MicOff className="h-3 w-3" /> : <Mic className="h-3 w-3" />}
            {muted ? 'Muted' : 'Mic on'}
          </button>
          {session?.is_admin && (
            <button
              type="button"
              onClick={handleDissolve}
              title="Owner-only; dissolves the session for everyone"
              className="px-3 py-1.5 rounded-md text-xs border border-border/50 text-muted-foreground hover:text-foreground hover:bg-muted/30"
            >
              Dissolve
            </button>
          )}
          <button
            type="button"
            onClick={handleHangup}
            className="px-3 py-1.5 rounded-md text-xs bg-rose-500/20 text-rose-300 border border-rose-500/40 inline-flex items-center gap-1.5"
          >
            <PhoneOff className="h-3 w-3" /> Leave
          </button>
        </div>
      </div>

      <PeerRoster
        session={session}
        livePeerIDs={livePeerIDs}
        speakingMap={speakingMap}
        bufferDepthMap={bufferDepthMap}
        peerGains={peerGains}
        onPeerGainChange={handlePeerGainChange}
        peerLabel={peerLabel}
        canKick={!!session?.is_admin}
        onKickPeer={handleKickPeer}
        busyAdmin={busyAdmin}
      />

      {showInvite && session && (
        <InviteToRoomModal
          session={session}
          onClose={() => setShowInvite(false)}
          onInvited={() => setShowInvite(false)}
        />
      )}
    </div>
  );
};

// AllowanceTile shows last refresh delta + how long ago the BR server
// top-up landed. We can't show a true bar because BR doesn't expose the
// current remaining allowance balance.
const AllowanceTile = ({
  refreshes,
  addedMatoms,
  lastRefreshAt,
}: {
  refreshes: number;
  addedMatoms: number;
  lastRefreshAt: number | null;
}) => {
  const [, force] = useState(0);
  // Re-render every 5s so the "X ago" label freshens.
  useEffect(() => {
    const id = setInterval(() => force((n) => n + 1), 5000);
    return () => clearInterval(id);
  }, []);
  const dcr = addedMatoms / 1e11;
  const since = lastRefreshAt
    ? Math.max(0, Math.floor((Date.now() - lastRefreshAt) / 1000))
    : null;
  const sinceStr =
    since === null
      ? 'awaiting first refresh'
      : since < 60
        ? `${since}s ago`
        : since < 3600
          ? `${Math.floor(since / 60)}m ago`
          : `${Math.floor(since / 3600)}h ago`;
  return (
    <div className="rounded-lg border border-border/50 bg-background/40 p-3">
      <div className="text-[10px] text-muted-foreground uppercase tracking-wide flex items-center gap-1">
        <Coins className="h-3 w-3" /> Allowance
      </div>
      <div className="text-xl font-semibold text-foreground tabular-nums mt-0.5">
        {refreshes > 0 ? `+${dcr.toFixed(5).replace(/0+$/, '').replace(/\.$/, '')} DCR` : '—'}
      </div>
      <div className="text-[10px] text-muted-foreground mt-0.5">{sinceStr}</div>
    </div>
  );
};

const PeerRoster = ({
  session,
  livePeerIDs,
  speakingMap,
  bufferDepthMap,
  peerGains,
  onPeerGainChange,
  peerLabel,
  canKick,
  onKickPeer,
  busyAdmin,
}: {
  session: RTDTSession | null;
  livePeerIDs: number[];
  speakingMap: Record<number, boolean>;
  bufferDepthMap: Record<number, number>;
  peerGains: Record<number, number>;
  onPeerGainChange: (peerID: number, gain: number) => void;
  peerLabel: (peerID: number) => string;
  canKick: boolean;
  onKickPeer: (peerID: number) => void;
  busyAdmin: string | null;
}) => {
  // Build a unified row list from (a) the BR side (members/publishers
  // we know exist) and (b) live audio peers we have a decoder for. The
  // union catches the in-between state where audio is arriving for a
  // peer the session metadata hasn't refreshed yet.
  const fromSession: { peerID: number; pub?: RTDTSessionPublisher }[] = [];
  if (session) {
    for (const pub of session.publishers) {
      if (pub.peer_id === session.local_peer_id) continue;
      fromSession.push({ peerID: pub.peer_id, pub });
    }
  }
  const known = new Set(fromSession.map((r) => r.peerID));
  for (const id of livePeerIDs) {
    if (!known.has(id)) fromSession.push({ peerID: id });
  }
  fromSession.sort((a, b) => a.peerID - b.peerID);

  return (
    <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5 space-y-3">
      <h4 className="text-sm font-semibold flex items-center gap-2">
        <Users className="h-4 w-4 text-primary" />
        Peers
        <span className="text-[10px] text-muted-foreground font-mono ml-1">
          {livePeerIDs.length} live
        </span>
      </h4>

      {fromSession.length === 0 ? (
        <p className="text-xs text-muted-foreground italic">
          No remote peers yet. Audio starts flowing as soon as another peer
          joins the session.
        </p>
      ) : (
        <div className="space-y-2">
          {fromSession.map(({ peerID, pub }) => {
            const isLive = livePeerIDs.includes(peerID);
            const speaking = !!speakingMap[peerID];
            const bufMs = bufferDepthMap[peerID];
            const gain = peerGains[peerID] ?? 1.0;
            return (
              <div
                key={peerID}
                className="grid grid-cols-[auto_1fr_auto] gap-3 items-center px-3 py-2 rounded-lg bg-background/40 border border-border/40"
              >
                <div
                  className={`relative h-8 w-8 rounded-full flex items-center justify-center text-xs font-semibold ${
                    isLive
                      ? 'bg-primary/15 text-primary border border-primary/30'
                      : 'bg-muted/30 text-muted-foreground border border-border/50'
                  }`}
                >
                  {(pub?.alias || `p${peerID}`).slice(0, 2).toUpperCase()}
                  {speaking && (
                    <span
                      className="absolute inset-0 rounded-full ring-2 ring-emerald-400/70 animate-pulse"
                      aria-hidden
                    />
                  )}
                </div>
                <div className="min-w-0">
                  <div className="text-sm font-medium text-foreground truncate">
                    {peerLabel(peerID)}
                  </div>
                  <div className="text-[10px] text-muted-foreground">
                    {isLive
                      ? bufMs !== undefined
                        ? `Jitter buffer ${bufMs}ms`
                        : 'Live'
                      : 'Not in audio session'}
                    {speaking && (
                      <span className="ml-1.5 text-emerald-400">· Speaking</span>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <label className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                    <Volume2 className="h-3 w-3" />
                    <input
                      type="range"
                      min={0}
                      max={2}
                      step={0.05}
                      value={gain}
                      disabled={!isLive}
                      onChange={(e) => onPeerGainChange(peerID, Number(e.target.value))}
                      className="w-20 accent-primary disabled:opacity-40"
                    />
                    <span className="tabular-nums w-7 text-right">
                      {Math.round(gain * 100)}%
                    </span>
                  </label>
                  {canKick && (
                    <button
                      type="button"
                      onClick={() => onKickPeer(peerID)}
                      disabled={busyAdmin === `kick-${peerID}`}
                      title="Kick this peer from the live audio (2 hour ban)"
                      className="p-1.5 rounded text-muted-foreground hover:text-rose-400 hover:bg-rose-500/10 disabled:opacity-50"
                    >
                      {busyAdmin === `kick-${peerID}` ? (
                        <Loader2 className="h-3 w-3 animate-spin" />
                      ) : (
                        <UserMinus className="h-3 w-3" />
                      )}
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};

const CounterTile = ({
  label,
  value,
  hint,
}: {
  label: string;
  value: number;
  hint?: string;
}) => (
  <div className="rounded-lg border border-border/50 bg-background/40 p-3">
    <div className="text-[10px] text-muted-foreground uppercase tracking-wide">{label}</div>
    <div className="text-xl font-semibold text-foreground tabular-nums mt-0.5">{value}</div>
    {hint && <div className="text-[10px] text-muted-foreground mt-0.5">{hint}</div>}
  </div>
);

const ErrorBanner = ({ msg }: { msg: string }) => (
  <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 flex items-start gap-2 text-sm text-destructive">
    <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
    <span className="break-words">{msg}</span>
  </div>
);

const Loading = () => (
  <div className="flex items-center gap-2 text-sm text-muted-foreground">
    <Loader2 className="h-4 w-4 animate-spin" />
    <span>Loading sessions…</span>
  </div>
);

// RTDTChatPanel is the in-call text chat: history from the daemon's
// session-lifetime buffer, live append via the rtdt-chat event, plus a send
// box. Own messages are appended optimistically since the library does not
// loop them back.
const RTDTChatPanel = ({
  rv,
  peerLabel,
}: {
  rv: string;
  peerLabel: (peerID: number) => string;
}) => {
  const [msgs, setMsgs] = useState<{ label: string; message: string; ts: number }[]>([]);
  const [draft, setDraft] = useState('');
  const [sending, setSending] = useState(false);
  const [chatErr, setChatErr] = useState<string | null>(null);
  const { addListener } = useBisonrelayLive();
  const endRef = useRef<HTMLDivElement | null>(null);
  const peerLabelRef = useRef(peerLabel);
  peerLabelRef.current = peerLabel;

  useEffect(() => {
    getBisonrelayRTDTMessages(rv)
      .then((list) =>
        setMsgs(
          list.map((m) => ({
            label: peerLabelRef.current(m.peer_id),
            message: m.message,
            ts: m.timestamp,
          })),
        ),
      )
      .catch(() => {
        /* older daemon without the endpoint; panel starts empty */
      });
  }, [rv]);

  useEffect(() => {
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type !== 'rtdt-chat') return;
      const p = (evt.payload ?? {}) as Record<string, unknown>;
      if (p.sessRV !== rv) return;
      setMsgs((prev) => [
        ...prev,
        {
          label: peerLabelRef.current(Number(p.peerID ?? 0)),
          message: String(p.message ?? ''),
          ts: Number(p.ts ?? 0),
        },
      ]);
    });
  }, [addListener, rv]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: 'nearest' });
  }, [msgs]);

  const send = async (e: React.FormEvent) => {
    e.preventDefault();
    const text = draft.trim();
    if (!text || sending) return;
    setSending(true);
    setChatErr(null);
    try {
      await sendBisonrelayRTDTChat(rv, text);
      setMsgs((prev) => [
        ...prev,
        { label: 'you', message: text, ts: Math.floor(Date.now() / 1000) },
      ]);
      setDraft('');
    } catch (err: any) {
      const body = err?.response?.data;
      setChatErr(typeof body === 'string' ? body : err?.message || 'Send failed');
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="rounded-lg bg-background/40 border border-border/50 p-3 space-y-2">
      <div className="text-[10px] text-muted-foreground uppercase tracking-wide">Call chat</div>
      <div className="max-h-40 overflow-y-auto space-y-1">
        {msgs.length === 0 ? (
          <p className="text-xs text-muted-foreground italic">No messages yet.</p>
        ) : (
          msgs.map((m, i) => (
            <div key={i} className="text-xs break-words">
              <span className="font-medium text-foreground/90">{m.label}</span>
              <span className="text-muted-foreground"> {m.message}</span>
            </div>
          ))
        )}
        <div ref={endRef} />
      </div>
      {chatErr && <p className="text-xs text-destructive break-words">{chatErr}</p>}
      <form onSubmit={send} className="flex items-center gap-2">
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="Message the call…"
          disabled={sending}
          className="flex-1 px-3 py-1.5 rounded-lg bg-background border border-border text-foreground text-xs focus:outline-none focus:border-primary disabled:opacity-50"
        />
        <button
          type="submit"
          disabled={!draft.trim() || sending}
          className="p-1.5 rounded-lg bg-primary/20 text-primary hover:bg-primary/30 transition-colors disabled:opacity-50"
          aria-label="Send"
        >
          <Send className="h-3.5 w-3.5" />
        </button>
      </form>
    </div>
  );
};
