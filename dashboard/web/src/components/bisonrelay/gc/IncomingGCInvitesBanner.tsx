// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Loader2, MessageSquare, Users, X } from 'lucide-react';
import {
  acceptBisonrelayGCInvite,
  listBisonrelayGCInvites,
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
export const IncomingGCInvitesBanner = ({
  onAccepted,
}: {
  onAccepted: () => void;
}) => {
  const [pending, setPending] = useState<PendingGCInvite[]>([]);
  const [busyIID, setBusyIID] = useState<number | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const { addListener } = useBisonrelayLive();

  // Initial fetch so the banner survives page reloads.
  useEffect(() => {
    listBisonrelayGCInvites()
      .then((invites) => {
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
      })
      .catch((e: any) => {
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not load invites');
      });
  }, []);

  // Live: append new invites as they arrive.
  useEffect(() => {
    return addListener((evt) => {
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
    });
  }, [addListener]);

  const dismiss = useCallback((iid: number) => {
    setPending((prev) => prev.filter((x) => x.iid !== iid));
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

  if (pending.length === 0 && !err) return null;

  return (
    <div className="space-y-2 mb-3">
      {err && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 text-xs text-destructive break-words">
          {err}
        </div>
      )}
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
