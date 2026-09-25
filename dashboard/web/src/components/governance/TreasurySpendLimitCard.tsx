// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { Gauge } from 'lucide-react';
import { getTreasurySpendLimit, TreasurySpendLimit } from '../../services/treasuryApi';
import { useVisiblePoll } from '../../hooks/useVisiblePoll';
import { toDcr } from '../../utils/amounts';

const dcr = (atoms: number) =>
  toDcr(atoms).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

// What dcrd's DCP-0013 expenditure rule lets the treasury spend at the next
// TVI block. Hidden when the rule is not in force.
export const TreasurySpendLimitCard = () => {
  const [limit, setLimit] = useState<TreasurySpendLimit | null>(null);

  useVisiblePoll(() => {
    getTreasurySpendLimit().then(setLimit).catch(() => {});
  }, 300000);

  if (!limit?.active) return null;

  const floorApplies = limit.maxSpendableAtoms === limit.floorAtoms;
  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
      <div className="flex items-center gap-2 mb-4">
        <Gauge className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold">Treasury Spend Limit</h3>
      </div>
      <div className="text-3xl font-bold">{dcr(limit.allowedAtoms)} DCR</div>
      <p className="text-xs text-muted-foreground mt-1">
        {limit.atTvi
          ? `dcrd's limit for the treasury vote block ${limit.nextTvi.toLocaleString()}.`
          : `What dcrd would allow if the next block were a treasury vote block. Next one: block ${limit.nextTvi.toLocaleString()}.`}
      </p>
      <dl className="grid grid-cols-1 sm:grid-cols-3 gap-4 mt-4 text-sm">
        <div>
          <dt className="text-muted-foreground">Window limit</dt>
          <dd className="font-semibold">{dcr(limit.maxSpendableAtoms)} DCR</dd>
          <dd className="text-xs text-muted-foreground">
            {floorApplies ? 'The fixed floor' : '4% of the balance plus the window’s spends'}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Spent in the window</dt>
          <dd className="font-semibold">{dcr(limit.spentInWindowAtoms)} DCR</dd>
          <dd className="text-xs text-muted-foreground">
            Last {limit.policyWindowBlocks.toLocaleString()} blocks, fees included
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Balance counted</dt>
          <dd className="font-semibold">{dcr(limit.balanceAtoms)} DCR</dd>
          <dd className="text-xs text-muted-foreground">Including funds maturing in the next block</dd>
        </div>
      </dl>
    </div>
  );
};
