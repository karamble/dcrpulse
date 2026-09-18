// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Loader2, Phone, PhoneOff } from 'lucide-react';
import { useIncomingCalls } from './IncomingCallsProvider';

// IncomingInviteBanner stacks the waiting calls at the top of the Realtime tab.
// It shares useIncomingCalls with the global pill, so a call that arrived while
// this page was closed is listed here too rather than only one that happened to
// ring while the tab was mounted.
export const IncomingInviteBanner = ({
  activeRV,
  onAccepted,
}: {
  activeRV: string | null;
  onAccepted: (rv: string) => void;
}) => {
  const { calls, accept, dismiss, busyRV, error: err } = useIncomingCalls();

  const handleAccept = async (inv: (typeof calls)[number]) => {
    if (await accept(inv)) onAccepted(inv.sessRV);
  };

  // Filter out invites for the room we're currently in.
  const visible = calls.filter((i) => i.sessRV !== activeRV);
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
          className="rounded-xl bg-gradient-card border border-primary/40 p-4 flex items-center gap-3"
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
            title="Ignore this call"
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
