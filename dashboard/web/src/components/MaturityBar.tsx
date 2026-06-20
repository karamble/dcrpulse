// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { calculateMaturityProgress } from '../services/ticketService';

interface MaturityBarProps {
  // Blocks remaining until the stake output matures; 0 or undefined once matured.
  blocksRemaining?: number;
  // Phrase shown after the block count while maturing, e.g. "until spendable"
  // (a voted ticket's returned funds) or "to live" (a ticket purchase).
  pendingSuffix?: string;
  // Caption shown once matured.
  doneLabel?: string;
  className?: string;
}

// Thin 256-block maturity progress bar with a caption, shared by the staking
// history and the wallet transaction lists so they render identically.
export const MaturityBar = ({
  blocksRemaining,
  pendingSuffix = 'until spendable',
  doneLabel = 'Funds Spendable',
  className = '',
}: MaturityBarProps) => {
  const remaining = blocksRemaining ?? 0;
  const maturing = remaining > 0;
  const caption = maturing
    ? `${remaining} ${remaining === 1 ? 'block' : 'blocks'} ${pendingSuffix}`
    : doneLabel;

  return (
    <div className={className}>
      <div className="h-1.5 bg-gray-300 dark:bg-gray-700 rounded-full overflow-hidden">
        <div
          className={`h-full transition-all duration-300 ${maturing ? 'bg-warning' : 'bg-success'}`}
          style={{ width: `${calculateMaturityProgress(blocksRemaining)}%` }}
        />
      </div>
      <div className="text-xs text-muted-foreground mt-1">{caption}</div>
    </div>
  );
};
