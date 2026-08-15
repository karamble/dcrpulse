// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, AlertTriangle, Check, Loader2 } from 'lucide-react';
import {
  GamePolicy,
  GamingReportedTable,
  GamingSpend,
  decideGamingSpend,
  getGamingState,
} from '../../services/gamingApi';
import { validateAddress } from '../../services/api';
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

interface Ownership {
  state: 'checking' | 'mine' | 'theirs' | 'invalid' | 'unknown';
  account?: number;
  error?: string;
}

export const GamingSpendApprovals = ({
  policies,
  bridgeEnabled,
  gameCount,
  mode = 'pending',
}: {
  // Deciding and browsing have opposite urgencies, so they are separate
  // sections. The component serves both because the data and the wording are
  // the same; only which half renders differs.
  mode?: 'pending' | 'history';
  // The policy carries the account a spend is drawn from and the caps it sits
  // under. All of it is already on this page; the approval row simply never
  // asked for it, so somebody approved a debit without being told which
  // bankroll it came out of.
  policies?: Record<string, GamePolicy>;
  bridgeEnabled?: boolean;
  gameCount?: number;
}) => {
  // The poll lives in a store, so the count survives this panel not being on
  // screen and the strip and the tab badge read the same answer.
  const feed = useGamingSpends();
  const spends: GamingSpend[] | null = feed.lastOkAt ? feed.spends : null;
  const offset = feed.offset;
  const [showAll, setShowAll] = useState(false);

  // armed is the one row whose passphrase field is open. One field for the
  // whole panel meant two live Approve buttons sharing it, which is a wrong-row
  // payment waiting to happen.
  const [armed, setArmed] = useState<{ id: string; passphrase: string } | null>(null);
  const [deciding, setDeciding] = useState<string | null>(null);
  const [rowErr, setRowErr] = useState<Record<string, string>>({});
  const [outcome, setOutcome] = useState<Record<string, { txid?: string; failed?: string }>>({});
  const [watchOnly, setWatchOnly] = useState(false);
  const firstSeen = useRef<Record<string, number>>({});

  const refresh = refreshGamingSpends;

  // One miss is a flap on a five second poll. Two is worth saying, because by
  // then a game could be waiting and this list would not show it.
  const stale = feed.failures >= 2;
  const pollErr = feed.error;
  const lastOkAt = feed.lastOkAt;

  // Sorted by deadline and never re-sorted while one runs, so a row cannot move
  // out from under a cursor that is on its way to a button.
  const pending = feed.pending;

  // A second hand of its own, so a deadline moves between five second polls.
  // Only while something is actually counting down.
  const [, setTick] = useState(0);
  const counting = pending.length > 0;
  useEffect(() => {
    if (!counting) return;
    const t = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, [counting]);

  const now = Math.floor(Date.now() / 1000) + offset;
  const decided = useMemo(
    () => (spends ?? []).filter((s) => s.state !== 'pending' && s.state !== 'publishing'),
    [spends],
  );
  // A payment on its way to the network: nothing to answer, but its money is
  // committed and the row says so until the outcome lands.
  const publishing = useMemo(
    () => (spends ?? []).filter((s) => s.state === 'publishing'),
    [spends],
  );

  // Stamped during render, not in an effect. An effect runs after the first
  // paint, so the row would already be on screen with a live Approve button
  // for the moment the guard exists to cover.
  for (const s of pending) {
    if (!firstSeen.current[s.id]) firstSeen.current[s.id] = Date.now();
  }

  // What this game has already committed today, by the same rule the server
  // applies: everything approved inside a day, plus everything still waiting,
  // because several requests answered at once would otherwise walk past a cap
  // none of them individually passed.
  const usedToday = (game: string): number =>
    (spends ?? []).reduce((total, s) => {
      if (s.game !== game) return total;
      if (s.state === 'pending' || s.state === 'publishing') return total + s.amountAtoms;
      if (s.state === 'approved' && s.decidedAt && now - s.decidedAt < 86400) {
        return total + s.amountAtoms;
      }
      return total;
    }, 0);

  // Whether the address a game named belongs to this wallet. For a buy-in the
  // answer should be no; yes is the odd one, and "could not check" is its own
  // answer rather than a quiet no.
  const [owners, setOwners] = useState<Record<string, Ownership>>({});
  useEffect(() => {
    for (const s of pending) {
      if (owners[s.address]) continue;
      setOwners((o) => ({ ...o, [s.address]: { state: 'checking' } }));
      validateAddress(s.address)
        .then((v) =>
          setOwners((o) => ({
            ...o,
            [s.address]: v.isValid
              ? { state: v.isMine ? 'mine' : 'theirs', account: v.accountNumber }
              : { state: 'invalid' },
          })),
        )
        .catch((e) =>
          setOwners((o) => ({
            ...o,
            [s.address]: { state: 'unknown', error: apiError(e, 'the check failed') },
          })),
        );
    }
  }, [pending, owners]);

  // What the game says its open tables cost, so an amount can be corroborated
  // without pretending the request named one.
  const [tables, setTables] = useState<Record<string, GamingReportedTable[] | null>>({});
  useEffect(() => {
    for (const game of new Set(pending.map((s) => s.game))) {
      if (game in tables) continue;
      setTables((t) => ({ ...t, [game]: null }));
      getGamingState(game)
        .then((st) => setTables((t) => ({ ...t, [game]: st.tables ?? [] })))
        .catch(() => setTables((t) => ({ ...t, [game]: null })));
    }
  }, [pending, tables]);

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
      await refresh();
    } catch (e) {
      const text = apiError(e, 'could not answer that request');
      if (isWatchOnly(text)) setWatchOnly(true);
      setRowErr((r) => ({ ...r, [s.id]: text }));
    } finally {
      setDeciding(null);
    }
  };

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

  if (mode === 'history') {
    return (
      <div className="space-y-2">
        <h3 className="font-medium text-sm">What games have asked for</h3>
        {decided.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            Nothing has been answered yet. Payments and refusals are both kept here.
          </p>
        ) : (
          <div className="space-y-1 p-3 rounded-lg bg-muted/10 border border-border/50">
            {(showAll ? decided : decided.slice(0, 10)).map((s) => (
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
            {decided.length > 10 && (
              <button
                type="button"
                onClick={() => setShowAll((v) => !v)}
                className="text-xs text-primary hover:underline"
              >
                {showAll ? 'Show fewer' : `Showing 10 of ${decided.length} \u00b7 show all`}
              </button>
            )}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-2" aria-live="polite">
      <div className="flex items-baseline justify-between gap-2">
        <h3 className="font-medium text-sm">Buy-in approvals</h3>
        {pending.length > 0 && (
          <span className="text-xs text-warning">
            {pending.length === 1 ? '1 waiting' : `${pending.length} waiting`}
          </span>
        )}
      </div>

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

      {pending.map((s) => {
        const left = s.expiresAt - now;
        const total = Math.max(1, s.expiresAt - s.requestedAt);
        const frac = Math.max(0, Math.min(1, left / total));
        const lapsed = left <= 0;
        const fresh = Date.now() - (firstSeen.current[s.id] ?? 0) < 1500;
        const p = policies?.[s.game];
        const own = owners[s.address];
        const used = usedToday(s.game);
        const capAtoms = p?.perDayCapDcr ? Math.round(p.perDayCapDcr * 1e8) : 0;
        const reason = sanitizeReason(s.reason ?? '');
        const known = tables[s.game];
        const matches = (known ?? []).filter((t) => !t.over && t.buyinAtoms === s.amountAtoms);
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
              {own?.state === 'mine' && (
                <span className="block text-warning">
                  This address is in your own wallet (account {own.account}). A buy-in that pays you
                  back is not what a table looks like.
                </span>
              )}
              {own?.state === 'theirs' && (
                <span className="block text-muted-foreground">Not an address in this wallet.</span>
              )}
              {own?.state === 'invalid' && (
                <span className="block text-destructive">
                  This is not a valid address, so nothing could be paid to it.
                </span>
              )}
              {own?.state === 'unknown' && (
                <span className="block text-muted-foreground">
                  Could not check whether this address is yours: {own.error}.
                </span>
              )}
              {own?.state === 'checking' && (
                <span className="block text-muted-foreground">Checking whose address this is.</span>
              )}
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

            <p className="text-xs text-muted-foreground">
              This request names no table; nothing links it to one but the line above.{' '}
              {known === null || known === undefined
                ? `${s.game} has not said what tables it holds, so there is nothing to compare it against.`
                : matches.length === 1
                  ? `${s.game} reports one open table with a ${fmtDcr(
                      matches[0].buyinAtoms,
                    )} DCR buy-in. The amount matches; that is all it proves.`
                  : matches.length > 1
                    ? `${s.game} reports ${matches.length} open tables with that buy-in.`
                    : `${s.game} reports no open table with a ${fmtDcr(s.amountAtoms)} DCR buy-in.`}
            </p>

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
                  void decide(s, true, armed.passphrase);
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
                    disabled={busyRow || !armed.passphrase}
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
                  onClick={() => setArmed({ id: s.id, passphrase: '' })}
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
      })}

      {/* An answered request leaves the pending list at once, so the outcome
       * lives out here rather than inside a row that is about to vanish.
       * A txid that flashes for one poll and disappears is a txid nobody
       * has. It stays until it is dismissed; it is also in the history
       * below, permanently. */}
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

      {decided.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {decided.length === 1 ? '1 earlier request' : `${decided.length} earlier requests`}, with
          their transaction ids and any refusals, are under History.
        </p>
      )}

      {spends === null && !pollErr && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          Reading what games have asked for.
        </div>
      )}
      {spends === null && pollErr && (
        <div className="flex items-start gap-2 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span className="break-words">
            The list of requests could not be read: {pollErr}. A game may be waiting and this would
            not show it.
          </span>
        </div>
      )}
    </div>
  );
};
