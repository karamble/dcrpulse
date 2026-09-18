// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { useBisonrelayLive } from '../components/bisonrelay/BisonrelayLiveProvider';

export interface RealtimeNotice {
  id: number;
  text: string;
  tone: 'info' | 'warn';
}

const secondsAsText = (s: number): string => {
  if (s <= 0) return '';
  if (s < 60) return `${s} seconds`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m} minute${m === 1 ? '' : 's'}`;
  const h = Math.round(m / 60);
  return `${h} hour${h === 1 ? '' : 's'}`;
};

const who = (p: Record<string, unknown>): string => {
  const nick = String(p.byNick ?? p.acceptorNick ?? p.inviterNick ?? '').trim();
  if (nick) return nick;
  const uid = String(p.by ?? p.acceptor ?? p.inviter ?? '').trim();
  return uid ? `${uid.slice(0, 12)}…` : 'Someone';
};

// noticeFor turns a realtime event into the sentence a user should see, or null
// when the event changes state the UI already reflects elsewhere. Events the
// call view handles itself are deliberately absent.
const noticeFor = (type: string, p: Record<string, unknown>): Omit<RealtimeNotice, 'id'> | null => {
  switch (type) {
    case 'rtdt-dissolved':
      return { text: `${who(p)} ended the call`, tone: 'info' };
    case 'rtdt-removed': {
      const reason = String(p.reason ?? '').trim();
      return {
        text: reason ? `${who(p)} removed you: ${reason}` : `${who(p)} removed you from the room`,
        tone: 'warn',
      };
    }
    case 'rtdt-kicked': {
      const ban = secondsAsText(Number(p.banSeconds ?? 0));
      return {
        text: ban ? `You were removed from the call for ${ban}` : 'You were removed from the call',
        tone: 'warn',
      };
    }
    case 'rtdt-invite-canceled':
      return { text: `${who(p)} cancelled the call`, tone: 'info' };
    case 'rtdt-cookies-rotated':
      return { text: `${who(p)} rotated this room's keys`, tone: 'info' };
    case 'rtdt-admin-cookies':
      return { text: `${who(p)} made you an admin of this room`, tone: 'info' };
    default:
      return null;
  }
};

// useRealtimeNotices carries the realtime events that matter even when the user
// is somewhere else in the dashboard. Being removed from a call used to bounce
// the view with no explanation; this is where the explanation lives.
export const useRealtimeNotices = () => {
  const [notices, setNotices] = useState<RealtimeNotice[]>([]);
  const { addListener } = useBisonrelayLive();

  const dismiss = useCallback((id: number) => {
    setNotices((prev) => prev.filter((n) => n.id !== id));
  }, []);

  useEffect(() => {
    let seq = 0;
    return addListener((evt) => {
      if (!evt.type.startsWith('rtdt-')) return;
      const n = noticeFor(evt.type, (evt.payload ?? {}) as Record<string, unknown>);
      if (!n) return;
      // Stamped here rather than from the event: the bus timestamp is when
      // brclientd published it, which is not when this tab found out.
      const id = ++seq;
      setNotices((prev) => [...prev.slice(-2), { id, ...n }]);
      setTimeout(() => setNotices((prev) => prev.filter((x) => x.id !== id)), 8000);
    });
  }, [addListener]);

  return { notices, dismiss };
};
