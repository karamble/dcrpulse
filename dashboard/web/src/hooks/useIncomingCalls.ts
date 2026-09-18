// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  acceptRTDTSession,
  joinRTDTSession,
  listRTDTInvites,
} from '../services/bisonrelayApi';
import { apiError } from '../utils/apiError';
import { useBisonrelayLive } from '../components/bisonrelay/BisonrelayLiveProvider';

export interface IncomingCall {
  sessRV: string;
  inviter: string;
  inviterNick: string;
  size: number;
  description: string;
  asPublisher: boolean;
  isInstant: boolean;
  receivedMs: number;
}

// A live rtdt-invited event and the stored invite describe the same thing in
// different shapes, so both are normalised to one.
const fromEvent = (p: Record<string, unknown>): IncomingCall => ({
  sessRV: String(p.sessRV ?? ''),
  inviter: String(p.inviter ?? ''),
  inviterNick: String(p.inviterNick ?? ''),
  size: Number(p.size ?? 0),
  description: String(p.description ?? ''),
  asPublisher: Boolean(p.asPublisher),
  isInstant: Boolean(p.isInstant),
  receivedMs: Date.now(),
});

// useIncomingCalls tracks the calls waiting to be answered. It loads the
// outstanding ones rather than only listening for new events, because an
// invitation that arrived while this page was closed would otherwise never be
// seen: the event is gone and Bison Relay cannot list its own stored invites.
export const useIncomingCalls = () => {
  const [calls, setCalls] = useState<IncomingCall[]>([]);
  const [busyRV, setBusyRV] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { addListener } = useBisonrelayLive();

  // Dismissing hides a call locally. It stays outstanding on the daemon until
  // it is accepted or cancelled, so without this a reload would bring it back.
  const dismissed = useRef<Set<string>>(new Set());

  const merge = useCallback((next: IncomingCall[]) => {
    setCalls(
      next.filter((c) => c.sessRV && c.inviter && !dismissed.current.has(c.sessRV)),
    );
  }, []);

  const reload = useCallback(async () => {
    try {
      const rows = await listRTDTInvites();
      merge(
        rows.map((r) => ({
          sessRV: r.sess_rv,
          inviter: r.inviter,
          inviterNick: r.inviter_nick,
          size: r.size,
          description: r.description,
          asPublisher: r.as_publisher,
          isInstant: r.is_instant,
          receivedMs: r.received_ms,
        })),
      );
    } catch {
      // A call we cannot list is not worth an error banner; the live event
      // still delivers anything that arrives from now on.
    }
  }, [merge]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    return addListener((evt) => {
      if (evt.type === 'rtdt-invited') {
        const call = fromEvent((evt.payload ?? {}) as Record<string, unknown>);
        if (!call.sessRV || !call.inviter || dismissed.current.has(call.sessRV)) return;
        setCalls((prev) =>
          prev.some((c) => c.sessRV === call.sessRV) ? prev : [...prev, call],
        );
        return;
      }
      if (evt.type === 'rtdt-invite-canceled' || evt.type === 'rtdt-dissolved') {
        const rv = String(((evt.payload ?? {}) as Record<string, unknown>).sessRV ?? '');
        if (rv) setCalls((prev) => prev.filter((c) => c.sessRV !== rv));
      }
    });
  }, [addListener]);

  const dismiss = useCallback((rv: string) => {
    dismissed.current.add(rv);
    setCalls((prev) => prev.filter((c) => c.sessRV !== rv));
  }, []);

  const accept = useCallback(async (call: IncomingCall): Promise<boolean> => {
    if (busyRV) return false;
    setBusyRV(call.sessRV);
    setError(null);
    try {
      await acceptRTDTSession(call.sessRV, call.inviter, call.asPublisher);
      try {
        await joinRTDTSession(call.sessRV);
      } catch (e: any) {
        // Bison Relay auto-joins instant calls on accept, so /join then
        // reports the session is already maintained. Only that is expected.
        const msg = apiError(e, '');
        if (!/already|pending|maintained/i.test(msg)) throw e;
      }
      setCalls((prev) => prev.filter((c) => c.sessRV !== call.sessRV));
      return true;
    } catch (e: any) {
      setError(apiError(e, 'Could not answer the call'));
      return false;
    } finally {
      setBusyRV(null);
    }
  }, [busyRV]);

  return { calls, accept, dismiss, reload, busyRV, error };
};
