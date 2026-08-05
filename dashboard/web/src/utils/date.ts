// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// toYMD formats a date as YYYY-MM-DD (ISO 8601 date) in local time. It accepts
// a Date or a millisecond epoch so call sites can pass either.
export const toYMD = (d: Date | number): string => {
  const dt = typeof d === 'number' ? new Date(d) : d;
  const y = dt.getFullYear();
  const m = String(dt.getMonth() + 1).padStart(2, '0');
  const day = String(dt.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
};

// toYMDTime formats a date as YYYY-MM-DD HH:MM (24-hour) in local time.
export const toYMDTime = (d: Date | number): string => {
  const dt = typeof d === 'number' ? new Date(d) : d;
  const hh = String(dt.getHours()).padStart(2, '0');
  const mm = String(dt.getMinutes()).padStart(2, '0');
  return `${toYMD(dt)} ${hh}:${mm}`;
};

// timeAgo renders a compact relative time ("just now", "5m ago", "3h ago"),
// falling back to the date for anything older than a week.
export const timeAgo = (d: Date | number | string): string => {
  const t =
    typeof d === 'string' ? new Date(d).getTime() : typeof d === 'number' ? d : d.getTime();
  const s = Math.floor((Date.now() - t) / 1000);
  if (s < 60) return 'just now';
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  if (s < 7 * 86400) return `${Math.floor(s / 86400)}d ago`;
  return toYMD(t);
};
