// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// The trust boundary, written down as a message list.
//
// A game's interface runs in an iframe sandboxed with `allow-scripts` and
// deliberately *without* `allow-same-origin`, so it has an opaque origin: it
// cannot read this document, it sends no cookies, and neither side can name the
// other's origin in a postMessage call. That last point is why a MessageChannel
// is used rather than window.postMessage - see GamePanel.
//
// Keep this list short. Every entry is something the untrusted side gets to
// influence or to learn, and two absences are deliberate:
//
//   - No address and no amount is ever accepted *from* the frame. The payout
//     address is derived by the host from the bound wallet account and pushed
//     in. A page that could name where winnings go could redirect them.
//   - No signature and no key, in either direction. The plugin signs; the page
//     is a view and an input device.

export const PROTOCOL_VERSION = 1 as const;

/** What the panel sends into the frame. */
export type ToFrame =
  | {
      type: 'pulse.init';
      v: 1;
      /** Where the frame's API calls go. Relative to this origin. */
      apiBase: string;
      token: string;
      expiresAt?: string;
      /** The table the panel was opened for, if it was opened from an
       *  invitation. It selects among tables the player is already at; it
       *  grants nothing. */
      tableId?: string | null;
      /** Where this host will have the game pay the user. Injected, never
       *  accepted. */
      payoutAddress?: string;
    }
  | { type: 'pulse.token'; v: 1; token: string; expiresAt?: string }
  | { type: 'pulse.close'; v: 1 };

/** What the frame may send back. All of it is display-only. */
export type FromFrame =
  | { type: 'game.ready'; v: 1 }
  | { type: 'game.close'; v: 1 }
  | { type: 'game.title'; v: 1; text: string }
  | { type: 'game.error'; v: 1; message: string };

const fromFrameTypes = ['game.ready', 'game.close', 'game.title', 'game.error'] as const;

/** parseFromFrame validates a message from the untrusted side.
 *
 *  Everything that arrives is checked against a literal list and truncated,
 *  because everything that arrives was written by a game. */
export function parseFromFrame(data: unknown): FromFrame | null {
  if (!data || typeof data !== 'object') return null;
  const msg = data as Record<string, unknown>;
  if (msg.v !== PROTOCOL_VERSION) return null;
  if (typeof msg.type !== 'string') return null;
  if (!(fromFrameTypes as readonly string[]).includes(msg.type)) return null;

  switch (msg.type) {
    case 'game.ready':
      return { type: 'game.ready', v: 1 };
    case 'game.close':
      return { type: 'game.close', v: 1 };
    case 'game.title':
      return { type: 'game.title', v: 1, text: text(msg.text) };
    case 'game.error':
      return { type: 'game.error', v: 1, message: text(msg.message) };
    default:
      return null;
  }
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.slice(0, 200) : '';
}
