// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// AVERAGE_BLOCK_TIME is Decred's target block interval in minutes.
const AVERAGE_BLOCK_TIME = 5;

// blocksToDuration renders a block count as an approximate human duration, at
// the target rate. A block takes as long as it takes, so the answer is always a
// rough one, coarsened to a single unit: "~7 days", "~3 hours", "~5 minutes".
export const blocksToDuration = (blocks: number): string => {
  const minutes = Math.max(0, Math.round(blocks)) * AVERAGE_BLOCK_TIME;
  const days = Math.floor(minutes / 1440);
  if (days > 0) return `~${days} ${days === 1 ? 'day' : 'days'}`;
  const hours = Math.floor(minutes / 60);
  if (hours > 0) return `~${hours} ${hours === 1 ? 'hour' : 'hours'}`;
  return `~${minutes} ${minutes === 1 ? 'minute' : 'minutes'}`;
};
