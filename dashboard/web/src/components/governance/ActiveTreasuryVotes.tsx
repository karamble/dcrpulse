// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { CheckCircle2, Vote } from 'lucide-react';
import { getTreasuryInfo, TSpend } from '../../services/treasuryApi';

const dcr = (v: number) =>
  v.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

const shortAddr = (a: string) => (a.length > 16 ? `${a.slice(0, 8)}…${a.slice(-6)}` : a);

// Treasury spends require 60% yes votes (TreasuryVoteRequiredMultiplier 3/5).
const REQUIRED_APPROVAL = 60;

const VoteRow = ({ t }: { t: TSpend }) => {
  const cast = t.yesVotes + t.noVotes;
  const approval = cast > 0 ? (t.yesVotes / cast) * 100 : 0;
  const yesPct = cast > 0 ? (t.yesVotes / cast) * 100 : 0;
  const passing = approval >= REQUIRED_APPROVAL;

  return (
    <div className="p-4 rounded-lg bg-muted/10 border border-border/50 space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="font-semibold">{dcr(t.amount)} DCR</div>
          <div className="text-xs text-muted-foreground font-mono truncate">
            {shortAddr(t.payee || t.txHash)}
          </div>
        </div>
        <div className="text-right shrink-0">
          <div className="text-xs text-muted-foreground">
            ends in {t.blocksRemaining.toLocaleString()} blocks
          </div>
          <div className={`text-sm font-semibold ${passing ? 'text-success' : 'text-warning'}`}>
            {cast > 0 ? `${approval.toFixed(1)}% yes` : 'no votes yet'}
          </div>
        </div>
      </div>

      {/* yes (green) / no (red) split bar */}
      <div className="h-2 w-full rounded-full bg-destructive/40 overflow-hidden flex">
        <div className="h-full bg-success/70" style={{ width: `${yesPct}%` }} />
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>Yes {t.yesVotes.toLocaleString()}</span>
        <span>needs {REQUIRED_APPROVAL}% to pass</span>
        <span>No {t.noVotes.toLocaleString()}</span>
      </div>
    </div>
  );
};

export const ActiveTreasuryVotes = () => {
  const [active, setActive] = useState<TSpend[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    const load = () => {
      getTreasuryInfo()
        .then((i) => setActive(i.activeTSpends ?? []))
        .catch(() => {})
        .finally(() => setLoaded(true));
    };
    load();
    const id = window.setInterval(load, 60000);
    return () => window.clearInterval(id);
  }, []);

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
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
