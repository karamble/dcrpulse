// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// shortHash abbreviates a long hex string for compact display.
export function shortHash(h?: string, head = 10, tail = 6): string {
  if (!h) return '';
  if (h.length <= head + tail + 1) return h;
  return `${h.slice(0, head)}…${h.slice(-tail)}`;
}

// fromUnix converts unix seconds to a JS Date (or null when 0/absent).
export function fromUnix(sec?: number): Date | null {
  if (!sec || sec <= 0) return null;
  return new Date(sec * 1000);
}
