// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

interface VoteResultsBarProps {
  yes: number;
  no: number;
  abstain?: number;
  eligibleTickets: number;
  quorumMin: number;
}

// VoteResultsBar matches proposals.decred.org's design: a full-width
// bar split yes/no/abstain by share of total cast votes, with a
// right-aligned text indicator showing whether quorum was reached and
// the total cast / eligible ticket count.
export const VoteResultsBar = ({
  yes,
  no,
  abstain = 0,
  eligibleTickets,
  quorumMin,
}: VoteResultsBarProps) => {
  const totalCast = yes + no + abstain;
  if (totalCast === 0) {
    return (
      <div className="text-xs text-muted-foreground">
        No votes cast yet
        {eligibleTickets > 0 && (
          <span>
            {' '}&middot; <span className="font-mono">0</span> /{' '}
            <span className="font-mono">{eligibleTickets.toLocaleString()}</span> votes
          </span>
        )}
      </div>
    );
  }

  const pct = (n: number) => (n / totalCast) * 100;
  const yesPct = pct(yes);
  const noPct = pct(no);
  const abstainPct = pct(abstain);
  const quorumMet = quorumMin <= 0 || totalCast >= quorumMin;

  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-sm bg-success" />
          yes <span className="font-mono text-foreground">{yes.toLocaleString()}</span>
          <span>({yesPct.toFixed(1)}%)</span>
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-sm bg-warning" />
          no <span className="font-mono text-foreground">{no.toLocaleString()}</span>
          <span>({noPct.toFixed(1)}%)</span>
        </span>
        {abstain > 0 && (
          <span className="inline-flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-sm bg-muted-foreground/40" />
            abstain <span className="font-mono text-foreground">{abstain.toLocaleString()}</span>
            <span>({abstainPct.toFixed(1)}%)</span>
          </span>
        )}
        <span className="ml-auto">
          <span className={quorumMet ? 'text-foreground/70' : 'text-destructive'}>
            quorum {quorumMet ? 'reached' : 'not met'}
          </span>{' '}
          <span className="font-mono text-foreground">{totalCast.toLocaleString()}</span>
          {eligibleTickets > 0 && (
            <span className="text-muted-foreground">
              {' '}/ {eligibleTickets.toLocaleString()} votes
            </span>
          )}
        </span>
      </div>
      <div className="relative h-2 rounded bg-muted/20 overflow-hidden">
        <div className="absolute top-0 left-0 h-full bg-success" style={{ width: `${yesPct}%` }} />
        <div
          className="absolute top-0 h-full bg-warning"
          style={{ left: `${yesPct}%`, width: `${noPct}%` }}
        />
        {abstainPct > 0 && (
          <div
            className="absolute top-0 h-full bg-muted-foreground/40"
            style={{ left: `${yesPct + noPct}%`, width: `${abstainPct}%` }}
          />
        )}
      </div>
    </div>
  );
};
