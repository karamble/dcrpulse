// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from "react";
import { AlertCircle, Loader2, RefreshCw } from "lucide-react";
import {
  GamingLock,
  GamingReportedState,
  getGamingState,
  reclaimGaming,
} from "../../services/gamingApi";

const fmtDcr = (atoms: number): string =>
  (atoms / 1e8).toFixed(8).replace(/\.?0+$/, "");

// Blocks are the unit the chain enforces; the hours are a courtesy, so they are
// stated as an estimate and never as a deadline.
const roughly = (blocks: number): string => {
  const mins = blocks * 5;
  if (mins < 90) return `about ${Math.round(mins)} minutes`;
  const hours = mins / 60;
  if (hours < 48) return `about ${Math.round(hours)} hours`;
  return `about ${Math.round(hours / 24)} days`;
};

const label = (l: GamingLock): string => {
  if (l.kind === "bond") return "Fidelity bond";
  if (l.kind === "stake") return `Stake at seat ${l.seat}`;
  return `Table bond at seat ${l.seat}`;
};

// GamingLockedCoin lists what a game is holding on the chain and offers to take
// it back once its timelock has run.
//
// It lives here rather than in the game's own page because a reclaim builds and
// broadcasts a transaction: that belongs where the operator already approves
// money, not in a page a game serves.
export const GamingLockedCoin = ({
  game,
  name,
}: {
  game: string;
  name: string;
}) => {
  const [state, setState] = useState<GamingReportedState | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState<Record<string, string>>({});

  const load = useCallback(
    async (refresh = false) => {
      try {
        setState(await getGamingState(game, refresh));
      } catch {
        /* A game that has never reported leaves this empty, which the
           render below already says in words. */
      }
    },
    [game],
  );

  useEffect(() => {
    void load();
  }, [load]);

  // key identifies the button; outpoint is what goes on the wire, and is sent
  // only when a specific piece of coin is named. A synthetic key must never
  // reach the game as an outpoint - that field overrides the seat's own stake.
  const take = async (
    key: string,
    kind: "bond" | "stake" | "tablebond",
    sid?: string,
    outpoint?: string,
  ) => {
    setBusy(key);
    setError(null);
    try {
      const txid = await reclaimGaming(game, kind, sid, outpoint);
      setSent((s) => ({ ...s, [key]: txid }));
      void load(true);
    } catch (e) {
      // The game's refusal names how many blocks are left, which is the
      // only thing that answers "when".
      const body = (e as { response?: { data?: string } })?.response?.data;
      setError(
        typeof body === "string"
          ? body
          : (e as Error)?.message || "Could not reclaim it",
      );
    } finally {
      setBusy(null);
    }
  };

  const locks = (state?.locks ?? []).filter((l) => !l.spent);

  return (
    <div className="space-y-2 pt-2">
      <div className="flex items-center justify-between">
        <h4 className="text-xs font-medium text-muted-foreground">
          Coin {name} has locked
        </h4>
        <button
          type="button"
          onClick={() => void load(true)}
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <RefreshCw className="h-3 w-3" />
          Refresh
        </button>
      </div>

      {state && !state.reported ? (
        <p className="text-xs text-muted-foreground">
          {name} has not reported since this bridge started, so nothing is known
          about what it holds.
        </p>
      ) : locks.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          Nothing of {name}'s is locked on the chain.
        </p>
      ) : (
        <div className="space-y-2">
          {locks.map((l) => (
            <div
              key={l.outpoint}
              className="p-2 rounded-lg bg-muted/10 border border-border/50 space-y-1"
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <span className="text-sm font-medium">
                    {fmtDcr(l.atoms)} DCR &middot; {label(l)}
                  </span>
                  <span className="block font-mono text-[11px] text-muted-foreground break-all">
                    {l.outpoint}
                  </span>
                  {l.sid && (
                    <span className="block font-mono text-[11px] text-muted-foreground break-all">
                      table {l.sid}
                    </span>
                  )}
                </div>
                {sent[l.outpoint] ? (
                  <span className="shrink-0 text-xs text-muted-foreground">
                    sent
                  </span>
                ) : (
                  <button
                    type="button"
                    onClick={() =>
                      void take(l.outpoint, l.kind, l.sid, l.outpoint)
                    }
                    disabled={!l.spendable || busy !== null}
                    title={l.spendable ? undefined : "Still timelocked"}
                    className="shrink-0 inline-flex items-center gap-1 px-2.5 py-1 rounded-lg text-xs font-medium bg-primary/10 text-primary hover:bg-primary/20 disabled:opacity-40 disabled:cursor-not-allowed"
                  >
                    {busy === l.outpoint && (
                      <Loader2 className="h-3 w-3 animate-spin" />
                    )}
                    Take it back
                  </button>
                )}
              </div>

              {sent[l.outpoint] ? (
                <span className="block font-mono text-[11px] text-muted-foreground break-all">
                  {sent[l.outpoint]}
                </span>
              ) : l.spendable ? (
                <span className="block text-xs text-muted-foreground">
                  Free to take back.
                </span>
              ) : (
                <span className="block text-xs text-muted-foreground">
                  Locked until block {l.maturesAt.toLocaleString()} &mdash;{" "}
                  {l.blocksLeft} more
                  {l.blocksLeft === 1 ? " block" : " blocks"},{" "}
                  {roughly(l.blocksLeft)}.
                </span>
              )}
            </div>
          ))}
        </div>
      )}

      {(state?.tables ?? []).length > 0 && (
        <div className="space-y-2">
          <span className="text-xs text-muted-foreground block">
            Tables {name} is still holding. A stake and a table bond are not
            reported with a maturity yet, so taking one back asks the game,
            which answers with how many blocks are left if it is too early.
          </span>
          {(state?.tables ?? []).map((t) => (
            <div
              key={t.sid}
              className="p-2 rounded-lg bg-muted/10 border border-border/50 space-y-1"
            >
              <span className="block text-sm">
                {fmtDcr(t.buyinAtoms)} DCR a seat &middot; {t.state}
                {t.over ? ", over" : ""}
              </span>
              <span className="block font-mono text-[11px] text-muted-foreground break-all">
                {t.sid}
              </span>
              <div className="flex flex-wrap gap-2 pt-1">
                {(["stake", "tablebond"] as const).map((kind) => (
                  <button
                    key={kind}
                    type="button"
                    onClick={() => void take(`${t.sid}:${kind}`, kind, t.sid)}
                    disabled={busy !== null}
                    className="px-2.5 py-1 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 disabled:opacity-40"
                  >
                    {busy === `${t.sid}:${kind}` && (
                      <Loader2 className="h-3 w-3 animate-spin inline mr-1" />
                    )}
                    Take back{" "}
                    {kind === "stake" ? "the stake" : "the table bond"}
                  </button>
                ))}
              </div>
              {sent[`${t.sid}:stake`] && (
                <span className="block font-mono text-[11px] text-muted-foreground break-all">
                  stake sent: {sent[`${t.sid}:stake`]}
                </span>
              )}
              {sent[`${t.sid}:tablebond`] && (
                <span className="block font-mono text-[11px] text-muted-foreground break-all">
                  table bond sent: {sent[`${t.sid}:tablebond`]}
                </span>
              )}
            </div>
          ))}
        </div>
      )}

      {state?.chainErr && (
        <p className="text-xs text-muted-foreground">
          {name} could not read the chain: {state.chainErr}
        </p>
      )}

      {error && (
        <div className="flex items-start gap-2 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span className="break-words">{error}</span>
        </div>
      )}
    </div>
  );
};
