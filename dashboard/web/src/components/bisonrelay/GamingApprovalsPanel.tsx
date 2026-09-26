// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { AlertCircle, AlertTriangle, Loader2, ShieldCheck } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { GamePolicy, GamingPayout, GamingSpend } from '../../services/gamingApi';
import { formatAtomsTrimmed } from '../../utils/amounts';
import { refreshGamingSpends, useGamingSpends } from '../../hooks/useGamingSpends';
import { GamingSpendRequestCard, SpendOutcomes, useSpendDecisions } from './GamingSpendApprovals';
import { GamingPayoutCard, payoutWaiting, sortSettledPayouts, useGamingPayouts, usePayoutActions } from './GamingPayoutApprovals';

const fmtDcr = (atoms: number): string => formatAtomsTrimmed(atoms);

type WaitingItem =
  | { kind: 'buyin'; id: string; deadline: number; spend: GamingSpend }
  | { kind: 'payout'; id: string; deadline: number; payout: GamingPayout };

// waitingOrder puts what the operator has to answer first: soonest deadline
// first on the server clock, anything without one after, ties by kind and id
// so the order holds between polls.
export function waitingOrder(pending: GamingSpend[], payouts: GamingPayout[], now: number): WaitingItem[] {
  const items: WaitingItem[] = [
    ...pending.map((s): WaitingItem => ({ kind: 'buyin', id: s.id, deadline: s.expiresAt, spend: s })),
    ...payouts
      .filter((p) => payoutWaiting(p, now))
      .map((p): WaitingItem => ({
        kind: 'payout', id: p.id, payout: p,
        deadline: p.state === 'awaiting_approval' && now < p.expiresAt ? p.expiresAt : Number.POSITIVE_INFINITY,
      })),
  ];
  return items.sort((a, b) =>
    a.deadline !== b.deadline ? (a.deadline < b.deadline ? -1 : 1) : a.kind !== b.kind ? (a.kind < b.kind ? -1 : 1) : a.id < b.id ? -1 : a.id > b.id ? 1 : 0,
  );
}

// GamingApprovalsPanel is the Approvals tab: everything waiting for the
// operator on top, then buy-ins, then match payouts.
export const GamingApprovalsPanel = ({
  policies,
  bridgeEnabled,
  gameCount,
}: {
  policies?: Record<string, GamePolicy>;
  bridgeEnabled?: boolean;
  gameCount?: number;
}) => {
  const feed = useGamingSpends();
  const loaded = feed.lastOkAt > 0;
  const stale = feed.failures >= 2;
  const pollErr = feed.error;
  const lastOkAt = feed.lastOkAt;
  const pending = feed.pending;
  const publishing = feed.publishing;
  const decidedTotal = feed.decidedTotal;
  const refresh = refreshGamingSpends;
  const d = useSpendDecisions();
  const { watchOnly } = d;
  const { rows: payouts, loadError, load } = useGamingPayouts();
  const actions = usePayoutActions(load);

  // A second hand of its own, so a deadline moves between five second polls.
  // Only while something is actually counting down.
  const [, setTick] = useState(0);
  const counting = pending.length > 0 || payouts.some((p) => p.state === 'awaiting_approval');
  useEffect(() => {
    if (!counting) return;
    const t = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, [counting]);
  const now = Math.floor(Date.now() / 1000) + feed.offset;

  // The panel is never absent. Somebody who has only ever seen it empty should
  // still know what answering one involves before they are asked to do it
  // against a two minute clock.
  const resting = () => {
    if (bridgeEnabled === false) {
      return 'The gaming bridge is off, so no game can ask for anything.';
    }
    if (gameCount === 0) {
      return 'No games are registered, so nothing can ask for anything.';
    }
    return null;
  };

  // Quote a wait only when every registered game agrees on one; naming a
  // number that is true of one game and not another is worse than saying none.
  const waits = [...new Set(Object.values(policies ?? {}).map((p) => p.approvalTimeoutSecs))];

  const waiting = waitingOrder(pending, payouts, now);
  const settled = sortSettledPayouts(payouts.filter((p) => !payoutWaiting(p, now)));

  return (
    <div className="space-y-6">
      {waiting.length > 0 && (
        <section className="space-y-2" aria-live="polite">
          <div className="flex items-baseline justify-between gap-2">
            <h3 className="font-medium text-sm">Waiting for you</h3>
            <span className="text-xs text-warning">
              {waiting.length === 1 ? '1 waiting' : `${waiting.length} waiting`}
            </span>
          </div>
          {waiting.map((w) =>
            w.kind === 'buyin' ? (
              <GamingSpendRequestCard
                key={`buyin-${w.id}`}
                s={w.spend}
                now={now}
                policy={policies?.[w.spend.game]}
                used={feed.usedToday[w.spend.game] ?? 0}
                stale={stale}
                d={d}
              />
            ) : (
              <GamingPayoutCard
                key={`payout-${w.id}`}
                payout={w.payout}
                now={now}
                busy={actions.busy}
                onReview={actions.review}
                onSend={actions.send}
              />
            ),
          )}
        </section>
      )}

      <section className="space-y-2">
        <h3 className="font-medium text-sm">Buy-in approvals</h3>

      {stale && (
        <div className="p-2 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning space-y-1">
          <div className="flex items-start gap-2">
            <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
            <span className="break-words">
              Not reading approvals. The last answer was{' '}
              {lastOkAt ? `${Math.round((Date.now() - lastOkAt) / 1000)} seconds ago` : 'never'}, so
              a game may be waiting on you right now and this list would not show it. {pollErr}
            </span>
          </div>
          <button
            type="button"
            onClick={() => void refresh()}
            className="text-xs underline hover:no-underline"
          >
            Try now
          </button>
        </div>
      )}

      {watchOnly && (
        <div className="p-2 rounded-lg bg-destructive/10 border border-destructive/30 text-xs text-destructive break-words">
          This wallet has no private keys loaded, so it cannot sign. No passphrase will help, and
          nothing here can be approved until a spending wallet is open.
        </div>
      )}

      {pending.length === 0 && (
        <div className="p-3 rounded-lg bg-muted/10 border border-border/50 text-xs text-muted-foreground space-y-1">
          {resting() ? (
            <span className="block">{resting()}</span>
          ) : (
            <>
              <span className="block font-medium text-foreground">Nothing is waiting on you.</span>
              <span className="block">
                When a game asks to spend, it appears here and you have as long as that game's
                approval wait to answer{waits.length === 1 ? `, which is ${waits[0]} seconds` : ''}.
              </span>
              <span className="block">
                A request states the amount, the account it comes out of, and the address in full.
                It also carries a line of the game's own words, which nothing checks.
              </span>
              <span className="block">
                Approving takes your wallet passphrase every time. This dashboard never stores it,
                and there is no setting that pays automatically.
              </span>
            </>
          )}
        </div>
      )}

      {publishing.map((s) => (
        <div
          key={s.id}
          className="p-3 rounded-lg bg-muted/10 border border-border/50 text-xs space-y-1"
        >
          <div className="flex items-baseline justify-between gap-3">
            <span className="truncate">
              {s.game} - {fmtDcr(s.amountAtoms)} DCR
            </span>
            <span className="shrink-0 text-muted-foreground">being paid</span>
          </div>
          <span className="block text-muted-foreground break-words">
            Approved and on its way to the network. Nothing to answer here; the outcome lands in
            History on its own.
          </span>
        </div>
      ))}

        <SpendOutcomes d={d} />

      {decidedTotal > 0 && (
        <p className="text-xs text-muted-foreground">
          {decidedTotal === 1 ? '1 earlier request' : `${decidedTotal} earlier requests`}, with
          their transaction ids and any refusals, are under History.
        </p>
      )}

      {!loaded && !pollErr && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          Reading what games have asked for.
        </div>
      )}
      {!loaded && pollErr && (
        <div className="flex items-start gap-2 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span className="break-words">
            The list of requests could not be read: {pollErr}. A game may be waiting and this would
            not show it.
          </span>
        </div>
      )}
      </section>

      <section className="space-y-3">
        <h3 className="flex items-center gap-2 font-semibold"><ShieldCheck className="h-5 w-5 text-emerald-400" /> Match payouts</h3>
        {(actions.error || loadError) && <p role="alert" className="text-sm text-red-400">{actions.error || loadError}</p>}
        {payouts.length === 0 && <p className="text-sm text-gray-400">Payout proposals will appear here for your approval.</p>}
        {settled.map((p) => (
          <GamingPayoutCard key={p.id} payout={p} now={now} busy={actions.busy} onReview={actions.review} onSend={actions.send} />
        ))}
      </section>
      {actions.dialog}
    </div>
  );
};
