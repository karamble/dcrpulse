// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { CheckCircle2, Vote } from 'lucide-react';
import { TSpend } from '../../services/treasuryApi';
import { TSpendApprovalCard } from './TSpendApprovalCard';

const dcr = (v: number) =>
  v.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

const shortAddr = (a: string) => (a.length > 16 ? `${a.slice(0, 8)}…${a.slice(-6)}` : a);

// Treasury spends require 60% yes votes (TreasuryVoteRequiredMultiplier 3/5).
const REQUIRED_APPROVAL = 60;

const VoteRow = ({ t }: { t: TSpend }) => (
  <div className="p-4 rounded-lg bg-muted/10 border border-border/50 space-y-2">
    <div className="flex items-center justify-between gap-3">
      <div className="min-w-0">
        <div className="font-semibold">{dcr(t.amount)} DCR</div>
        <div className="text-xs text-muted-foreground font-mono truncate">
          {shortAddr(t.payee || t.txHash)}
        </div>
      </div>
      <div className="text-xs text-muted-foreground text-right shrink-0">
        ends in {t.blocksRemaining.toLocaleString()} blocks
      </div>
    </div>

    <TSpendApprovalCard
      yesVotes={t.yesVotes}
      noVotes={t.noVotes}
      requiredApprovalPct={REQUIRED_APPROVAL}
      txHash={t.txHash}
    />
  </div>
);

interface ActiveTreasuryVotesProps {
  active: TSpend[];
  loaded: boolean;
}

// Fed by GovernanceDashboard's shared treasury poll: four cards used to fetch
// the same endpoint on their own timers.
export const ActiveTreasuryVotes = ({ active, loaded }: ActiveTreasuryVotesProps) => {

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
      <div className="flex items-center gap-2 mb-4">
        <Vote className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold">Active Treasury Votes</h3>
        {active.length > 0 && (
          <span className="text-xs text-muted-foreground">({active.length} in voting)</span>
        )}
      </div>

      {active.length === 0 ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <CheckCircle2 className="h-4 w-4 text-success" />
          {loaded ? 'No treasury votes in progress.' : 'Loading…'}
        </div>
      ) : (
        <div className="space-y-3">
          {active.map((t) => (
            <VoteRow key={t.txHash} t={t} />
          ))}
        </div>
      )}
    </div>
  );
};
