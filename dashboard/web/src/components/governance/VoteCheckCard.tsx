// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { AlertCircle, CheckCircle2, Download, Loader2, ShieldCheck } from 'lucide-react';
import {
  ProposalVoteCheck,
  ProposalVoteCheckProgress,
  downloadProposalVotes,
  getProposalVoteCheck,
  startProposalVoteCheck,
} from '../../services/api';
import { useVisiblePoll } from '../../hooks/useVisiblePoll';
import { apiError } from '../../utils/apiError';
import { downloadBlob } from '../../utils/files';

interface Props {
  token: string;
  check?: ProposalVoteCheck;
  // tally is Politeia's reported count per option.
  tally: Record<string, number>;
  onChecked: () => void;
}

// VoteCheckCard checks every ballot of an archived proposal on this
// dashboard and offers the ballots for Politeia's own politeiaverify.
export const VoteCheckCard = ({ token, check, tally, onChecked }: Props) => {
  const [progress, setProgress] = useState<ProposalVoteCheckProgress | null>(null);
  const [error, setError] = useState<string | null>(null);
  const running = !!progress?.running;
  // wasRunning tells a check that just finished from one read at rest.
  const wasRunning = useRef(false);

  const poll = async () => {
    try {
      const p = await getProposalVoteCheck(token);
      if (wasRunning.current && !p.running && !p.error) onChecked();
      wasRunning.current = p.running;
      setProgress(p);
    } catch (err) {
      setError(apiError(err, 'Could not read the vote check'));
    }
  };

  useEffect(() => {
    poll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);
  useVisiblePoll(poll, 2000, { enabled: running, immediate: false });

  const onStart = async () => {
    setError(null);
    try {
      const p = await startProposalVoteCheck(token);
      wasRunning.current = p.running;
      setProgress(p);
    } catch (err) {
      setError(apiError(err, 'Could not start the vote check'));
    }
  };

  const onDownload = async () => {
    setError(null);
    try {
      const resp = await downloadProposalVotes(token);
      downloadBlob(resp.data, `${token}-votes.json`, 'application/json');
    } catch (err) {
      setError(apiError(err, 'Download failed'));
    }
  };

  const options = Array.from(new Set([...Object.keys(tally), ...Object.keys(check?.counts ?? {})]));

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50 space-y-3 text-sm">
      <h3 className="font-semibold flex items-center gap-2">
        <ShieldCheck className="h-4 w-4" />
        Votes
      </h3>
      <p className="text-muted-foreground">
        Checks every ballot&apos;s signature and Politeia&apos;s receipt, that its ticket was
        eligible as Politeia lists them and voted once, and that it was signed with the
        ticket&apos;s commitment address on this node. The downloaded ballots can also be checked
        with Politeia&apos;s politeiaverify.
      </p>

      {check && (
        <div className="space-y-2">
          <p className="flex items-center gap-2">
            {check.tallyMatches ? (
              <CheckCircle2 className="h-4 w-4 text-success" />
            ) : (
              <AlertCircle className="h-4 w-4 text-destructive" />
            )}
            {check.counted} of {check.ballots} ballots verified;{' '}
            {check.tallyMatches
              ? "the count matches Politeia's tally."
              : "the count differs from Politeia's tally."}
          </p>
          {!check.tallyMatches && (
            <ul className="pl-6 text-muted-foreground">
              {options.map((id) => (
                <li key={id}>
                  {id}: counted {check.counts[id] ?? 0}, Politeia {tally[id] ?? 0}
                </li>
              ))}
            </ul>
          )}
          {check.problems && check.problems.length > 0 && (
            <div className="space-y-1">
              <p className="text-destructive">{check.problems.length} ballots not counted:</p>
              <ul className="max-h-48 overflow-y-auto pl-6 list-disc text-muted-foreground">
                {check.problems.map((b, i) => (
                  <li key={`${b.ticket}-${i}`} className="break-all">
                    <span className="font-mono">{b.ticket}</span>: {b.problem}
                  </li>
                ))}
              </ul>
            </div>
          )}
          <p className="text-xs text-muted-foreground">
            Checked {new Date(check.checkedAt * 1000).toLocaleString()}.
          </p>
        </div>
      )}

      {running && (
        <p className="flex items-center gap-2 text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          {progress && progress.total > 0
            ? `Checking ballots, ${progress.done} of ${progress.total}...`
            : 'Fetching the ballots from Politeia...'}
        </p>
      )}
      {progress?.error && !running && (
        <p className="text-destructive">The check stopped: {progress.error}</p>
      )}
      {error && <p className="text-destructive">{error}</p>}

      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          onClick={onStart}
          disabled={running}
          className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg border border-border/50 bg-background hover:bg-muted/10 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          <ShieldCheck className="h-4 w-4" />
          {check ? 'Verify votes again' : 'Verify votes'}
        </button>
        {check && (
          <button
            type="button"
            onClick={onDownload}
            disabled={running}
            className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg border border-border/50 bg-background hover:bg-muted/10 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            <Download className="h-4 w-4" />
            Download votes
          </button>
        )}
      </div>
    </div>
  );
};
