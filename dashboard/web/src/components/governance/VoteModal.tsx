// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, CheckCircle2, Clock, Loader2, Vote, X } from 'lucide-react';
import {
  CastVoteResult,
  VoteEligibility,
  castPoliteiaVote,
  getVoteEligibility,
  startVoteTrickle,
} from '../../services/api';

interface VoteModalProps {
  isOpen: boolean;
  token: string;
  onClose: () => void;
  // onVoted is called after a successful cast so the parent can refresh the
  // proposal (updated tallies + "you voted X").
  onVoted: () => void | Promise<void>;
}

// VoteModal does all the heavy, on-demand voting work: on open it asks the
// backend to compute the wallet's eligibility (owned-ticket count, options,
// already-voted state) - which is where the eligible-ticket snapshot is
// fetched - then lets the user pick a choice and broadcast the ballot.
export const VoteModal = ({ isOpen, token, onClose, onVoted }: VoteModalProps) => {
  const [loading, setLoading] = useState(true);
  const [eligibility, setEligibility] = useState<VoteEligibility | null>(null);
  const [eligError, setEligError] = useState<string | null>(null);

  const [selected, setSelected] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const [casting, setCasting] = useState(false);
  const [castError, setCastError] = useState<string | null>(null);
  const [castResult, setCastResult] = useState<CastVoteResult | null>(null);
  // politeiavoter mode: trickle the votes out over a window instead of one batch.
  const [trickle, setTrickle] = useState(false);
  const [durationMinutes, setDurationMinutes] = useState('360');
  const [bunches, setBunches] = useState('1');

  useEffect(() => {
    if (!isOpen) {
      setLoading(true);
      setEligibility(null);
      setEligError(null);
      setSelected('');
      setPassphrase('');
      setCasting(false);
      setCastError(null);
      setCastResult(null);
      return;
    }
    let cancelled = false;
    (async () => {
      setLoading(true);
      setEligError(null);
      try {
        const e = await getVoteEligibility(token);
        if (cancelled) return;
        setEligibility(e);
        if (e.currentChoice) setSelected(e.currentChoice);
      } catch (err: any) {
        if (cancelled) return;
        const body = err?.response?.data;
        setEligError(typeof body === 'string' ? body : err?.message || 'Failed to check eligibility');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [isOpen, token]);

  if (!isOpen) return null;

  const handleClose = () => {
    if (casting) return;
    onClose();
  };

  const handleCast = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selected || !passphrase || casting) return;
    setCasting(true);
    setCastError(null);
    try {
      if (trickle) {
        const minutes = Math.max(1, parseFloat(durationMinutes) || 0);
        await startVoteTrickle({
          token,
          voteOption: selected,
          durationSeconds: Math.round(minutes * 60),
          bunches: Math.max(1, parseInt(bunches, 10) || 1),
          passphrase,
        });
        setPassphrase('');
        await onVoted();
        // The worker now runs in the background; the status card surfaces it.
        onClose();
        return;
      }
      const r = await castPoliteiaVote(token, selected, passphrase);
      setCastResult(r);
      setPassphrase('');
      await onVoted();
    } catch (err: any) {
      const body = err?.response?.data;
      setCastError(
        typeof body === 'string'
          ? body
          : err?.message || (trickle ? 'Failed to start trickle' : 'Vote cast failed')
      );
    } finally {
      setCasting(false);
    }
  };

  const canVote =
    !!eligibility && !eligibility.alreadyVoted && eligibility.ownedEligibleCount > 0 && !castResult;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in">
      <div className="w-full max-w-md mx-4 rounded-xl bg-card border border-border/50 shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <Vote className="h-5 w-5 text-primary" />
            Cast Politeia Vote
          </h3>
          <button
            onClick={handleClose}
            disabled={casting}
            className="text-muted-foreground hover:text-foreground disabled:opacity-50"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-6 space-y-4">
          {loading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Checking your eligibility (fetching the ticket snapshot)...
            </div>
          ) : eligError ? (
            <div className="flex items-start gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>{eligError}</span>
            </div>
          ) : eligibility?.alreadyVoted ? (
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-sm text-success">
                <CheckCircle2 className="h-4 w-4" />
                {`You voted "${eligibility.currentChoice}"${
                  eligibility.votedTicketCount > 0
                    ? ` with ${eligibility.votedTicketCount} ticket${eligibility.votedTicketCount === 1 ? '' : 's'}`
                    : ''
                }.`}
              </div>
              <p className="text-xs text-muted-foreground">
                Each ticket can vote once, so this proposal can no longer be voted from this
                wallet.
              </p>
            </div>
          ) : eligibility && eligibility.ownedEligibleCount === 0 ? (
            <p className="text-sm text-muted-foreground">
              None of your tickets are eligible for this vote
              {eligibility.eligibleTickets > 0
                ? ` (${eligibility.eligibleTickets.toLocaleString()} tickets are eligible in total).`
                : '.'}
            </p>
          ) : eligibility ? (
            <form onSubmit={handleCast} className="space-y-4">
              <p className="text-sm text-muted-foreground">
                You can vote with{' '}
                <span className="font-semibold text-foreground">
                  {eligibility.ownedEligibleCount.toLocaleString()}
                </span>{' '}
                ticket{eligibility.ownedEligibleCount === 1 ? '' : 's'}.
              </p>

              <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
                {eligibility.voteOptions.map((o) => (
                  <button
                    key={o.id}
                    type="button"
                    onClick={() => setSelected(o.id)}
                    disabled={!canVote || casting}
                    className={`p-3 rounded-lg border text-sm font-medium transition-colors disabled:opacity-50 ${
                      selected === o.id
                        ? 'border-primary/40 bg-primary/10 text-foreground'
                        : 'border-border/50 bg-muted/10 hover:bg-muted/20'
                    }`}
                  >
                    {o.id}
                  </button>
                ))}
              </div>

              <div className="rounded-lg border border-border/50 bg-muted/10 p-3 space-y-3">
                <label className="flex items-start justify-between gap-3 cursor-pointer">
                  <span className="text-sm">
                    <span className="font-medium flex items-center gap-1.5">
                      <Clock className="h-4 w-4 text-primary" /> politeiavoter mode
                    </span>
                    <span className="block text-xs text-muted-foreground mt-0.5">
                      Trickle the votes out at random times over a window instead of all at once.
                    </span>
                  </span>
                  <input
                    type="checkbox"
                    checked={trickle}
                    onChange={(e) => setTrickle(e.target.checked)}
                    disabled={!canVote || casting}
                    className="h-4 w-4 mt-0.5 accent-primary shrink-0"
                  />
                </label>
                {trickle && (
                  <div className="grid grid-cols-2 gap-2 pt-1">
                    <div>
                      <label
                        className="block text-xs text-muted-foreground mb-1"
                        htmlFor="trickle-minutes"
                      >
                        Spread over (minutes)
                      </label>
                      <input
                        id="trickle-minutes"
                        type="number"
                        min="1"
                        step="any"
                        value={durationMinutes}
                        onChange={(e) => setDurationMinutes(e.target.value)}
                        disabled={casting}
                        className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm focus:outline-none focus:border-primary disabled:opacity-50"
                      />
                    </div>
                    <div>
                      <label
                        className="block text-xs text-muted-foreground mb-1"
                        htmlFor="trickle-bunches"
                      >
                        Bunches
                      </label>
                      <input
                        id="trickle-bunches"
                        type="number"
                        min="1"
                        step="1"
                        value={bunches}
                        onChange={(e) => setBunches(e.target.value)}
                        disabled={casting}
                        className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm focus:outline-none focus:border-primary disabled:opacity-50"
                      />
                    </div>
                    <p className="col-span-2 text-[11px] text-muted-foreground">
                      Votes are signed now, then submitted over the window. A dashboard restart ends
                      the run; it is safe to re-launch (already-cast tickets are skipped).
                    </p>
                  </div>
                )}
              </div>

              <div>
                <label className="block text-sm text-muted-foreground mb-1" htmlFor="vote-passphrase">
                  Wallet passphrase
                </label>
                <input
                  id="vote-passphrase"
                  type="password"
                  autoComplete="current-password"
                  value={passphrase}
                  onChange={(e) => setPassphrase(e.target.value)}
                  disabled={casting}
                  className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
                />
              </div>

              {castError && (
                <div className="flex items-start gap-2 text-sm text-destructive">
                  <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                  <span>{castError}</span>
                </div>
              )}

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={handleClose}
                  disabled={casting}
                  className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm disabled:opacity-50"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={!selected || !passphrase || casting}
                  className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {casting
                    ? trickle
                      ? 'Starting...'
                      : 'Casting...'
                    : trickle
                      ? 'Sign & start trickle'
                      : 'Sign & cast'}
                </button>
              </div>
            </form>
          ) : null}

          {castResult && (
            <div className="space-y-2 border-t border-border/30 pt-4">
              <div className="flex items-center gap-2 text-sm text-success">
                <CheckCircle2 className="h-4 w-4" />
                Cast {castResult.cast} ticket vote{castResult.cast === 1 ? '' : 's'}.
              </div>
              {castResult.skipped > 0 && (
                <div className="text-sm text-warning">Skipped {castResult.skipped}.</div>
              )}
              {castResult.errors && castResult.errors.length > 0 && (
                <ul className="list-disc pl-5 text-xs text-destructive">
                  {castResult.errors.slice(0, 5).map((e, i) => (
                    <li key={i}>{e}</li>
                  ))}
                </ul>
              )}
              <div className="flex justify-end pt-2">
                <button
                  type="button"
                  onClick={onClose}
                  className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm"
                >
                  Done
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
