// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { BisonrelayContact } from '../../services/bisonrelayApi';

// displayNick renders the best name a contact has: a local alias if one was
// set, else the nick they published, else a prefix of their identity. The
// final fallback covers a contact with no identity at all, which the wire type
// allows and brclientd does not produce.
export const displayNick = (c: BisonrelayContact): string =>
  c.nick_alias || c.id?.nick || c.id?.identity?.slice(0, 12) || 'unknown';

// contactByUid finds a contact by identity. Neither /br/contacts nor the id
// validator normalises case, so both sides are lowercased.
export const contactByUid = (
  uid: string,
  contacts: BisonrelayContact[],
): BisonrelayContact | undefined => {
  const want = uid.toLowerCase();
  return contacts.find((c) => c.id?.identity?.toLowerCase() === want);
};

// displayStatsNick makes the same choice for the stats endpoint's flat contact
// shape, which carries uid and nick directly rather than a nested id.
export const displayStatsNick = (c: {
  uid: string;
  nick?: string;
  nick_alias?: string;
}): string => c.nick_alias || c.nick || c.uid.slice(0, 12) || 'unknown';
