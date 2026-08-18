// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Shared reconnecting WebSocket subscription. The daemons behind these streams
// restart, wallets get switched and laptops sleep, so a stream that stops at
// the first close leaves its panel frozen with no sign anything is wrong.
// Delays match the backoff the Go side uses for its own reconnects
// (internal/rpc/brclientd_ws.go).

export const MIN_DELAY_MS = 1000;
const MAX_DELAY_MS = 30000;

// nextDelay doubles the wait, clamping after the doubling rather than before:
// clamping first lets 20s double to 40s and overshoot the ceiling.
export function nextDelay(current: number): number {
  return Math.min(current * 2, MAX_DELAY_MS);
}

export interface SubscribeOpts {
  // onOpen fires on the first connection and on every reconnection. Streams
  // whose server replays nothing on connect should refetch here, or events
  // that happened during the gap are lost.
  onOpen?: () => void;
  onError?: (err: Error) => void;
  // onDrop fires on every disconnection, including ones we are about to
  // retry. Deliberately not called onClose: the callers that already have an
  // onClose use it to mean the stream is finished for good.
  onDrop?: () => void;
}

// subscribeJSON opens a same-origin WebSocket to path and delivers each JSON
// frame to onMessage, reconnecting with capped exponential backoff until the
// returned cleanup is called. Frames carrying an error string are treated as a
// failed connection rather than a message: the daemons write one and hang up
// when the service behind the stream is unavailable.
export function subscribeJSON<T>(
  path: string,
  onMessage: (msg: T) => void,
  opts: SubscribeOpts = {},
): () => void {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const url = `${proto}//${window.location.host}${path}`;

  let ws: WebSocket | null = null;
  let cancelled = false;
  let delay = MIN_DELAY_MS;
  let retryTimer: ReturnType<typeof setTimeout> | null = null;

  // Detach before closing so a frame already queued on a dying socket cannot
  // land after its replacement has opened.
  const detach = (sock: WebSocket) => {
    sock.onopen = null;
    sock.onmessage = null;
    sock.onerror = null;
    sock.onclose = null;
  };

  const connect = () => {
    if (cancelled) return;
    const sock = new WebSocket(url);
    ws = sock;

    sock.onopen = () => {
      delay = MIN_DELAY_MS;
      opts.onOpen?.();
    };

    sock.onmessage = (event) => {
      let frame: unknown;
      try {
        frame = JSON.parse(event.data);
      } catch {
        return; // ignore non-JSON frames
      }
      const err = (frame as { error?: unknown })?.error;
      if (typeof err === 'string') {
        opts.onError?.(new Error(err));
        return;
      }
      onMessage(frame as T);
    };

    // Funnel errors through close so there is exactly one reconnect path.
    sock.onerror = () => {
      opts.onError?.(new Error(`${path} connection error`));
      try {
        sock.close();
      } catch {
        /* already closing */
      }
    };

    sock.onclose = () => {
      opts.onDrop?.();
      if (cancelled) return;
      retryTimer = setTimeout(connect, delay);
      delay = nextDelay(delay);
    };
  };

  connect();

  return () => {
    cancelled = true;
    if (retryTimer) {
      clearTimeout(retryTimer);
      retryTimer = null;
    }
    if (ws) {
      detach(ws);
      ws.close();
      ws = null;
    }
  };
}
