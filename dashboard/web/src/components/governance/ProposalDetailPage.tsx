// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import {
  AlertCircle,
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Loader2,
  MessageSquare,
  RefreshCw,
  Vote,
} from 'lucide-react';
import {
  ProposalComment,
  ProposalDetail,
  getProposalDetail,
  refreshProposalDetail,
} from '../../services/api';
import { VoteModal } from './VoteModal';
import { useWalletReady } from '../../hooks/useWalletReady';
import { VoteResultsBar } from './VoteResultsBar';

const POLITEIA_BASE = 'https://proposals.decred.org/record';

// formatDuration renders a positive second count as "7h 12m" / "12m" / "<1m".
const formatDuration = (secs: number) => {
  const s = Math.max(0, secs);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return '<1m';
};

// formatAgo renders elapsed seconds as "just now" / "12m ago" / "7h 12m ago".
const formatAgo = (secs: number) => {
  const s = Math.max(0, secs);
  if (s < 60) return 'just now';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h}h ${m}m ago`;
  return `${m}m ago`;
};

type CommentNode = ProposalComment & { replies: CommentNode[] };

// buildCommentTree turns Politeia's flat comment list into a reply tree.
// A parentID of 0 marks a top-level comment; any other value points at the
// commentID it replies to. Comments arrive sorted oldest-first from the
// backend, so children keep that order. Orphans (missing parent) are treated
// as top-level so nothing is silently dropped.
const buildCommentTree = (comments: ProposalComment[]): CommentNode[] => {
  const byId = new Map<number, CommentNode>();
  comments.forEach((c) => byId.set(c.commentID, { ...c, replies: [] }));
  const roots: CommentNode[] = [];
  byId.forEach((node) => {
    const parent =
      node.parentID !== 0 && node.parentID !== node.commentID
        ? byId.get(node.parentID)
        : undefined;
    if (parent) parent.replies.push(node);
    else roots.push(node);
  });
  return roots;
};

// CommentThread renders a comment as a bordered card and nests its replies
// under a connector rail. Indentation is capped past depth 4 so deep chains
// don't overflow narrow screens; cards still nest logically.
const CommentThread = ({ node, now, depth }: { node: CommentNode; now: number; depth: number }) => {
  const score = node.upvotes - node.downvotes;
  const netColor = score > 0 ? 'text-success' : score < 0 ? 'text-destructive' : 'text-muted-foreground';
  return (
    <div>
      <div className="rounded-lg border border-border/50 bg-muted/10 px-4 py-3">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 text-xs min-w-0">
            <span className="font-semibold text-foreground truncate">{node.username || 'unknown'}</span>
            {node.createdAt > 0 && (
              <>
                <span className="text-muted-foreground/50">&middot;</span>
                <span className="text-muted-foreground shrink-0">{formatAgo(now - node.createdAt)}</span>
              </>
            )}
          </div>
          <div className="flex items-center gap-2 text-xs shrink-0">
            <span className="inline-flex items-center gap-0.5 text-success">
              <ChevronUp className="h-3.5 w-3.5" />
              {node.upvotes}
            </span>
            <span className="inline-flex items-center gap-0.5 text-destructive">
              <ChevronDown className="h-3.5 w-3.5" />
              {node.downvotes}
            </span>
            <span className={`px-1.5 py-0.5 rounded bg-muted/30 font-medium ${netColor}`}>
              {score > 0 ? `+${score}` : score}
            </span>
          </div>
        </div>
        <div className="mt-2 pt-2 border-t border-border/30">
          {node.deleted ? (
            <p className="text-sm italic text-muted-foreground">
              Comment removed{node.reason ? `: ${node.reason}` : '.'}
            </p>
          ) : node.commentHtml ? (
            <div
              className="proposal-body text-sm text-foreground/80 leading-relaxed"
              dangerouslySetInnerHTML={{ __html: node.commentHtml }}
            />
          ) : (
            <pre className="whitespace-pre-wrap text-sm text-foreground/80 font-sans">
              {node.comment}
            </pre>
          )}
        </div>
      </div>
      {node.replies.length > 0 && (
        <div className={`mt-3 space-y-3 ${depth < 4 ? 'pl-4 border-l border-border/40' : ''}`}>
          {node.replies.map((child) => (
            <CommentThread key={child.commentID} node={child} now={now} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  );
};

export const ProposalDetailPage = () => {
  const { token } = useParams<{ token: string }>();
  const [detail, setDetail] = useState<ProposalDetail | null>(null);
  const [fetchedAt, setFetchedAt] = useState(0);
  const [refreshAvailableAt, setRefreshAvailableAt] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [voteModalOpen, setVoteModalOpen] = useState(false);
  // Watch-only wallets cannot sign a consensus vote, so hide the cast-vote CTA.
  const { isWatchOnly } = useWalletReady();
  // now (unix seconds) drives the live countdown / "updated X ago" display.
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));

  const load = async () => {
    if (!token) return;
    setError(null);
    try {
      const r = await getProposalDetail(token);
      setDetail(r.detail);
      setFetchedAt(r.fetchedAt);
      setRefreshAvailableAt(r.refreshAvailableAt);
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

  // Tick the clock once a minute so the countdown and age stay current.
  useEffect(() => {
    const id = setInterval(() => setNow(Math.floor(Date.now() / 1000)), 60 * 1000);
    return () => clearInterval(id);
  }, []);

  const handleRefresh = async () => {
    if (!token) return;
    setRefreshing(true);
    setRefreshError(null);
    try {
      const r = await refreshProposalDetail(token);
      setDetail(r.detail);
      setFetchedAt(r.fetchedAt);
      setRefreshAvailableAt(r.refreshAvailableAt);
    } catch (err: any) {
      const status = err?.response?.status;
      const data = err?.response?.data;
      if (status === 429 && data && typeof data === 'object') {
        // Cooling down: re-sync from the server so the countdown matches.
        if (data.detail) setDetail(data.detail);
        if (data.fetchedAt) setFetchedAt(data.fetchedAt);
        if (data.refreshAvailableAt) setRefreshAvailableAt(data.refreshAvailableAt);
      } else {
        setRefreshError(typeof data === 'string' ? data : err?.message || 'Refresh failed');
      }
    } finally {
      setRefreshing(false);
      setNow(Math.floor(Date.now() / 1000));
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

  const isVoting = detail.status === 'voting';
  const cooldownRemaining = refreshAvailableAt - now;
  const canRefresh = !refreshing && cooldownRemaining <= 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Link
          to="/wallet/governance/proposals"
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to proposals
        </Link>
        <div className="ml-auto flex items-center gap-3">
          {fetchedAt > 0 && (
            <span className="text-xs text-muted-foreground">updated {formatAgo(now - fetchedAt)}</span>
          )}
          <button
            type="button"
            onClick={handleRefresh}
            disabled={!canRefresh}
            title={
              canRefresh
                ? 'Refresh this proposal from Politeia'
                : `Next refresh available in ${formatDuration(cooldownRemaining)}`
            }
            className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm border border-border/50 bg-background hover:bg-muted/10 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
            {refreshing
              ? 'Refreshing...'
              : cooldownRemaining > 0
                ? `Refresh in ${formatDuration(cooldownRemaining)}`
                : 'Refresh'}
          </button>
        </div>
      </div>

      {refreshError && (
        <div className="text-xs text-destructive flex items-center gap-1">
          <AlertCircle className="h-3 w-3" />
          {refreshError}
        </div>
      )}

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

      {isVoting && !isWatchOnly && (
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-3">
          <h3 className="font-semibold">Cast your vote</h3>
          {detail.currentChoice && (
            <div className="flex items-center gap-2 text-sm text-success">
              <CheckCircle2 className="h-4 w-4" />
              You voted "{detail.currentChoice}".
            </div>
          )}
          <button
            type="button"
            onClick={() => setVoteModalOpen(true)}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all"
          >
            <Vote className="h-4 w-4" />
            {detail.currentChoice ? 'View vote' : 'Vote'}
          </button>
          <p className="text-xs text-muted-foreground">
            Opening this checks how many of your tickets are eligible (it fetches the ticket
            snapshot from Politeia), so it runs only when you click.
          </p>
        </div>
      )}

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-2">
        <h3 className="font-semibold flex items-center gap-2">
          <MessageSquare className="h-4 w-4" />
          Discussion
          {detail.comments && detail.comments.length > 0 && (
            <span className="text-xs font-normal text-muted-foreground">
              ({detail.comments.length})
            </span>
          )}
        </h3>
        {detail.comments && detail.comments.length > 0 ? (
          <div className="space-y-3 pt-1">
            {buildCommentTree(detail.comments).map((node) => (
              <CommentThread key={node.commentID} node={node} now={now} depth={0} />
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No comments on this proposal yet.</p>
        )}
      </div>

      {token && (
        <VoteModal
          isOpen={voteModalOpen}
          token={token}
          onClose={() => setVoteModalOpen(false)}
          onVoted={load}
        />
      )}
    </div>
  );
};
