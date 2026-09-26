// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { Check, Loader2 } from 'lucide-react';
import {
  GamePolicy,
  GamingSpend,
  decideGamingSpend,
  getGamingSpendHistory,
} from '../../services/gamingApi';
import { Pagination } from '../explorer/Pagination';
import { apiError } from '../../utils/apiError';
import { formatAtomsTrimmed, toDcr } from '../../utils/amounts';
import { refreshGamingSpends, useGamingSpends } from '../../hooks/useGamingSpends';

const fmtDcr = (atoms: number): string => formatAtomsTrimmed(atoms);
const fmtWhen = (unix: number): string =>
  unix ? new Date(unix * 1000).toLocaleString() : 'an unknown time';

// mmss renders a deadline the way somebody reads one under pressure.
const mmss = (secs: number): string => {
  const s = Math.max(0, Math.floor(secs));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
};

// A game's own words about what it wants the money for, made safe to put on a
// screen next to facts this console is asserting.
//
// The string is chosen by the game and the game is not trusted. Left alone it
// can imitate the console's own voice, fake a structured field, reorder the
// line around it with a bidi override, or run long enough to push the buttons
// out of view. None of that is exotic - it is the ordinary consequence of
// rendering somebody else's text in a place where a person is deciding whether
// to part with coin.
//
// So: strip the control and direction-changing characters, collapse the
// whitespace that would otherwise make it tall, and let the caller render the
// result in a quarantined block with no colour of its own.
export const sanitizeReason = (raw: string): string =>
  raw
    // C0 and C1 control characters.
    .replace(/[\u0000-\u001f\u007f-\u009f]/g, '')
    // Bidi overrides and isolates, which can reorder the line around this.
    .replace(/[\u202a-\u202e\u2066-\u2069]/g, '')
    // Zero-width characters, which can hide text or pad it invisibly.
    .replace(/[\u200b-\u200d\ufeff]/g, '')
    // Newlines and runs of spaces, which are how a short field becomes tall.
    .replace(/\s+/g, ' ')
    .trim();

// The wallet passphrase failing is not the same kind of event as the payment
// failing, and the difference is the whole of what a person needs to know: a
// signature that did not happen means nothing was sent and the request is
// still theirs to answer.
const isPassphraseRefusal = (text: string): boolean =>
  /invalid passphrase|incorrect passphrase|passphrase is incorrect|could not decrypt.*private key|wrong passphrase/i.test(
    text,
  );

const isWatchOnly = (text: string): boolean => /watch[- ]?only/i.test(text);

// useSpendDecisions is the state every buy-in row shares. armed is the one row
// whose passphrase field is open: one field for the whole panel, because two
// live Approve buttons sharing it is a wrong-row payment waiting to happen.
export function useSpendDecisions() {
  const [armed, setArmed] = useState<{ id: string; passphrase: string } | null>(null);
  const [deciding, setDeciding] = useState<string | null>(null);
  const [rowErr, setRowErr] = useState<Record<string, string>>({});
  const [outcome, setOutcome] = useState<Record<string, { txid?: string; failed?: string }>>({});
  const [watchOnly, setWatchOnly] = useState(false);
  const firstSeen = useRef<Record<string, number>>({});

  const decide = async (s: GamingSpend, approve: boolean, passphrase?: string) => {
    setDeciding(s.id);
    setRowErr((r) => ({ ...r, [s.id]: '' }));
    try {
      const answer = await decideGamingSpend(s.id, approve, passphrase);
      setArmed(null);
      if (approve) {
        setOutcome((o) => ({
          ...o,
          [s.id]: answer.txid ? { txid: answer.txid } : { failed: answer.error || '' },
        }));
      }
      await refreshGamingSpends();
    } catch (e) {
      const text = apiError(e, 'could not answer that request');
      if (isWatchOnly(text)) setWatchOnly(true);
      setRowErr((r) => ({ ...r, [s.id]: text }));
    } finally {
      setDeciding(null);
    }
  };

  return { armed, setArmed, deciding, rowErr, outcome, setOutcome, watchOnly, firstSeen, decide };
}

export type SpendDecisions = ReturnType<typeof useSpendDecisions>;

// GamingSpendRequestCard is one buy-in waiting for the operator's answer.
export function GamingSpendRequestCard({ s, now, policy: p, used, stale, d }: {
  s: GamingSpend;
  now: number;
  policy?: GamePolicy;
  used: number;
  stale: boolean;
  d: SpendDecisions;
}) {
  const { armed, setArmed, deciding, rowErr, watchOnly, firstSeen, decide } = d;
  // Stamped during render, not in an effect. An effect runs after the first
  // paint, so the row would already be on screen with a live Approve button
  // for the moment the guard exists to cover.
  if (!firstSeen.current[s.id]) firstSeen.current[s.id] = Date.now();
  const left = s.expiresAt - now;
  const total = Math.max(1, s.expiresAt - s.requestedAt);
  const frac = Math.max(0, Math.min(1, left / total));
  const lapsed = left <= 0;
  const fresh = Date.now() - (firstSeen.current[s.id] ?? 0) < 1500;
  const capAtoms = p?.perDayCapDcr ? Math.round(p.perDayCapDcr * 1e8) : 0;
  const reason = sanitizeReason(s.reason ?? '');
  const verified = !!s.depositId && !!s.tableId && s.fundingFeeAtoms > 0 && s.recoveryLockBlocks > 0;
  const depositLabel = {seatbond: 'Admission bond', stake: 'Game stake', tablebond: 'Table bond'}[s.depositKind];
  const err = rowErr[s.id];
  const isArmed = armed?.id === s.id;
  const busyRow = deciding === s.id;
  const tone = lapsed
    ? 'border-border/50'
    : frac <= 0.25
      ? 'border-destructive/40'
      : frac <= 0.5
        ? 'border-warning/40'
        : 'border-border/50';

  return (
    <div
      key={s.id}
      className={`rounded-lg border ${tone} bg-muted/10 p-3 space-y-2 ${
        lapsed || stale ? 'opacity-60' : ''
      }`}
    >
      <div className="h-0.5 rounded-full bg-muted/30 overflow-hidden">
        <div
          className={`h-full ${
            frac <= 0.25
              ? 'bg-destructive/70'
              : frac <= 0.5
                ? 'bg-warning/60'
                : 'bg-primary/40'
          }`}
          style={{ width: `${frac * 100}%` }}
        />
      </div>

      <div className="flex flex-col gap-1 sm:flex-row sm:items-baseline sm:justify-between">
        <span className="text-xl font-semibold tabular-nums whitespace-nowrap">
          {fmtDcr(s.amountAtoms)} DCR
        </span>
        <span
          className={`text-sm tabular-nums ${
            lapsed
              ? 'text-muted-foreground'
              : frac <= 0.25
                ? 'text-destructive'
                : frac <= 0.5
                  ? 'text-warning'
                  : 'text-muted-foreground'
          }`}
        >
          {stale
            ? 'clock unknown while the list is unreadable'
            : lapsed
              ? 'lapsed'
              : `${mmss(left)} left of ${mmss(total)}`}
        </span>
      </div>

      <p className="text-xs text-muted-foreground">
        <span className="font-mono">{s.game}</span>
        {p?.account ? (
          <>
            {' '}
            · debits account <span className="font-mono">{p.account}</span>
          </>
        ) : (
          <span className="text-warning"> · no account bound</span>
        )}
      </p>

      <div className="text-xs space-y-0.5">
        <span className="text-muted-foreground block">To</span>
        <span className="block font-mono break-all">{s.address}</span>
        <span className="block text-muted-foreground">Bridge-verified escrow · recovery key held by your wallet</span>
      </div>

      <p className="text-xs text-muted-foreground">
        {capAtoms > 0 ? (
          <>
            Today: {fmtDcr(used)} of {toDcr(capAtoms)} DCR used by {s.game}, counting what is
            still waiting.
          </>
        ) : (
          <span className="text-warning">
            No daily cap is set for {s.game}. {fmtDcr(used)} DCR has been asked for in the
            last day.
          </span>
        )}
      </p>

      <div className="text-xs space-y-0.5">
        <span className="text-muted-foreground block">{s.game} says:</span>
        {reason ? (
          <span className="block border-l-2 border-border/60 pl-2 text-muted-foreground line-clamp-3 break-words">
            <bdi>{reason}</bdi>
          </span>
        ) : (
          <span className="block border-l-2 border-border/60 pl-2 text-muted-foreground">
            {s.game} gave no reason.
          </span>
        )}
      </div>

      <dl className="grid grid-cols-2 gap-x-3 gap-y-1 rounded-lg bg-background/50 p-3 text-xs">
        <dt className="text-muted-foreground">Deposit</dt><dd>{depositLabel ?? 'Unverified'}</dd>
        <dt className="text-muted-foreground">Table</dt><dd className="font-mono break-all">{s.tableId}</dd>
        <dt className="text-muted-foreground">Network fee</dt><dd>{fmtDcr(s.fundingFeeAtoms ?? 0)} DCR</dd>
        <dt className="text-muted-foreground">Total wallet debit</dt><dd className="font-semibold">{fmtDcr(s.amountAtoms + (s.fundingFeeAtoms ?? 0))} DCR</dd>
        <dt className="text-muted-foreground">Refund delay</dt><dd>{s.recoveryLockBlocks} blocks after confirmation</dd>
      </dl>
      <p className="text-xs text-muted-foreground">After closing the table, recover mature deposits in Gaming → Recovery. Other players do not need to approve your refund.</p>
      {!verified && <p className="text-xs text-destructive">The bridge has not supplied complete verified payment terms. Approval is unavailable.</p>}

      {err && (
        <div className="text-xs space-y-1">
          {isPassphraseRefusal(err) && (
            <span className="block text-warning">
              That passphrase was not accepted. Nothing was signed and nothing was sent -{' '}
              {s.game}'s request is still waiting, with {mmss(Math.max(0, left))} left to try
              again. The wallet passphrase is the one that unlocks this wallet for spending,
              not the app password for this dashboard.
            </span>
          )}
          <span className="block text-destructive break-words">{err}</span>
        </div>
      )}

      {isArmed && !lapsed ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (verified) void decide(s, true, armed.passphrase);
          }}
          className="space-y-2"
        >
          <input
            type="password"
            autoFocus
            autoComplete="current-password"
            value={armed.passphrase}
            onChange={(e) => setArmed({ id: s.id, passphrase: e.target.value })}
            placeholder="Wallet passphrase"
            className="w-full px-2 py-2 rounded-lg bg-background border border-border text-sm"
          />
          <div className="flex flex-col gap-2 sm:flex-row">
            <button
              type="submit"
              disabled={busyRow || !verified || !armed.passphrase}
              className="inline-flex items-center justify-center gap-1 min-h-9 px-3 py-2 rounded-lg bg-primary/20 text-primary text-xs font-medium hover:bg-primary/30 disabled:opacity-50"
            >
              {busyRow && <Loader2 className="h-3 w-3 animate-spin" />}
              Approve {fmtDcr(s.amountAtoms)} DCR
            </button>
            <button
              type="button"
              onClick={() => setArmed(null)}
              disabled={busyRow}
              className="min-h-9 px-3 py-2 rounded-lg bg-muted/20 text-muted-foreground text-xs font-medium hover:bg-muted/30 disabled:opacity-50"
            >
              Cancel
            </button>
          </div>
        </form>
      ) : (
        <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
          <button
            type="button"
            onClick={() => void decide(s, false)}
            disabled={busyRow || lapsed || stale}
            className="min-h-9 px-3 py-2 rounded-lg bg-muted/20 text-muted-foreground text-xs font-medium hover:bg-muted/30 disabled:opacity-50"
          >
            Deny
          </button>
          <button
            type="button"
            onClick={() => { if (verified) setArmed({ id: s.id, passphrase: '' }); }}
            disabled={
              busyRow || lapsed || stale || watchOnly || fresh || (armed !== null && !isArmed)
            }
            className="min-h-9 px-3 py-2 rounded-lg bg-primary/20 text-primary text-xs font-medium hover:bg-primary/30 disabled:opacity-50"
          >
            Approve...
          </button>
        </div>
      )}

      {lapsed && (
        <span className="block text-xs text-muted-foreground">
          Lapsed. {s.game} was told nothing was decided, and it can ask again.
        </span>
      )}
      {!lapsed && armed !== null && !isArmed && (
        <span className="block text-xs text-muted-foreground text-right">
          Answering another request.
        </span>
      )}
      {fresh && !lapsed && (
        <span className="block text-xs text-muted-foreground text-right">Just arrived.</span>
      )}
    </div>
  );
}

// SpendOutcomes keeps an answered request's result on screen until dismissed.
// An answered request leaves the pending list at once, so the outcome lives
// outside the row that is about to vanish.
export function SpendOutcomes({ d }: { d: SpendDecisions }) {
  const { outcome, setOutcome } = d;
  return (
    <>
      {Object.entries(outcome).map(([id, o]) => (
        <div
          key={id}
          className={`text-xs space-y-0.5 p-2 rounded-lg border ${
            o.txid ? 'bg-success/5 border-success/30' : 'bg-destructive/5 border-destructive/30'
          }`}
        >
          {o.txid ? (
            <>
              <span className="flex items-center gap-1 text-success">
                <Check className="h-3 w-3" /> Paid.
              </span>
              <span className="block font-mono break-all text-muted-foreground">{o.txid}</span>
              <span className="block text-muted-foreground">
                Broadcast. It is not in a block yet.
              </span>
            </>
          ) : (
            <>
              <span className="block text-destructive break-words">
                The payment failed{o.failed ? `: ${o.failed}` : '.'}
              </span>
              <span className="block text-muted-foreground">
                It may have reached the network anyway. This request will not be offered again, so
                check the account's transactions before paying that game another way.
              </span>
            </>
          )}
          <button
            type="button"
            onClick={() =>
              setOutcome((s) => {
                const next = { ...s };
                delete next[id];
                return next;
              })
            }
            className="text-xs text-muted-foreground hover:text-foreground underline"
          >
            Dismiss
          </button>
        </div>
      ))}
    </>
  );
}

// GamingSpendApprovals is the History tab: every request a game has made that
// was answered, paid or refused.
export const GamingSpendApprovals = () => {
  const feed = useGamingSpends();
  const decided = feed.decided;
  const decidedTotal = feed.decidedTotal;
  const histPageSize = 10;
  const [histPage, setHistPage] = useState(1);
  const [hist, setHist] = useState<{ page: number; decided: GamingSpend[] } | null>(null);
  const [histLoading, setHistLoading] = useState(false);
  const totalPages = Math.max(1, Math.ceil(decidedTotal / histPageSize));
  useEffect(() => {
    if (histPage > totalPages) {
      setHistPage(totalPages);
      return;
    }
    if (histPage === 1) {
      setHist(null);
      return;
    }
    let dead = false;
    setHistLoading(true);
    getGamingSpendHistory(histPage, histPageSize)
      .then((h) => {
        if (!dead) setHist({ page: histPage, decided: h.decided });
      })
      .catch(() => {
        if (!dead) setHist(null);
      })
      .finally(() => {
        if (!dead) setHistLoading(false);
      });
    return () => {
      dead = true;
    };
  }, [histPage, totalPages]);
  const histRows = histPage === 1 ? decided : hist?.page === histPage ? hist.decided : [];

    return (
      <div className="space-y-2">
        <h3 className="font-medium text-sm">What games have asked for</h3>
        {decidedTotal === 0 ? (
          <p className="text-xs text-muted-foreground">
            Nothing has been answered yet. Payments and refusals are both kept here.
          </p>
        ) : (
          <div className="space-y-1 p-3 rounded-lg bg-muted/10 border border-border/50">
            {histRows.map((s) => (
              <div key={s.id} className="text-xs py-1 border-b border-border/30 last:border-0">
                <div className="flex items-baseline justify-between gap-3">
                  <span className="truncate">
                    {s.game} - {fmtDcr(s.amountAtoms)} DCR
                  </span>
                  <span className="shrink-0 text-muted-foreground">
                    {s.state}
                    {s.decidedAt ? ` \u00b7 ${fmtWhen(s.decidedAt)}` : ''}
                  </span>
                </div>
                {s.txid && (
                  <span className="block font-mono text-[11px] break-all text-muted-foreground">
                    {s.txid}
                  </span>
                )}
                {s.error && <span className="block text-destructive break-words">{s.error}</span>}
              </div>
            ))}
            {histRows.length === 0 && !histLoading && (
              <p className="text-xs text-muted-foreground">Could not read that page.</p>
            )}
            {histLoading && (
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <Loader2 className="h-3 w-3 animate-spin" />
                Reading that page.
              </div>
            )}
            <Pagination
              currentPage={histPage}
              totalPages={totalPages}
              onPageChange={setHistPage}
              loading={histLoading}
            />
          </div>
        )}
      </div>
    );
};
