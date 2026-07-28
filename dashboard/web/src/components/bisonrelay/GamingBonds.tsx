// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Coins, Loader2 } from 'lucide-react';
import { getGamingBond, reclaimGaming, type GamingBond } from '../../services/gamingApi';

const fmtDcr = (atoms: number): string => (atoms / 1e8).toFixed(8).replace(/\.?0+$/, '');

// A game's standing bond is coin locked for weeks with nothing elsewhere to say
// so. Taking it back signs and broadcasts a transaction, which is why it lives
// here and not in the game's own page.

export const GamingBonds = ({ games }: { games: string[] }) => {
  const [bonds, setBonds] = useState<Record<string, GamingBond>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState<string | null>(null);

  const refresh = useCallback(() => {
    games.forEach((game) => {
      getGamingBond(game)
        .then((b) => setBonds((all) => ({ ...all, [game]: b })))
        .catch(() => {});
    });
  }, [games]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  if (games.length === 0) return null;

  const reclaim = (game: string) => {
    setBusy(game);
    setError(null);
    setSent(null);
    reclaimGaming(game, 'bond')
      .then((r) => {
        setSent(r.txid);
        refresh();
      })
      .catch((e) => {
        const body = (e as { response?: { data?: string } })?.response?.data;
        setError(String(body ?? (e as Error)?.message ?? '').trim() || 'could not take that bond back');
      })
      .finally(() => setBusy(null));
  };

  return (
    <div className="space-y-2">
      <h3 className="font-medium flex items-center gap-2 text-sm">
        <Coins className="h-4 w-4" />
        Bonds
      </h3>
      <div className="space-y-2 p-3 rounded-lg bg-muted/10 border border-border/50">
        <p className="text-xs text-muted-foreground">
          A seat has to cost something, or a table can be filled or blocked by keys that cost
          nothing to make. A bond is locked when a game is first used and is yours again after the
          lock; it is never forfeitable.
        </p>

        {games.map((game) => {
          const b = bonds[game];
          if (!b) {
            return (
              <div key={game} className="text-xs text-muted-foreground">
                Asking {game}...
              </div>
            );
          }
          if (!b.hasDeposit) {
            return (
              <div key={game} className="text-xs text-muted-foreground">
                <span className="capitalize">{game}</span> has no bond posted yet, so it can join
                nothing. It is posted from the game itself.
              </div>
            );
          }
          if (b.spent) {
            return (
              <div key={game} className="text-xs text-muted-foreground">
                <span className="capitalize">{game}</span>: nothing is locked at{' '}
                <code className="break-all">{b.outpoint}</code> any more.
              </div>
            );
          }
          return (
            <div key={game} className="space-y-1 text-xs">
              <div className="flex items-center justify-between gap-2">
                <span>
                  <span className="capitalize font-medium">{game}</span>{' '}
                  {fmtDcr(b.atoms || b.minAtoms)} DCR locked
                  {b.spendable
                    ? ', free to take back'
                    : b.maturesAt
                      ? `, free from block ${b.maturesAt.toLocaleString()}`
                      : ''}
                </span>
                <button
                  type="button"
                  onClick={() => reclaim(game)}
                  disabled={busy !== null || !b.spendable}
                  className="inline-flex items-center gap-1 px-2 py-1 rounded-md bg-muted/30 hover:bg-muted/50 disabled:opacity-50 text-xs font-medium shrink-0"
                >
                  {busy === game && <Loader2 className="h-3 w-3 animate-spin" />}
                  Take it back
                </button>
              </div>
              <code className="block text-muted-foreground break-all">{b.outpoint}</code>
              {!b.spendable && b.blocksLeft ? (
                <span className="text-muted-foreground">
                  {b.blocksLeft.toLocaleString()} blocks to go, about{' '}
                  {Math.round((b.blocksLeft * 5) / 60 / 24)} days.
                </span>
              ) : null}
            </div>
          );
        })}

        {sent && (
          <div className="p-2 rounded-lg bg-muted/20 border border-border/50 text-xs">
            Sent. It lands in the gaming account once it confirms.
            <code className="block break-all mt-1">{sent}</code>
          </div>
        )}
        {error && (
          <div className="p-2 rounded-lg bg-destructive/10 border border-destructive/30 text-xs text-destructive">
            {error}
          </div>
        )}
      </div>
    </div>
  );
};
