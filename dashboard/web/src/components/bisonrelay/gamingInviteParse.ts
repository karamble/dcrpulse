// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// A game invite is an ordinary chat message, not protocol traffic.
//
// That is deliberate. Gameplay rides a hidden --gaming[...]-- envelope because
// it is between the running games and nobody should read it as conversation.
// An invitation is the opposite: a person is asking other people to play, and
// it has to be legible to whoever receives it - including someone whose client
// knows nothing about games, who should see an invitation they can act on
// rather than nothing at all. It is also what starts a game, so there is no
// game running to deliver it to.
//
// The link is rendered as a chip the same way an lnpay:// invoice is.
//
//   gaming://poker/table?buyin=10000000&seats=6&sid=<hex>
//
// The host is the game id - the same routing key the wire envelope carries -
// so one renderer serves every game. Only the fields the host can present
// honestly are read here: what the game does with the rest is the game's
// business.
const GAMING_INVITE_RE = /\bgaming:\/\/([a-z0-9][a-z0-9_-]{0,31})\/([a-z0-9_-]{1,32})(\?[^\s]*)?/gi;

export type GamingInvite = {
  /** The game id, which is also the wire envelope's routing key. */
  game: string;
  /** What is being offered. "table" is the only kind today. */
  kind: string;
  /** Buy-in in atoms, when the invite states one. */
  buyinAtoms: number | null;
  /** Seats at the table, when stated. */
  seats: number | null;
  /** The session the invite refers to, carried through to accept. */
  sid: string;
  /** The whole link, so accept can hand it back untouched. */
  raw: string;
};

export type GamingInvitePart =
  | { kind: 'text'; text: string }
  | { kind: 'invite'; invite: GamingInvite };

const intOrNull = (v: string | null): number | null => {
  if (v === null) return null;
  const n = Number(v);
  return Number.isFinite(n) && n >= 0 ? Math.floor(n) : null;
};

const parseInvite = (game: string, kind: string, query: string, raw: string): GamingInvite => {
  const params = new URLSearchParams(query.startsWith('?') ? query.slice(1) : query);
  return {
    game: game.toLowerCase(),
    kind: kind.toLowerCase(),
    buyinAtoms: intOrNull(params.get('buyin')),
    seats: intOrNull(params.get('seats')),
    sid: params.get('sid') ?? '',
    raw,
  };
};

// splitGamingInvites splits a chat segment around game invites so the caller can
// interleave prose with invite chips, mirroring splitLnInvoices. Returns a
// single text part when there is no invite.
export const splitGamingInvites = (text: string): GamingInvitePart[] => {
  const re = new RegExp(GAMING_INVITE_RE);
  const parts: GamingInvitePart[] = [];
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) parts.push({ kind: 'text', text: text.slice(last, m.index) });
    parts.push({ kind: 'invite', invite: parseInvite(m[1], m[2], m[3] ?? '', m[0]) });
    last = m.index + m[0].length;
  }
  if (parts.length === 0) return [{ kind: 'text', text }];
  if (last < text.length) parts.push({ kind: 'text', text: text.slice(last) });
  return parts;
};
