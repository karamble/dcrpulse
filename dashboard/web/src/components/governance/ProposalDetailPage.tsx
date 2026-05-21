// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { AlertCircle, ArrowLeft, CheckCircle2, ExternalLink, Loader2 } from 'lucide-react';
import {
  CastVoteResult,
  ProposalDetail,
  castPoliteiaVote,
  getProposalDetail,
} from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';
import { VoteResultsBar } from './VoteResultsBar';

const POLITEIA_BASE = 'https://proposals.decred.org/record';

export const ProposalDetailPage = () => {
  const { token } = useParams<{ token: string }>();
  const [detail, setDetail] = useState<ProposalDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string>('');
  const [modalOpen, setModalOpen] = useState(false);
  const [voteResult, setVoteResult] = useState<CastVoteResult | null>(null);

  const load = async () => {
    if (!token) return;
    setError(null);
    try {
      const d = await getProposalDetail(token);
      setDetail(d);
      if (d.currentChoice) setSelected(d.currentChoice);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to load proposal');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  const handleSubmit = async (passphrase: string) => {
    if (!token || !selected) return;
    try {
      const r = await castPoliteiaVote(token, selected, passphrase);
      setVoteResult(r);
      setModalOpen(false);
      await load();
    } catch (err: any) {
      const body = err?.response?.data;
      throw new Error(typeof body === 'string' ? body : err?.message || 'Vote cast failed');
    }
  };

  if (loading && !detail) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground flex items-center gap-2">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading proposal...
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-center gap-2">
        <AlertCircle className="h-4 w-4" />
        {error}
      </div>
    );
  }

  if (!detail) return null;

  const isVoting = detail.status === 'voting' && detail.voteOptions.length > 0;
  const canCast = isVoting && detail.eligibleTickets > 0 && selected !== '';

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Link
          to="/wallet/governance/proposals"
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to proposals
        </Link>
      </div>

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-xl font-semibold">{detail.name || detail.token}</h2>
            <p className="text-xs text-muted-foreground mt-1">
              by {detail.username || 'unknown'} &middot; token{' '}
              <span className="font-mono">{detail.token}</span>
            </p>
          </div>
          <a
            href={`${POLITEIA_BASE}/${detail.token}`}
            target="_blank"
            rel="noopener noreferrer nofollow"
            className="inline-flex items-center gap-1 text-sm text-primary hover:underline shrink-0"
          >
            View on Politeia
            <ExternalLink className="h-3 w-3" />
          </a>
        </div>

        <div className="flex flex-wrap gap-3 text-xs">
          <span className="px-2 py-0.5 rounded bg-muted/15 border border-border/50">
            status: {detail.voteStatus}
          </span>
          {detail.endBlock > 0 && (
            <span className="text-muted-foreground">
              ends at block {detail.endBlock.toLocaleString()}
            </span>
          )}
          {detail.blocksLeft > 0 && (
            <span className="text-muted-foreground">
              {detail.blocksLeft.toLocaleString()} blocks left
            </span>
          )}
          {detail.eligibleTickets > 0 && (
            <span className="text-muted-foreground">
              eligible tickets: {detail.eligibleTickets.toLocaleString()}
            </span>
          )}
        </div>

        {detail.eligibleTickets > 0 && (
          <div className="pt-2 border-t border-border/30">
            <VoteResultsBar
              yes={detail.voteCounts.yes ?? 0}
              no={detail.voteCounts.no ?? 0}
              abstain={detail.voteCounts.abstain ?? 0}
              eligibleTickets={detail.eligibleTickets}
              quorumMin={detail.quorumMin}
            />
          </div>
        )}
      </div>

      {(detail.descriptionHtml || detail.description) && (
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-2">
          <h3 className="font-semibold">Description</h3>
          {detail.descriptionHtml ? (
            <div
              className="proposal-body text-sm text-foreground/80 leading-relaxed"
              dangerouslySetInnerHTML={{ __html: detail.descriptionHtml }}
            />
          ) : (
            <pre className="whitespace-pre-wrap text-sm text-foreground/80 font-sans">
              {detail.description}
            </pre>
          )}
          <p className="text-xs text-muted-foreground pt-2 border-t border-border/30">
            Embedded images are not loaded. Open the proposal on Politeia for visuals or the
            original formatting.
          </p>
        </div>
      )}

      {isVoting && (
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
          <h3 className="font-semibold">Cast your vote</h3>
          {detail.currentChoice && (
            <div className="flex items-center gap-2 text-sm text-success">
              <CheckCircle2 className="h-4 w-4" />
              You previously voted "{detail.currentChoice}".
            </div>
          )}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
            {detail.voteOptions.map((o) => (
              <button
                key={o.id}
                type="button"
                onClick={() => setSelected(o.id)}
                className={`p-3 rounded-lg border text-sm font-medium transition-colors ${
                  selected === o.id
                    ? 'border-primary/40 bg-primary/10 text-foreground'
                    : 'border-border/50 bg-muted/10 hover:bg-muted/20'
                }`}
              >
                {o.id}
              </button>
            ))}
          </div>
          {voteResult && (
            <div className="text-sm">
              <div className="text-success">
                Cast {voteResult.cast} ticket vote{voteResult.cast === 1 ? '' : 's'}.
              </div>
              {voteResult.skipped > 0 && (
                <div className="text-warning">Skipped {voteResult.skipped}.</div>
              )}
              {voteResult.errors && voteResult.errors.length > 0 && (
                <ul className="mt-2 list-disc pl-5 text-xs text-destructive">
                  {voteResult.errors.slice(0, 5).map((e, i) => (
                    <li key={i}>{e}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
          <button
            type="button"
            onClick={() => {
              setVoteResult(null);
              setModalOpen(true);
            }}
            disabled={!canCast}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Cast vote
          </button>
          {detail.eligibleTickets === 0 && (
            <p className="text-xs text-muted-foreground">
              This proposal has no eligible tickets, so no vote can be cast yet.
            </p>
          )}
        </div>
      )}

      <PassphraseModal
        isOpen={modalOpen}
        title="Cast Politeia Vote"
        description={`Sign the vote message for every eligible ticket the wallet owns and submit the ballot to proposals.decred.org. This can take a moment if you have many tickets.`}
        submitLabel="Sign & cast"
        busyLabel="Casting..."
        onSubmit={handleSubmit}
        onClose={() => setModalOpen(false)}
      />
    </div>
  );
};
