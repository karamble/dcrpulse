// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { RTDTSession } from '../../../services/bisonrelayApi';

// A call is not answerable the moment it is created. Bison Relay only joins us
// once the session update carrying the other side's publisher key has been
// exchanged, and joining sooner caches that peer as unkeyed for the rest of the
// call. These phases describe the wait so the UI can show it rather than
// pretending the call is already up.
export type CallPhase =
  | 'ringing' // we called, nobody has answered yet
  | 'awaiting-host' // we answered, the caller's keys have not reached us
  | 'connecting' // keys are in place, Bison Relay's join is in flight
  | 'not-joined' // a room we are a member of but have not joined
  | 'live'; // joined; audio may start

// hasOwnerKey reports whether the session owner's publisher entry has arrived.
// The owner is always its own first publisher, so this is exactly "the first
// session update landed" and is what makes the owner decryptable.
export const hasOwnerKey = (s: RTDTSession): boolean =>
  s.publishers.some((p) => p.uid === s.owner);

// localUID finds our own member entry by the peer id Bison Relay assigned us.
const localUID = (s: RTDTSession): string | undefined =>
  s.members.find((m) => m.peer_id === s.local_peer_id)?.uid;

export const isOwner = (s: RTDTSession): boolean => {
  const me = localUID(s);
  return !!me && me === s.owner;
};

export const callPhase = (s: RTDTSession | null): CallPhase => {
  if (!s) return 'connecting';
  if (s.live) return 'live';

  if (isOwner(s)) {
    // Our own room: joining is an explicit choice, never a wait.
    if (!s.is_instant) return 'not-joined';
    const answered = s.members.some((m) => m.peer_id !== s.local_peer_id && m.accepted);
    return answered ? 'connecting' : 'ringing';
  }

  if (!hasOwnerKey(s)) return 'awaiting-host';
  return s.is_instant ? 'connecting' : 'not-joined';
};
