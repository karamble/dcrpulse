// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { getTransaction, TSpendVotingInfo } from '../../services/explorerApi';
import { TimeAgo } from '../explorer/TimeAgo';

interface TSpendApprovalCardProps {
  yesVotes: number;
  noVotes: number;
  requiredApprovalPct?: number;
  votingInfo?: TSpendVotingInfo;
  txHash?: string;
  defaultExpanded?: boolean;
}

// Collapsible tspend approval view shared by the treasury card and the
// explorer transaction page: a compact yes/no split bar, expanding to the
// full voting details. Callers without votingInfo pass txHash so details
// are fetched on first expand.
export const TSpendApprovalCard = ({
  yesVotes,
  noVotes,
  requiredApprovalPct = 60,
  votingInfo,
  txHash,
  defaultExpanded = false,
}: TSpendApprovalCardProps) => {
  const navigate = useNavigate();
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [fetched, setFetched] = useState<TSpendVotingInfo | null>(null);
  const [fetchState, setFetchState] = useState<'idle' | 'loading' | 'done'>('idle');

  const info = votingInfo ?? fetched ?? undefined;
  const required = info?.requiredApprovalPct ?? requiredApprovalPct;

  const cast = yesVotes + noVotes;
  const approval = cast > 0 ? (yesVotes / cast) * 100 : 0;
  const yesPct = cast > 0 ? (yesVotes / cast) * 100 : 0;
  const noPct = cast > 0 ? (noVotes / cast) * 100 : 0;
  const passing = approval >= required;

  // Refetch on every expand so treasury rows retry after a failed fetch and
  // pick up fresh counts alongside the dashboard's 60s poll.
  const toggle = () => {
    const next = !expanded;
    setExpanded(next);
    if (next && !votingInfo && txHash && fetchState !== 'loading') {
      if (!fetched) setFetchState('loading');
      getTransaction(txHash)
        .then((tx) => {
          if (tx.votingInfo) setFetched(tx.votingInfo);
          setFetchState('done');
        })
        .catch(() => setFetchState('done'));
    }
  };

  return (
    <div className="space-y-3">
      <button
        onClick={toggle}
        aria-expanded={expanded}
        className="w-full text-left space-y-2 rounded-lg hover:bg-muted/10 transition-colors"
      >
        <div className="flex items-center justify-between">
          <span className={`text-sm font-semibold ${
            cast > 0 ? (passing ? 'text-success' : 'text-destructive') : 'text-muted-foreground'
          }`}>
            {cast > 0 ? `${approval.toFixed(1)}% yes` : 'no votes yet'}
          </span>
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          )}
        </div>
        <div className="h-2 w-full rounded-full bg-muted/20 overflow-hidden flex">
          <div className="h-full bg-success/70" style={{ width: `${yesPct}%` }} />
          <div className="h-full bg-destructive/40" style={{ width: `${noPct}%` }} />
        </div>
        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>Yes {yesVotes.toLocaleString()}</span>
          <span>needs {required.toFixed(0)}% to pass</span>
          <span>No {noVotes.toLocaleString()}</span>
        </div>
      </button>

      {expanded && (
        info ? (
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <span className={`px-3 py-1 rounded-full text-sm font-medium ${
                info.votingComplete ? 'bg-primary/10 text-primary' : 'bg-warning/10 text-warning'
              }`}>
                {info.votingComplete ? 'Voting Complete' : 'Ongoing Vote'}
              </span>
              {info.votingComplete && info.votesCast > 0 && (
                <span className={`px-3 py-1 rounded-full text-sm font-medium ${
                  info.approved ? 'bg-success/10 text-success' : 'bg-destructive/10 text-destructive'
                }`}>
                  {info.approved ? 'Approved' : 'Rejected'}
                </span>
              )}
            </div>

            {info.votesCast > 0 && (
              <div className="p-3 rounded-lg bg-background/50">
                <p className="text-sm">
                  <span className={info.quorumAchieved ? 'text-success' : 'text-warning'}>
                    {info.quorumAchieved ? 'Quorum achieved' : 'Quorum not yet reached'}
                  </span>
                  {' '}({info.votesCast.toLocaleString()} of {info.quorumRequired.toLocaleString()} needed votes)
                </p>
              </div>
            )}

            {info.votesCast > 0 && (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">Yes Votes</p>
                  <p className="text-lg font-semibold text-success">
                    {info.yesVotes.toLocaleString()} votes ({((info.yesVotes / info.votesCast) * 100).toFixed(1)}%)
                  </p>
                </div>

                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">No Votes</p>
                  <p className="text-lg font-semibold text-destructive">
                    {info.noVotes.toLocaleString()} votes ({((info.noVotes / info.votesCast) * 100).toFixed(1)}%)
                  </p>
                </div>

                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">Eligible Votes</p>
                  <p className="text-lg font-semibold">
                    {info.eligibleVotes.toLocaleString()}
                  </p>
                </div>

                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">Votes Cast</p>
                  <p className="text-lg font-semibold">
                    {info.votesCast.toLocaleString()} ({info.turnoutRate.toFixed(1)}% turnout)
                  </p>
                </div>

                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">Voting Started</p>
                  <p className="text-sm font-semibold">
                    <TimeAgo timestamp={info.votingStartTime} showFull />
                  </p>
                </div>

                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">
                    {info.votingComplete ? 'Voting Ended' : 'Voting Ends'}
                  </p>
                  <p className="text-sm font-semibold">
                    {info.votingEndEstimated && '~'}
                    <TimeAgo timestamp={info.votingEndTime} showFull />
                  </p>
                </div>

                <div className="p-4 rounded-lg bg-background/50 md:col-span-2">
                  <p className="text-sm text-muted-foreground mb-2">Voting Period</p>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => navigate(`/explorer/block/${info.votingStartBlock}`)}
                      className="text-sm font-semibold hover:text-primary transition-colors"
                    >
                      {info.votingStartBlock.toLocaleString()}
                    </button>
                    <span className="text-muted-foreground">-</span>
                    <button
                      onClick={() => navigate(`/explorer/block/${info.votingEndBlock}`)}
                      className="text-sm font-semibold hover:text-primary transition-colors"
                    >
                      {info.votingEndBlock.toLocaleString()}
                    </button>
                  </div>
                </div>
              </div>
            )}
          </div>
        ) : fetchState === 'loading' ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : (
          <p className="text-sm text-muted-foreground">No voting details available.</p>
        )
      )}
    </div>
  );
};
