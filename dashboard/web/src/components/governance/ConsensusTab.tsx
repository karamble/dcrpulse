// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';
import {
  Agenda,
  AgendaVotes,
  ConsensusVoteInfo,
  getAgendaVotes,
  getAgendas,
  setAgendaChoice,
} from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';
import { useVisiblePoll } from '../../hooks/useVisiblePoll';
import { apiError } from '../../utils/apiError';

const statusColor = (status: string) => {
  switch (status) {
    case 'active':
    case 'started':
      return 'bg-info/15 text-info border border-info/30';
    case 'lockedin':
    case 'locked_in':
      return 'bg-success/15 text-success border border-success/30';
    case 'failed':
      return 'bg-destructive/15 text-destructive border border-destructive/30';
    case 'defined':
      return 'bg-muted/15 text-muted-foreground border border-border/50';
    default:
      return 'bg-muted/15 text-muted-foreground border border-border/50';
  }
};

const pct = (fraction: number) => `${(fraction * 100).toFixed(1)}%`;
const pctOf = (percent: number) => `${percent.toFixed(1)}%`;

// AgendaCard renders one agenda and loads its tally the first time it scrolls
// into view. dcrd reports counts only while an agenda is being voted on, so a
// settled one has to be reconstructed from the blocks it was voted in, and that
// is worth deferring until the card is actually looked at.
const AgendaCard = ({
  agenda,
  quorum,
  onChoose,
}: {
  agenda: Agenda;
  quorum: number;
  onChoose: (agendaID: string, choiceID: string) => void;
}) => {
  const [votes, setVotes] = useState<AgendaVotes | null>(null);
  const [loadingVotes, setLoadingVotes] = useState(false);
  const ref = useRef<HTMLDivElement | null>(null);
  const requested = useRef(false);

  useEffect(() => {
    const el = ref.current;
    if (!el || requested.current) return;

    const load = () => {
      if (requested.current) return;
      requested.current = true;
      setLoadingVotes(true);
      getAgendaVotes(agenda.id)
        .then(setVotes)
        // An agenda whose window cannot be derived, or that expired without a
        // vote, simply keeps the card it already has.
        .catch(() => {})
        .finally(() => setLoadingVotes(false));
    };

    const obs = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          load();
          obs.disconnect();
        }
      },
      { rootMargin: '200px' },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [agenda.id]);

  // A live vote carries its counts inline from getvoteinfo; a settled one uses
  // the reconstructed tally. Either way the same markup renders it.
  const live = agenda.status === 'started';
  // A preference can only be changed before the vote settles; a settled
  // agenda keeps its card as a read-only record.
  const votable = agenda.status === 'defined' || agenda.status === 'started';
  const tally = votes;
  const showTally = live || tally !== null;

  // dcrd tallies the current voting interval live; shape it like the
  // reconstructed history so one block renders both.
  const liveTally = useMemo((): AgendaVotes | null => {
    if (!live) return null;
    let yes = 0;
    let no = 0;
    let abstain = 0;
    for (const c of agenda.choices) {
      if (c.isAbstain) abstain += c.count;
      else if (c.isNo) no += c.count;
      else yes += c.count;
    }
    const totalVotes = yes + no + abstain;
    const nonAbstain = yes + no;
    return {
      agendaId: agenda.id,
      voteVersion: agenda.voteVersion,
      voteStartHeight: 0,
      voteEndHeight: 0,
      yes,
      no,
      abstain,
      totalVotes,
      unmatched: 0,
      approvalRating: nonAbstain > 0 ? (100 * yes) / nonAbstain : 0,
      yesShare: totalVotes > 0 ? (100 * yes) / totalVotes : 0,
      quorum,
      quorumMet: nonAbstain >= quorum,
      complete: false,
    };
  }, [live, agenda, quorum]);
  const shown = live ? liveTally : tally;

  const choiceCount = (choiceID: string): { count: number; share: number } | null => {
    if (live) {
      const c = agenda.choices.find((x) => x.id === choiceID);
      return c ? { count: c.count, share: c.progress * 100 } : null;
    }
    if (!tally || tally.totalVotes === 0) return null;
    const count =
      choiceID === 'yes' ? tally.yes : choiceID === 'no' ? tally.no : tally.abstain;
    return { count, share: (100 * count) / tally.totalVotes };
  };

  return (
    <div ref={ref} className="p-6 rounded-xl bg-gradient-card border border-border/50 space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 className="font-semibold">{agenda.id}</h3>
          <p className="text-sm text-muted-foreground mt-1">{agenda.description}</p>
        </div>
        <span className={`px-2 py-0.5 rounded text-xs font-medium ${statusColor(agenda.status)}`}>
          {agenda.status}
        </span>
      </div>

      {agenda.status !== 'started' && agenda.since > 0 && (
        <p className="text-xs text-muted-foreground">
          {agenda.status === 'active' ? 'In force since block ' : 'Settled at block '}
          <span className="text-foreground font-medium">{agenda.since.toLocaleString()}</span>
          {agenda.voteStartHeight > 0 && (
            <>
              {', voted over '}
              <span className="text-foreground font-medium">
                {agenda.voteStartHeight.toLocaleString()}
              </span>
              {' to '}
              <span className="text-foreground font-medium">
                {agenda.voteEndHeight.toLocaleString()}
              </span>
            </>
          )}
        </p>
      )}

      {loadingVotes && !live && (
        <p className="text-xs text-muted-foreground flex items-center gap-2">
          <Loader2 className="h-3 w-3 animate-spin" />
          Counting votes...
        </p>
      )}

      {live && (
        <div className="space-y-2">
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>Quorum progress</span>
            <span className="text-foreground font-medium">{pct(agenda.quorumProgress)}</span>
          </div>
          <div className="h-2 w-full rounded-full bg-muted/20 overflow-hidden">
            <div
              className="h-full bg-info"
              style={{ width: `${Math.min(agenda.quorumProgress * 100, 100)}%` }}
            />
          </div>
        </div>
      )}

      {shown && shown.totalVotes > 0 && (
        <div className="space-y-2">
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              Approval{' '}
              <span
                className={
                  shown.approvalRating >= 75 ? 'text-success font-medium' : 'text-destructive font-medium'
                }
              >
                {pctOf(shown.approvalRating)}
              </span>
              {' of yes and no votes'}
            </span>
            <span>
              {shown.quorumMet ? 'quorum met' : 'quorum not met'} (
              {(shown.yes + shown.no).toLocaleString()} of {shown.quorum.toLocaleString()})
            </span>
          </div>
          {/* yes / no / abstain split across every vote cast */}
          <div className="h-2 w-full rounded-full bg-muted/20 overflow-hidden flex">
            <div
              className="h-full bg-success"
              style={{ width: `${(100 * shown.yes) / shown.totalVotes}%` }}
            />
            <div
              className="h-full bg-destructive"
              style={{ width: `${(100 * shown.no) / shown.totalVotes}%` }}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {shown.totalVotes.toLocaleString()} votes cast, {pctOf(shown.yesShare)} yes overall
            {!shown.complete && ' (still counting)'}
          </p>
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
        {agenda.choices.map((c) => {
          const isCurrent = agenda.currentChoice === c.id;
          const tallied = showTally ? choiceCount(c.id) : null;
          return (
            <button
              key={c.id}
              type="button"
              disabled={!votable}
              onClick={() => onChoose(agenda.id, c.id)}
              className={`p-3 rounded-lg border text-left text-sm transition-colors ${
                isCurrent
                  ? 'border-primary/40 bg-primary/10 text-foreground'
                  : votable
                    ? 'border-border/50 bg-muted/10 hover:bg-muted/20'
                    : 'border-border/50 bg-muted/10'
              }`}
            >
              <div className="font-medium">{c.id}</div>
              <div className="text-xs text-muted-foreground mt-0.5">{c.description}</div>
              {tallied && (
                <div className="text-xs text-foreground mt-1 font-medium">
                  {tallied.count.toLocaleString()} votes ({pctOf(tallied.share)})
                </div>
              )}
              {isCurrent && <div className="text-xs text-primary mt-1">current choice</div>}
            </button>
          );
        })}
      </div>
    </div>
  );
};

export const ConsensusTab = () => {
  const [voteInfo, setVoteInfo] = useState<ConsensusVoteInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState<{ agendaID: string; choiceID: string } | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);

  const agendas = voteInfo?.agendas ?? [];

  // Newest vote version first, so the current rule change leads the page.
  const byVersion = useMemo(() => {
    const groups = new Map<number, Agenda[]>();
    for (const a of agendas) {
      const list = groups.get(a.voteVersion);
      if (list) list.push(a);
      else groups.set(a.voteVersion, [a]);
    }
    return [...groups.entries()].sort((x, y) => y[0] - x[0]);
  }, [agendas]);

  const load = async () => {
    setError(null);
    try {
      setVoteInfo(await getAgendas());
    } catch (err: any) {
      setError(apiError(err, 'Failed to load agendas'));
    } finally {
      setLoading(false);
    }
  };

  useVisiblePoll(load, 30000);

  const handleSubmit = async (passphrase: string) => {
    if (!pending) return;
    try {
      await setAgendaChoice(pending.agendaID, pending.choiceID, passphrase);
      setFeedback(`Saved choice "${pending.choiceID}" for ${pending.agendaID}.`);
      setPending(null);
      await load();
    } catch (err: any) {
      throw new Error(apiError(err, 'Failed to set choice'));
    }
  };

  if (loading && agendas.length === 0) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground flex items-center gap-2">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading agendas...
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

  if (agendas.length === 0) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
        No consensus agendas reported by this node.
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {feedback && (
        <div className="flex items-center gap-2 text-sm text-success">
          <CheckCircle2 className="h-4 w-4" />
          {feedback}
        </div>
      )}
      {voteInfo && (
        <div className="px-4 py-3 rounded-xl bg-muted/10 border border-border/50 text-xs text-muted-foreground flex flex-wrap gap-x-6 gap-y-1">
          <span>
            Current vote version{' '}
            <span className="text-foreground font-medium">{voteInfo.currentVoteVersion}</span>
          </span>
          {/* The interval and its vote count belong to the tip, not to any
              settled agenda, so they are only shown while a vote is running. */}
          {voteInfo.voteInProgress ? (
            <>
              <span>
                Voting interval{' '}
                <span className="text-foreground font-medium">
                  {voteInfo.windowStartHeight.toLocaleString()}
                </span>
                {' to '}
                <span className="text-foreground font-medium">
                  {voteInfo.windowEndHeight.toLocaleString()}
                </span>
              </span>
              <span>
                {voteInfo.totalVotes.toLocaleString()} votes cast, quorum{' '}
                {voteInfo.quorum.toLocaleString()}
              </span>
            </>
          ) : (
            <span>No rule change is being voted on right now.</span>
          )}
        </div>
      )}

      {byVersion.map(([version, group]) => (
        <div key={version} className="space-y-3">
          <h2 className="text-xs uppercase tracking-wide text-muted-foreground pt-2">
            Vote version {version}
          </h2>
          {group.map((a) => (
            <AgendaCard
              key={a.id}
              agenda={a}
              quorum={voteInfo?.quorum ?? 0}
              onChoose={(agendaID, choiceID) => {
                setFeedback(null);
                setPending({ agendaID, choiceID });
              }}
            />
          ))}
        </div>
      ))}

      <PassphraseModal
        isOpen={pending !== null}
        title="Confirm Agenda Vote Choice"
        description={
          pending
            ? `Set "${pending.choiceID}" for agenda ${pending.agendaID}. The wallet is unlocked briefly to write the choice.`
            : ''
        }
        submitLabel="Save"
        busyLabel="Saving..."
        onSubmit={handleSubmit}
        onClose={() => setPending(null)}
      />
    </div>
  );
};
