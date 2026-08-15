// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useMemo, useState } from 'react';
import { AlertCircle, LayoutGrid, RefreshCw } from 'lucide-react';
import { GamingGame, GamingReportedState } from '../../services/gamingApi';
import { formatAtomsTrimmed } from '../../utils/amounts';
import { locksFor, sortTables, tableStage, type StageTone } from './gamingTables';

const fmtDcr = (atoms: number): string => formatAtomsTrimmed(atoms);

// The info tone is deliberately primary rather than an `info` class: there is
// no info colour in this build, so bg-info would render as nothing at all.
const toneCls: Record<StageTone, string> = {
  muted: 'bg-muted/30 text-muted-foreground border-border/50',
  info: 'bg-primary/15 text-primary border-primary/30',
  warning: 'bg-warning/15 text-warning border-warning/30',
  success: 'bg-success/15 text-success border-success/30',
};

export interface GameState {
  game: string;
  name: string;
  state: GamingReportedState | null;
  error?: string | null;
}

// Every table every registered game has reported, in one list.
//
// They used to appear inside each game's locked-coin panel, described only as
// somewhere coin might still be stuck. That answers "what can I take back" and
// not "what is my game doing", which is the question somebody has when they are
// about to approve a buy-in for one of them.
export const GamingTablesCard = ({
  games,
  states,
  onRefresh,
  refreshing,
}: {
  games: GamingGame[];
  states: Record<string, GameState>;
  onRefresh: () => void;
  refreshing?: boolean;
}) => {
  const [only, setOnly] = useState<string>('');
  const [showOver, setShowOver] = useState(false);

  const rows = useMemo(() => {
    const out: { game: string; name: string; tip: number; t: GamingReportedState['tables'] }[] = [];
    for (const g of games) {
      const st = states[g.id]?.state;
      if (!st?.tables?.length) continue;
      if (only && only !== g.id) continue;
      out.push({ game: g.id, name: g.name, tip: st.tipHeight ?? 0, t: st.tables });
    }
    return out;
  }, [games, states, only]);

  const total = rows.reduce((n, r) => n + (r.t?.length ?? 0), 0);
  const finished = rows.reduce((n, r) => n + (r.t ?? []).filter((x) => x.over).length, 0);

  return (
    <div className="rounded-xl bg-gradient-card border border-border/50 p-5 space-y-4">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold flex items-center gap-2">
          <LayoutGrid className="h-4 w-4 text-primary" />
          Tables
        </h3>
        <button
          type="button"
          onClick={onRefresh}
          disabled={refreshing}
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground disabled:opacity-50 disabled:cursor-wait"
        >
          <RefreshCw className={`h-3 w-3 ${refreshing ? 'animate-spin' : ''}`} />
          {refreshing ? 'Asking' : 'Refresh'}
        </button>
      </div>

      {games.length > 1 && (
        <div className="flex flex-wrap gap-1">
          {[{ id: '', name: 'All' }, ...games].map((g) => (
            <button
              key={g.id || 'all'}
              type="button"
              onClick={() => setOnly(g.id)}
              className={`px-2 py-1 rounded-md text-xs font-medium transition-colors ${
                only === g.id
                  ? 'bg-primary/20 text-primary'
                  : 'text-muted-foreground hover:bg-muted/30'
              }`}
            >
              {g.name}
            </button>
          ))}
        </div>
      )}

      {total === 0 ? (
        <p className="text-xs text-muted-foreground">
          No game has reported a table. Posting one from a game below starts here.
        </p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            A table's size is reported; how many of its seats are taken is not, so this says how big
            a table is rather than how full.
          </p>
          <div className="divide-y divide-border/40 max-h-[28rem] overflow-y-auto overscroll-contain">
            {rows.map((r) =>
              sortTables(r.t ?? [], r.tip)
                .filter((t) => showOver || !t.over)
                .map((t) => {
                  const stage = tableStage(t, r.tip);
                  const held = locksFor(states[r.game]?.state?.locks ?? [], t.sid);
                  const sum = held.reduce((n, l) => n + l.atoms, 0);
                  const soonest = held.length
                    ? Math.min(...held.map((l) => l.maturesAt || Number.MAX_SAFE_INTEGER))
                    : 0;
                  return (
                    <div key={`${r.game}:${t.sid}`} className="py-2 space-y-1">
                      <div className="flex flex-col gap-1 sm:flex-row sm:items-baseline sm:justify-between">
                        <span className="flex items-center gap-2 min-w-0">
                          <span
                            className={`shrink-0 px-2 py-0.5 rounded-full border text-[11px] font-medium ${
                              toneCls[stage.tone]
                            }`}
                          >
                            {stage.label}
                          </span>
                          <span className="text-sm truncate">
                            {fmtDcr(t.buyinAtoms)} DCR a seat &middot; table of {t.seats}
                          </span>
                        </span>
                        <span className="text-xs text-muted-foreground shrink-0">
                          {sum > 0 ? `${fmtDcr(sum)} DCR here` : 'nothing locked'}
                        </span>
                      </div>

                      <div className="text-[11px] text-muted-foreground space-y-0.5">
                        <span className="block">
                          {r.name}
                          {t.gcid && (
                            <>
                              {' '}
                              &middot; group chat{' '}
                              <span className="font-mono break-all">{t.gcid.slice(0, 16)}</span>
                            </>
                          )}
                        </span>
                        <span className="block font-mono break-all">{t.sid}</span>
                        <span className="block">{stage.line}</span>
                        {sum > 0 && soonest > 0 && soonest !== Number.MAX_SAFE_INTEGER && (
                          <span className="block">
                            The earliest of it comes free at block {soonest.toLocaleString()}.
                          </span>
                        )}
                        {held.some((l) => l.spending) && (
                          <span className="block text-warning">
                            A reclaim of this table's coin is broadcast and waiting for a block.
                          </span>
                        )}
                      </div>
                    </div>
                  );
                }),
            )}
          </div>
          {finished > 0 && (
            <button
              type="button"
              onClick={() => setShowOver((v) => !v)}
              className="text-xs text-primary hover:underline"
            >
              {showOver ? 'Hide finished tables' : `and ${finished} finished - show`}
            </button>
          )}
        </>
      )}

      {games.map((g) =>
        states[g.id]?.error ? (
          <div key={g.id} className="flex items-start gap-2 text-xs text-destructive">
            <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
            <span className="break-words">
              {g.name}'s tables could not be read: {states[g.id].error}
            </span>
          </div>
        ) : null,
      )}
    </div>
  );
};
