// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// What crosses to and from a game's frame. Deliberately short: no address, no
// amount and no key is ever accepted from the frame.

export const PROTOCOL_VERSION = 1 as const;

/** Messages the panel sends into the frame. */
export type ToFrame =
  | {
      type: 'pulse.init';
      v: 1;
      /** Where the frame's API calls go. Relative to this origin. */
      apiBase: string;
      token: string;
      expiresAt?: string;
      /** Selects among tables the player is already at; grants nothing. */
      tableId?: string | null;
      /** Injected by the host, never accepted from the frame. */
      payoutAddress?: string;
    }
  | { type: 'pulse.token'; v: 1; token: string; expiresAt?: string }
  | { type: 'pulse.close'; v: 1 };

/** Messages the frame may send back. All display-only. */
export type FromFrame =
  | { type: 'game.ready'; v: 1 }
  | { type: 'game.close'; v: 1 }
  | { type: 'game.title'; v: 1; text: string }
  | { type: 'game.error'; v: 1; message: string };

const fromFrameTypes = ['game.ready', 'game.close', 'game.title', 'game.error'] as const;

/** parseFromFrame validates a message from the frame against a literal list. */
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
