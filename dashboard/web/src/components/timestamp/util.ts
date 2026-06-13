// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// fmtBytes renders a byte count in human units.
export function fmtBytes(n?: number): string {
  if (!n || n <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v.toFixed(i > 0 && v < 10 ? 1 : 0)} ${units[i]}`;
}

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
