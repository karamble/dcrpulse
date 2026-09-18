// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Loader2, Phone, PhoneOff } from 'lucide-react';
import { useIncomingCalls } from '../../../hooks/useIncomingCalls';

// IncomingCallPill is the only place an incoming call is visible from anywhere
// in the dashboard. It sits above the alerts pill, shows nothing while nobody
// is calling, and answers the newest call in place so the user does not have to
// find the Realtime tab first.
export const IncomingCallPill = () => {
  const { calls, accept, dismiss, busyRV, error } = useIncomingCalls();
  const call = calls[calls.length - 1];
  if (!call) return null;

  const who = call.inviterNick || `${call.inviter.slice(0, 12)}...`;
  const busy = busyRV === call.sessRV;

  const answer = async () => {
    if (await accept(call)) {
      // The room view only mounts for #realtime/room/<rv>; a bare #realtime
      // would drop the user on the room list instead of into the call.
      window.location.hash = `#realtime/room/${call.sessRV}`;
    }
  };

  return (
    <div className="fixed bottom-20 right-4 z-40 w-72 rounded-xl bg-card text-card-foreground shadow-xl ring-2 ring-primary/60 overflow-hidden">
      <div className="flex items-center gap-2 px-3.5 py-2.5 bg-primary text-primary-foreground">
        <Phone className="h-4 w-4 animate-pulse" />
        <span className="text-xs font-semibold">
          {call.isInstant ? 'Incoming call' : 'Call invitation'}
        </span>
        {calls.length > 1 && (
          <span className="ml-auto text-[11px] opacity-80">+{calls.length - 1} waiting</span>
        )}
      </div>

      <div className="px-3.5 py-3 space-y-2">
        <p className="text-sm font-medium truncate" title={call.inviter}>
          {who}
        </p>
        {call.description && (
          <p className="text-xs text-muted-foreground line-clamp-2">{call.description}</p>
        )}
        {error && <p className="text-xs text-destructive">{error}</p>}

        <div className="flex gap-2 pt-0.5">
          <button
            type="button"
            onClick={answer}
            disabled={busy}
            className="flex-1 inline-flex items-center justify-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-primary-foreground disabled:opacity-60"
          >
            {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Phone className="h-3.5 w-3.5" />}
            Answer
          </button>
          <button
            type="button"
            onClick={() => dismiss(call.sessRV)}
            disabled={busy}
            className="inline-flex items-center justify-center gap-1.5 rounded-md bg-muted px-3 py-1.5 text-xs font-semibold text-muted-foreground disabled:opacity-60"
          >
            <PhoneOff className="h-3.5 w-3.5" />
            Ignore
          </button>
        </div>
      </div>
    </div>
  );
};
