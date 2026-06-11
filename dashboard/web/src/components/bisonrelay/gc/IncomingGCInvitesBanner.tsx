// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Info, Loader2, LogOut, MessageSquare, Users, X } from 'lucide-react';
import {
  acceptBisonrelayGCInvite,
  listBisonrelayGCInvites,
  partBisonrelayGC,
  BlockedGCReinvite,
} from '../../../services/bisonrelayApi';
import { useBisonrelayLive } from '../BisonrelayLiveProvider';

interface PendingGCInvite {
  iid: number;
  gcid: string;
  name: string;
  description: string;
  from: string;
  fromNick: string;
  expires: number;
  version: number;
}

// IncomingGCInvitesBanner subscribes to gc-invited live events AND pulls
// the existing pending-invites list once on mount (so banners survive
// page reload until accepted). Banners stack at the top of the chat
// surface; Dismiss only removes the local banner, the invite remains in
// brclientd until the user explicitly accepts or it expires.
//
// It also surfaces blocked re-invites: invites the BR client rejected
// because a local copy of the GC already exists (stale after a restore).
// The recovery action leaves the local copy; the sender must then issue
// a fresh invite, which shows up here as a normal invite.
export const IncomingGCInvitesBanner = ({
  onAccepted,
}: {
  onAccepted: () => void;
}) => {
  const [pending, setPending] = useState<PendingGCInvite[]>([]);
  const [blocked, setBlocked] = useState<BlockedGCReinvite[]>([]);
  const [leftHints, setLeftHints] = useState<BlockedGCReinvite[]>([]);
  const [busyIID, setBusyIID] = useState<number | null>(null);
  const [busyGCID, setBusyGCID] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const { addListener } = useBisonrelayLive();

  // Initial fetch so the banners survive page reloads.
  useEffect(() => {
    listBisonrelayGCInvites()
      .then(({ invites, blocked_reinvites }) => {
        const fresh = invites
          .filter((i) => !i.accepted)
          .map<PendingGCInvite>((i) => ({
            iid: i.id,
            gcid: i.gcid,
            name: i.name,
            description: i.description ?? '',
            from: i.from,
            fromNick: '',
            expires: i.expires,
            version: i.version,
          }));
        setPending(fresh);
        setBlocked(blocked_reinvites);
      })
      .catch((e: any) => {
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not load invites');
      });
  }, []);

  // Live: append new invites and upsert blocked re-invites as they arrive.
  useEffect(() => {
    return addListener((evt) => {
      if (evt.type === 'gc-reinvite-blocked') {
        const p = (evt.payload ?? {}) as Record<string, unknown>;
        const gcid = String(p.gcid ?? '');
        if (!gcid) return;
        const entry: BlockedGCReinvite = {
          gcid,
          name: String(p.name ?? ''),
          from: String(p.from ?? ''),
          fromNick: String(p.fromNick ?? ''),
          count: Number(p.count ?? 1),
          lastAttempt: String(p.lastAttempt ?? ''),
        };
        setBlocked((prev) => [...prev.filter((x) => x.gcid !== gcid), entry]);
        return;
      }
      if (evt.type !== 'gc-invited') return;
      const p = (evt.payload ?? {}) as Record<string, unknown>;
      const iid = Number(p.iid ?? 0);
      if (!iid) return;
      const inv: PendingGCInvite = {
        iid,
        gcid: String(p.gcid ?? ''),
        name: String(p.name ?? ''),
        description: String(p.description ?? ''),
        from: String(p.from ?? ''),
        fromNick: String(p.fromNick ?? ''),
        expires: Number(p.expires ?? 0),
        version: Number(p.version ?? 0),
      };
      setPending((prev) => {
        if (prev.some((x) => x.iid === iid)) return prev;
        return [...prev, inv];
      });
      // A stored invite means the GC no longer blocks re-inviting.
      if (inv.gcid) {
        setBlocked((prev) => prev.filter((x) => x.gcid !== inv.gcid));
        setLeftHints((prev) => prev.filter((x) => x.gcid !== inv.gcid));
      }
    });
  }, [addListener]);

  const dismiss = useCallback((iid: number) => {
    setPending((prev) => prev.filter((x) => x.iid !== iid));
  }, []);

  const dismissBlocked = useCallback((gcid: string) => {
    setBlocked((prev) => prev.filter((x) => x.gcid !== gcid));
  }, []);

  const dismissHint = useCallback((gcid: string) => {
    setLeftHints((prev) => prev.filter((x) => x.gcid !== gcid));
  }, []);

  const handleAccept = async (inv: PendingGCInvite) => {
    if (busyIID) return;
    setBusyIID(inv.iid);
    setErr(null);
    try {
      await acceptBisonrelayGCInvite(inv.iid);
      dismiss(inv.iid);
      onAccepted();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Accept failed');
    } finally {
      setBusyIID(null);
    }
  };

  const handleLeaveBlocked = async (b: BlockedGCReinvite) => {
    if (busyGCID) return;
    setBusyGCID(b.gcid);
    setErr(null);
    try {
      await partBisonrelayGC(b.gcid);
      dismissBlocked(b.gcid);
      setLeftHints((prev) => [...prev.filter((x) => x.gcid !== b.gcid), b]);
      onAccepted();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Leave failed');
    } finally {
      setBusyGCID(null);
    }
  };

  if (pending.length === 0 && blocked.length === 0 && leftHints.length === 0 && !err) {
    return null;
  }

  return (
    <div className="space-y-2 mb-3">
      {err && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 text-xs text-destructive break-words">
          {err}
        </div>
      )}
      {leftHints.map((b) => (
        <div
          key={`hint-${b.gcid}`}
          className="rounded-lg bg-primary/10 border border-primary/30 p-3 flex items-center gap-3 text-xs"
        >
          <Info className="h-4 w-4 text-primary shrink-0" />
          <div className="flex-1 min-w-0">
            Stale copy of {b.name || '(unnamed)'} removed. Ask{' '}
            {b.fromNick || b.from.slice(0, 12)} for a new invite; it will show up here.
          </div>
          <button
            type="button"
            onClick={() => dismissHint(b.gcid)}
            className="px-2.5 py-1.5 rounded-md text-xs border border-border/50 text-muted-foreground hover:text-foreground hover:bg-muted/30 inline-flex items-center gap-1.5"
          >
            <X className="h-3 w-3" /> Dismiss
          </button>
        </div>
      ))}
      {blocked.map((b) => (
        <div
          key={`blocked-${b.gcid}`}
          className="rounded-xl bg-amber-500/10 border border-amber-500/30 p-3 flex items-center gap-3"
        >
          <div className="h-8 w-8 rounded-full bg-amber-500/15 border border-amber-500/30 flex items-center justify-center shrink-0">
            <Users className="h-4 w-4 text-amber-400" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium truncate text-amber-300">
              Group re-invite blocked: {b.name || '(unnamed)'}
            </div>
            <div className="text-[11px] text-muted-foreground break-words">
              {b.fromNick || b.from.slice(0, 12)} invited you, but your local copy of this
              group blocks it (stale after a restore). Leave the local copy, then ask{' '}
              {b.fromNick || 'the sender'} for a new invite.
            </div>
          </div>
          <button
            type="button"
            onClick={() => dismissBlocked(b.gcid)}
            disabled={busyGCID === b.gcid}
            className="px-2.5 py-1.5 rounded-md text-xs border border-border/50 text-muted-foreground hover:text-foreground hover:bg-muted/30 inline-flex items-center gap-1.5 disabled:opacity-50"
            title="Dismiss banner (it returns on the next blocked attempt)"
          >
            <X className="h-3 w-3" /> Dismiss
          </button>
          <button
            type="button"
            onClick={() => handleLeaveBlocked(b)}
            disabled={busyGCID === b.gcid}
            className="px-3 py-1.5 rounded-md text-xs bg-amber-500/20 border border-amber-500/40 text-amber-300 font-semibold inline-flex items-center gap-1.5 hover:bg-amber-500/30 disabled:opacity-50"
          >
            {busyGCID === b.gcid ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <LogOut className="h-3 w-3" />
            )}
            Leave local copy
          </button>
        </div>
      ))}
      {pending.map((inv) => (
        <div
          key={inv.iid}
          className="rounded-xl bg-gradient-card backdrop-blur-sm border border-primary/40 p-3 flex items-center gap-3"
        >
          <div className="h-8 w-8 rounded-full bg-primary/15 border border-primary/30 flex items-center justify-center shrink-0">
            <Users className="h-4 w-4 text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium truncate">
              Group invite: {inv.name || '(unnamed)'}
            </div>
            <div className="text-[11px] text-muted-foreground truncate">
              From {inv.fromNick || inv.from.slice(0, 12)}
              {inv.description ? ` · ${inv.description}` : ''}
            </div>
          </div>
          <button
            type="button"
            onClick={() => dismiss(inv.iid)}
            disabled={busyIID === inv.iid}
            className="px-2.5 py-1.5 rounded-md text-xs border border-border/50 text-muted-foreground hover:text-foreground hover:bg-muted/30 inline-flex items-center gap-1.5 disabled:opacity-50"
            title="Dismiss banner (invite stays on BR side)"
          >
            <X className="h-3 w-3" /> Dismiss
          </button>
          <button
            type="button"
            onClick={() => handleAccept(inv)}
            disabled={busyIID === inv.iid}
            className="px-3 py-1.5 rounded-md text-xs bg-gradient-primary text-white font-semibold inline-flex items-center gap-1.5 disabled:opacity-50"
          >
            {busyIID === inv.iid ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <MessageSquare className="h-3 w-3" />
            )}
            Accept
          </button>
        </div>
      ))}
    </div>
  );
};
