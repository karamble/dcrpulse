// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Binary units, since the scale below divides by 1024.
const UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];

// formatBytes renders a byte count, one decimal below 10 of a unit.
export const formatBytes = (n: number): string => {
  if (!Number.isFinite(n) || n <= 0) return '0 B';
  let v = n;
  let i = 0;
  while (v >= 1024 && i < UNITS.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${UNITS[i]}`;
};
