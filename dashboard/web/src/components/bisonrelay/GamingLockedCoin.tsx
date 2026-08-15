// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from "react";
import { AlertCircle, AlertTriangle, Loader2, RefreshCw } from "lucide-react";
import {
  GamingLock,
  GamingReportedState,
  getGamingState,
  reclaimGaming,
} from "../../services/gamingApi";

const fmtDcr = (atoms: number): string =>
  (atoms / 1e8).toFixed(8).replace(/\.?0+$/, "");

const fmtWhen = (unix: number): string =>
  new Date(unix * 1000).toLocaleString();

const blocksWord = (blocks: number): string =>
  `${blocks.toLocaleString()} ${blocks === 1 ? "block" : "blocks"}`;

// Blocks are the unit the chain enforces; the hours are a courtesy, so they are
// stated as an estimate and never as a deadline.
const roughly = (blocks: number): string => {
  if (blocks <= 1) return "about five minutes, give or take";
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

// One target is one button: the bond is a singleton, every other lock is named
// by its outpoint, and a seat's stake is named by its table.
const keyOf = (l: GamingLock): string =>
  l.kind === "bond" ? "bond" : l.outpoint;

// What came of asking for one target. The game refuses in sentences, so these
// are the meanings behind them rather than the sentences themselves.
type Attempt =
  | { kind: "asking" }
  | { kind: "sent"; txid: string }
  | { kind: "moving" }
  | { kind: "early"; blocks: number; askedAt: number }
  | { kind: "gone" }
  | { kind: "empty" }
  | { kind: "surplus" }
  | { kind: "stale" }
  | { kind: "failed"; text: string };

// classify reads the game's refusal. The patterns are the game's own wording;
// anything unrecognised is shown as it arrived rather than guessed at.
const classify = (text: string, askedAt: number): Attempt => {
  const early = /not spendable for another (\d+) block/.exec(text);
  if (early) return { kind: "early", blocks: Number(early[1]), askedAt };
  if (text.includes("holds no coin")) return { kind: "gone" };
  if (text.includes("nothing was ever paid into seat")) return { kind: "empty" };
  if (text.includes("spare coin at the same script")) return { kind: "surplus" };
  if (text.includes("did not answer in time")) return { kind: "moving" };
  if (text.includes("takes no more stake")) return { kind: "stale" };
  return { kind: "failed", text };
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
  const [attempts, setAttempts] = useState<Record<string, Attempt>>({});
  const [panelErr, setPanelErr] = useState<string | null>(null);

  const load = useCallback(
    async (refresh = false) => {
      try {
        const s = await getGamingState(game, refresh);
        setState(s);
        if (!refresh) return;
        // A refresh is a fresh look, so old refusals are dropped. A sent txid
        // stays, and so does a timed-out ask unless the coin has left: that
        // reclaim may be broadcast already, and asking twice spends twice.
        const live = new Set<string>([
          ...(s.locks ?? []).filter((l) => !l.spent).map(keyOf),
          ...(s.tables ?? []).map((t) => `${t.sid}:stake`),
        ]);
        setAttempts((a) => {
          const next: Record<string, Attempt> = {};
          for (const [k, v] of Object.entries(a)) {
            if (v.kind === "sent") next[k] = v;
            else if (v.kind === "moving" && live.has(k)) next[k] = v;
          }
          return next;
        });
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
    const askedAt = state?.tipHeight ?? 0;
    setAttempts((a) => ({ ...a, [key]: { kind: "asking" } }));
    setPanelErr(null);
    try {
      const txid = await reclaimGaming(game, kind, sid, outpoint);
      setAttempts((a) => ({ ...a, [key]: { kind: "sent", txid } }));
      void load(true);
    } catch (e) {
      const err = e as {
        response?: { data?: unknown; status?: number };
        message?: string;
      };
      const body = err?.response?.data;
      const text =
        (typeof body === "string" && body.trim()) ||
        err?.message ||
        "Could not reclaim it";
      const a = classify(text, askedAt);
      const status = err?.response?.status;
      // 403 and 409 are the bridge refusing to carry the request at all, which
      // says nothing about the coin, so they belong to the panel not a row.
      if (a.kind === "failed" && (status === 403 || status === 409)) {
        setAttempts((s) => {
          const next = { ...s };
          delete next[key];
          return next;
        });
        setPanelErr(text);
        return;
      }
      setAttempts((s) => ({ ...s, [key]: a }));
    }
  };

  // What the game said about one target, in words rather than as a sentence
  // quoted back at whoever asked.
  const outcome = (a: Attempt | undefined) => {
    if (!a) return null;
    switch (a.kind) {
      case "asking":
        return (
          <span className="block text-xs text-muted-foreground">
            Asking {name}. It builds and broadcasts the transaction itself, so
            this can take a while.
          </span>
        );
      case "sent":
        return (
          <>
            <span className="block text-xs text-muted-foreground">
              {name} broadcast it.
            </span>
            <span className="block font-mono text-[11px] text-muted-foreground break-all">
              {a.txid}
            </span>
          </>
        );
      case "moving":
        return (
          <span className="block text-xs text-warning">
            {name} did not answer in time. It may have broadcast the reclaim
            anyway, so this will not offer to ask again. Refresh to see whether
            the coin has moved.
          </span>
        );
      case "early":
        return (
          <span className="block text-xs text-muted-foreground">
            {name} says it is not spendable for another{" "}
            {blocksWord(a.blocks)}, {roughly(a.blocks)}.
            {a.askedAt > 0
              ? ` Counted from block ${a.askedAt.toLocaleString()}, where the chain stood when it was asked.`
              : ""}
          </span>
        );
      case "gone":
        return (
          <span className="block text-xs text-muted-foreground">
            {name} says that script holds no coin, so there is nothing left to
            take back. Refresh to bring its record up to date.
          </span>
        );
      case "empty":
        return (
          <span className="block text-xs text-muted-foreground">
            {name} says nothing was ever paid into that seat, so there is no
            stake to refund.
          </span>
        );
      case "surplus":
        return (
          <span className="block text-xs text-warning">
            {name} found spare coin at the same script and would not decide on
            its own what to spend, so it built nothing. That script needs
            looking at on the chain before asking again.
          </span>
        );
      case "stale":
        return (
          <span className="block text-xs text-warning">
            {name} answered as though this were someone joining a table, which a
            refund is not. That answer only comes from an older build of the
            game; update it and ask again.
          </span>
        );
      case "failed":
        return (
          <span className="block text-xs text-destructive break-words">
            {name} would not do it: {a.text}
          </span>
        );
    }
  };

  const asking = Object.values(attempts).some((a) => a.kind === "asking");
  const unspent = (state?.locks ?? []).filter((l) => !l.spent);
  const spent = (state?.locks ?? []).filter((l) => l.spent);
  const tables = state?.tables ?? [];
  // The bond is the game's route back to every table it has, so it is offered
  // last and never as the first thing to hand.
  const ordered = [
    ...unspent.filter((l) => l.kind !== "bond"),
    ...unspent.filter((l) => l.kind === "bond"),
  ];
  const othersHold =
    unspent.some((l) => l.kind !== "bond") || tables.length > 0;
  const tip = state?.tipHeight ?? 0;
  // Tables mid-settlement, and tables whose stake is already listed as a lock
  // with a maturity of its own.
  const settling = new Set(
    tables.filter((t) => t.settling).map((t) => t.sid),
  );
  const staked = new Set(
    unspent.filter((l) => l.kind === "stake" && l.sid).map((l) => l.sid),
  );

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

      {state?.reported && (
        <p className="text-xs text-muted-foreground">
          {state.reportedAt
            ? `${name} last reported ${fmtWhen(state.reportedAt)}. `
            : ""}
          {tip > 0
            ? `Everything below is measured against block ${tip.toLocaleString()}.`
            : "The chain could not be read, so nothing below is measured against a block height."}
        </p>
      )}

      {panelErr && (
        <div className="flex items-start gap-2 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span className="break-words">{panelErr}</span>
        </div>
      )}

      {state?.chainErr && (
        <div className="flex items-start gap-2 p-2 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning">
          <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span className="break-words">
            {name} could not read the chain: {state.chainErr}. What follows is
            the game's own record of what it locked, with nothing below checked
            against the chain.
          </span>
        </div>
      )}

      {!state ? (
        <p className="text-xs text-muted-foreground">
          Nothing has come back from the bridge yet about what {name} holds.
        </p>
      ) : !state.reported ? (
        <p className="text-xs text-muted-foreground">
          {name} has not reported since this bridge started, so nothing is known
          about what it holds.
        </p>
      ) : ordered.length === 0 ? (
        state.chainErr ? (
          <p className="text-xs text-muted-foreground">
            Nothing can be listed while the chain cannot be read, which is not
            the same as {name} holding nothing.
          </p>
        ) : (
          <p className="text-xs text-muted-foreground">
            {name} reported, and holds nothing on the chain.
          </p>
        )
      ) : (
        <div className="space-y-2">
          {ordered.map((l) => {
            const k = keyOf(l);
            const a = attempts[k];
            // Three answers, not two: matured, waiting with a known height, or
            // a maturity nobody here can see.
            const gate = l.spendable
              ? "ready"
              : l.maturesAt > 0 && tip > 0
                ? "wait"
                : "unknown";
            const left =
              l.blocksLeft > 0 ? l.blocksLeft : Math.max(0, l.maturesAt - tip);
            // A stake is the settlement's input, so it is not offered while its
            // table can still settle.
            const held = l.kind === "stake" && !!l.sid && settling.has(l.sid);
            const offer =
              gate !== "wait" &&
              !held &&
              a?.kind !== "sent" &&
              a?.kind !== "moving";
            return (
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
                  {offer && (
                    <button
                      type="button"
                      onClick={() => void take(k, l.kind, l.sid, l.outpoint)}
                      disabled={asking}
                      className="shrink-0 inline-flex items-center gap-1 px-2.5 py-1 rounded-lg text-xs font-medium bg-primary/10 text-primary hover:bg-primary/20 disabled:opacity-40 disabled:cursor-not-allowed"
                    >
                      {a?.kind === "asking" && (
                        <Loader2 className="h-3 w-3 animate-spin" />
                      )}
                      Take it back
                    </button>
                  )}
                </div>

                {gate === "ready" && !held && (
                  <span className="block text-xs text-muted-foreground">
                    Free to take back.
                  </span>
                )}
                {held && (
                  <span className="block text-xs text-warning">
                    That table is still settling. Taking this stake back now
                    would spend an input the settlement needs and defeat the
                    payout every seat signed, so it is not offered here.
                  </span>
                )}
                {gate === "wait" && left > 0 && (
                  <span className="block text-xs text-muted-foreground">
                    Locked until block {l.maturesAt.toLocaleString()} &mdash;{" "}
                    {blocksWord(left)} more, {roughly(left)}. There is nothing
                    to ask for until then.
                  </span>
                )}
                {gate === "wait" && left === 0 && (
                  <span className="block text-xs text-muted-foreground">
                    Block {l.maturesAt.toLocaleString()} has been reached, but{" "}
                    {name} has not counted this as spendable yet. Give it a
                    block and refresh.
                  </span>
                )}
                {gate === "unknown" && (
                  <span className="block text-xs text-muted-foreground">
                    How long is left is not known here, because the chain could
                    not be read. Asking is safe: {name} counts the confirmations
                    itself and builds nothing if the lock has not run.
                  </span>
                )}

                {l.kind === "bond" && othersHold && (
                  <span className="block text-xs text-warning">
                    Take this one back last. The bond is how {name} reads its
                    own tables back, and without it the route to every stake and
                    table bond above goes with it. It is recoverable, by posting
                    a new bond, but nothing here can reach that coin until it
                    does.
                  </span>
                )}

                {outcome(a)}
              </div>
            );
          })}
        </div>
      )}

      {spent.length > 0 && (
        <div className="space-y-1 p-2 rounded-lg bg-muted/10 border border-border/50">
          <span className="text-xs text-muted-foreground block">
            {name}'s record of coin that has already left these scripts. It is
            listed because a missing row and a spent row are not the same thing;
            there is nothing here to take back.
          </span>
          {spent.map((l) => (
            <div key={l.outpoint} className="opacity-60">
              <span className="block text-xs text-muted-foreground">
                {fmtDcr(l.atoms)} DCR &middot; {label(l)}
              </span>
              <span className="block font-mono text-[11px] text-muted-foreground break-all">
                {l.outpoint}
              </span>
            </div>
          ))}
        </div>
      )}

      {tables.length > 0 && (
        <div className="space-y-2">
          <span className="text-xs text-muted-foreground block">
            Tables {name} is still holding. Their stakes and bonds are listed
            above with the rest of the locks, each with its own maturity. A
            stake {name} never recorded as a lock can only be asked for, and it
            answers with how many blocks are left if it is too early.
          </span>
          {tables.map((t) => {
            const k = `${t.sid}:stake`;
            const a = attempts[k];
            const listed = staked.has(t.sid);
            const offer =
              !listed &&
              !t.settling &&
              a?.kind !== "sent" &&
              a?.kind !== "moving";
            return (
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
                {t.settling ? (
                  <span className="block text-xs text-warning">
                    This table is still settling. Taking the stake back now
                    would spend an input the settlement needs and defeat the
                    payout every seat signed, so it is not offered here.
                  </span>
                ) : (
                  listed && (
                    <span className="block text-xs text-muted-foreground">
                      This table's stake is listed above, with the height it
                      matures at.
                    </span>
                  )
                )}
                {offer && (
                  <div className="flex flex-wrap gap-2 pt-1">
                    <button
                      type="button"
                      onClick={() => void take(k, "stake", t.sid)}
                      disabled={asking}
                      className="px-2.5 py-1 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 disabled:opacity-40 disabled:cursor-not-allowed"
                    >
                      {a?.kind === "asking" && (
                        <Loader2 className="h-3 w-3 animate-spin inline mr-1" />
                      )}
                      Take back the stake
                    </button>
                  </div>
                )}
                {outcome(a)}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
