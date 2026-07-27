// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { KeyRound, Loader2 } from 'lucide-react';
import { GamingBackup, getGamingIdentityBackup } from '../../services/gamingApi';

// GamingIdentityBackup offers to carry a game's seed out to the person.
//
// Every other secret a game touches belongs to the host: the wallet it asks to
// spend from, the Bison Relay identity it speaks through. Its own keys do not.
// A game generates a seed on first run, keeps it in its own volume, and derives
// its fidelity bond and every table key from it - so nothing else has a copy,
// by design, because the game is the component nobody trusts.
//
// The cost of that lands on the user silently. Remove the volume and the seed
// is gone, which leaves the bond permanently unspendable and any stake still in
// escrow unrefundable: both are locked to keys only that seed derives. A
// `docker compose down -v` is the ordinary way to do it, and it says nothing.
//
// So this is deliberately a button and not a panel. Nothing is fetched until
// somebody asks, and what comes back is never stored by the dashboard.
export const GamingIdentityBackup = ({ games }: { games: string[] }) => {
  const [backup, setBackup] = useState<GamingBackup | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (games.length === 0) return null;

  const reveal = async (game: string) => {
    setBusy(game);
    setError(null);
    setBackup(null);
    try {
      setBackup(await getGamingIdentityBackup(game));
    } catch (e) {
      const detail =
        (e as { response?: { data?: string } })?.response?.data ?? (e as Error)?.message ?? '';
      setError(String(detail).trim() || 'could not read that game’s seed');
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-2">
      <h3 className="font-medium flex items-center gap-2 text-sm">
        <KeyRound className="h-4 w-4" />
        Recovery key
      </h3>

      <div className="space-y-2 p-3 rounded-lg bg-muted/10 border border-border/50">
        <p className="text-xs text-muted-foreground">
          A game keeps its own keys, and nothing else has a copy. Its fidelity bond and any stake it
          has in escrow are locked to keys derived from the seed below — so if this game’s data is
          deleted without a copy of it, that money cannot be recovered by anyone, ever. Write it
          down somewhere safe.
        </p>

        <div className="flex flex-wrap gap-1">
          {games.map((game) => (
            <button
              key={game}
              type="button"
              onClick={() => void reveal(game)}
              disabled={busy !== null}
              className="inline-flex items-center gap-1 px-2 py-1 rounded-md bg-muted/30 hover:bg-muted/50 disabled:opacity-50 text-xs font-medium"
            >
              {busy === game && <Loader2 className="h-3 w-3 animate-spin" />}
              Show {game} recovery key
            </button>
          ))}
        </div>

        {error && (
          <div className="p-2 rounded-lg bg-destructive/10 border border-destructive/30 text-xs text-destructive">
            {error}
          </div>
        )}

        {backup && (
          <div className="space-y-1">
            <p className="text-xs text-muted-foreground">{backup.game} recovery key</p>
            <code className="block p-2 rounded-md bg-background border border-border/50 text-xs break-all select-all">
              {backup.seedHex}
            </code>
            {backup.bondOutpoint && (
              <p className="text-xs text-muted-foreground break-all">
                Reclaims the bond at {backup.bondOutpoint}
              </p>
            )}
            <button
              type="button"
              onClick={() => setBackup(null)}
              className="px-2 py-1 rounded-md bg-muted/30 hover:bg-muted/50 text-xs font-medium"
            >
              Hide
            </button>
          </div>
        )}
      </div>
    </div>
  );
};
