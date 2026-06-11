// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Loader2, Phone, PhoneOff } from 'lucide-react';
import { acceptRTDTSession, joinRTDTSession } from '../../../services/bisonrelayApi';
import { useBisonrelayLive } from '../BisonrelayLiveProvider';

interface PendingInvite {
  inviter: string;
  inviterNick: string;
  sessRV: string;
  size: number;
  description: string;
  isInstant: boolean;
  asPublisher: boolean;
  receivedAt: number;
}

// IncomingInviteBanner subscribes to the rtdt-invited event and stacks
// banners at the top of the Realtime tab. Accept invokes the BR
// accept-invite + auto-joins the live audio + routes the caller's
// onAccepted(rv) so the parent component can navigate to the active call.
// Dismiss just removes it from the banner; the invite remains on the BR
// side and will be re-surfaced on page reload via the stored invite.
export const IncomingInviteBanner = ({
  activeRV,
  onAccepted,
}: {
  activeRV: string | null;
  onAccepted: (rv: string) => void;
}) => {
  const [pending, setPending] = useState<PendingInvite[]>([]);
  const [busyRV, setBusyRV] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const { addListener } = useBisonrelayLive();

  useEffect(() => {
    return addListener((evt) => {
      if (evt.type !== 'rtdt-invited') return;
      const p = (evt.payload ?? {}) as Record<string, unknown>;
      const inv: PendingInvite = {
        inviter: String(p.inviter ?? ''),
        inviterNick: String(p.inviterNick ?? ''),
        sessRV: String(p.sessRV ?? ''),
        size: Number(p.size ?? 0),
        description: String(p.description ?? ''),
        isInstant: Boolean(p.isInstant),
        asPublisher: Boolean(p.asPublisher),
        receivedAt: Date.now(),
      };
      if (!inv.sessRV || !inv.inviter) return;
      setPending((prev) => {
        // De-duplicate by sessRV: a re-broadcasted invite shouldn't
        // stack a second banner.
        if (prev.some((x) => x.sessRV === inv.sessRV)) return prev;
        return [...prev, inv];
      });
    });
  }, [addListener]);

  const dismiss = useCallback((rv: string) => {
    setPending((prev) => prev.filter((x) => x.sessRV !== rv));
  }, []);

  const handleAccept = async (inv: PendingInvite) => {
    if (busyRV) return;
    setBusyRV(inv.sessRV);
    setErr(null);
    try {
      await acceptRTDTSession(inv.sessRV, inv.inviter, inv.asPublisher);
      try {
        await joinRTDTSession(inv.sessRV);
      } catch (e: any) {
        const body = e?.response?.data;
        const msg = typeof body === 'string' ? body : e?.message || '';
        // BR auto-joins instant calls on accept, in which case /join
        // returns an "already pending" style error. Only suppress THAT
        // kind of error; anything else is a real problem the user
        // should see.
        if (!/already|pending|maintained/i.test(msg)) {
          throw e;
        }
      }
      dismiss(inv.sessRV);
      onAccepted(inv.sessRV);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Accept failed');
    } finally {
      setBusyRV(null);
    }
  };

  // Filter out invites for the room we're currently in.
  const visible = pending.filter((i) => i.sessRV !== activeRV);
  if (visible.length === 0 && !err) return null;

  return (
    <div className="space-y-2">
      {err && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 text-xs text-destructive break-words">
          {err}
        </div>
      )}
      {visible.map((inv) => (
        <div
          key={inv.sessRV}
          className="rounded-xl bg-gradient-card backdrop-blur-sm border border-primary/40 p-4 flex items-center gap-3"
        >
          <div className="h-9 w-9 rounded-full bg-primary/15 border border-primary/30 flex items-center justify-center shrink-0">
            <Phone className="h-4 w-4 text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium truncate">
              Incoming call from {inv.inviterNick || inv.inviter.slice(0, 12)}
            </div>
            <div className="text-[11px] text-muted-foreground truncate">
              {inv.isInstant ? '1:1 instant call' : `Group room (cap ${inv.size})`}
              {inv.description ? ` · ${inv.description}` : ''}
            </div>
          </div>
          <button
            type="button"
            onClick={() => dismiss(inv.sessRV)}
            disabled={busyRV === inv.sessRV}
            className="px-2.5 py-1.5 rounded-md text-xs border border-border/50 text-muted-foreground hover:text-foreground hover:bg-muted/30 inline-flex items-center gap-1.5 disabled:opacity-50"
            title="Dismiss banner (invite stays on BR side)"
          >
            <PhoneOff className="h-3 w-3" /> Dismiss
          </button>
          <button
            type="button"
            onClick={() => handleAccept(inv)}
            disabled={busyRV === inv.sessRV}
            className="px-3 py-1.5 rounded-md text-xs bg-gradient-primary text-white font-semibold inline-flex items-center gap-1.5 disabled:opacity-50"
          >
            {busyRV === inv.sessRV ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <Phone className="h-3 w-3" />
            )}
            Accept
          </button>
        </div>
      ))}
    </div>
  );
};
