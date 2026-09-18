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
  | 'joining' // a room we are joining ourselves, since BR only joins instant calls
  | 'not-joined' // that join failed; the user has to ask for it
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

// joinFailed is set once an automatic join has been tried and refused, which is
// the only case where the user has to ask for one.
export const callPhase = (s: RTDTSession | null, joinFailed = false): CallPhase => {
  if (!s) return 'connecting';
  if (s.live) return 'live';

  if (isOwner(s)) {
    // Bison Relay joins nobody into a room, not even its owner, so opening one
    // we are not in means joining it.
    if (!s.is_instant) return joinFailed ? 'not-joined' : 'joining';
    const answered = s.members.some((m) => m.peer_id !== s.local_peer_id && m.accepted);
    return answered ? 'connecting' : 'ringing';
  }

  if (!hasOwnerKey(s)) return 'awaiting-host';
  // An instant call joins itself; a room is ours to join once we hold the keys.
  if (s.is_instant) return 'connecting';
  return joinFailed ? 'not-joined' : 'joining';
};
