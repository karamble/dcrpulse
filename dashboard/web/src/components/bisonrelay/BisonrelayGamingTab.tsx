// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { GamingSpendApprovals } from './GamingSpendApprovals';
import { Loader2, ShieldCheck } from 'lucide-react';
import {
  GamingGame,
  GamingSettings,
  getGamingGames,
  getGamingSettings,
  setGamingSettings,
} from '../../services/gamingApi';
import { AccountInfo, getAccounts } from '../../services/api';

const fmtDcr = (v: number) => `${v.toLocaleString(undefined, { maximumFractionDigits: 8 })} DCR`;

// BisonrelayGamingTab configures the bridge that stands between registered
// games and the wallet. Games are untrusted and are not run here: a person runs
// one wherever they like and it connects in, so nothing here hands it
// credentials. It reaches one account, under caps, through the bridge.
//
// Account scope is enforced by this policy rather than by the wallet, because
// dcrwallet accounts share one seed and one unlock passphrase. It is a boundary
// above the wallet, never a cryptographic one below it.
export const BisonrelayGamingTab = () => {
  const [settings, setSettings] = useState<GamingSettings | null>(null);
  const [draft, setDraft] = useState<GamingSettings | null>(null);
  const [games, setGames] = useState<GamingGame[]>([]);
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [newGame, setNewGame] = useState('');
  const [newName, setNewName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const s = await getGamingSettings();
      setSettings(s);
      setDraft((d) => d ?? s);
      setError(null);
    } catch (err: any) {
      setError(err?.message || 'Could not load the gaming policy');
    }
    try {
      setGames(await getGamingGames());
    } catch {
      /* catalogue is best-effort */
    }
    try {
      setAccounts(await getAccounts());
    } catch {
      /* wallet may still be starting */
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const apply = async (next: GamingSettings) => {
    setBusy(true);
    setError(null);
    try {
      const applied = await setGamingSettings(next);
      setSettings(applied);
      setDraft(applied);
      setGames(await getGamingGames());
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Save failed');
    } finally {
      setBusy(false);
    }
  };

  // Registering and removing save against what is stored, not the draft: they
  // are their own act, and carrying half-typed caps along with them would
  // commit edits nobody pressed Save for.
  const registerGame = () => {
    if (!settings) return;
    const id = newGame.trim().toLowerCase();
    if (!id) return;
    const name = newName.trim();
    apply({
      ...settings,
      installedGames: [...settings.installedGames, id],
      gameNames: name ? { ...(settings.gameNames ?? {}), [id]: name } : settings.gameNames,
    });
    setNewGame('');
    setNewName('');
  };

  const toggleGame = (id: string) => {
    if (!settings) return;
    apply({
      ...settings,
      installedGames: settings.installedGames.filter((g) => g !== id),
    });
  };

  if (!settings || !draft) {
    return (
      <div className="flex items-center gap-2 p-6 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading gaming policy...
      </div>
    );
  }

  const boundAccount = draft.account.trim();

  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-medium flex items-center gap-2">
          <ShieldCheck className="h-4 w-4" />
          Gaming
        </h2>
        <p className="text-sm text-muted-foreground">
          Games are separate programs that play over Bison Relay against other people, staking real
          funds. They never hold your wallet keys: everything they stake passes through the policy
          below, and they can only ever touch the one account you bind here.
        </p>
      </div>

      {error && (
        <div className="p-3 rounded-lg bg-destructive/10 border border-destructive/30 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="flex items-center justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
        <div>
          <span className="font-medium block">Gaming bridge</span>
          <span className="text-sm text-muted-foreground block">
            {settings.enabled
              ? `On. Installed games may stake from ${settings.account}.`
              : boundAccount
                ? 'Off. No game can stake anything.'
                : 'Off. Bind a gaming account below to enable.'}
          </span>
        </div>
        <button
          type="button"
          onClick={() => apply({ ...draft, enabled: !settings.enabled })}
          disabled={busy || !boundAccount}
          className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
            settings.enabled
              ? 'bg-success/20 text-success hover:bg-success/30'
              : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
          } disabled:opacity-50 disabled:cursor-not-allowed`}
        >
          {settings.enabled ? 'On' : 'Off'}
        </button>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-xs space-y-1 sm:col-span-2">
          <span className="text-muted-foreground block">
            Gaming account - the only account games can reach
          </span>
          <select
            value={draft.account}
            onChange={(e) => setDraft({ ...draft, account: e.target.value })}
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          >
            <option value="">No account bound</option>
            {accounts.map((a) => (
              <option key={a.accountNumber} value={a.accountName}>
                {a.accountName} ({fmtDcr(a.spendableBalance)} spendable)
              </option>
            ))}
          </select>
          <span className="text-muted-foreground block">
            Keep this separate from your main account. Only what you move into it is ever at stake.
          </span>
        </label>

        <div className="text-xs space-y-1">
          <span className="text-muted-foreground block">Buy-ins</span>
          <p className="text-muted-foreground">
            Every buy-in asks you, with your wallet passphrase. There is no setting that pays
            automatically - this dashboard never holds the passphrase.
          </p>
        </div>

        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Approval wait (seconds)</span>
          <input
            type="number"
            min={10}
            max={600}
            value={draft.approvalTimeoutSecs}
            onChange={(e) =>
              setDraft({ ...draft, approvalTimeoutSecs: Number(e.target.value) || 0 })
            }
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          />
        </label>

        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Max buy-in per table (DCR)</span>
          <input
            type="number"
            min={0}
            step="0.001"
            value={draft.perTableCapDcr}
            onChange={(e) => setDraft({ ...draft, perTableCapDcr: Number(e.target.value) || 0 })}
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          />
        </label>

        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Max staked per day (DCR)</span>
          <input
            type="number"
            min={0}
            step="0.001"
            value={draft.perDayCapDcr}
            onChange={(e) => setDraft({ ...draft, perDayCapDcr: Number(e.target.value) || 0 })}
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          />
        </label>

      </div>

      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => apply(draft)}
          disabled={busy}
          className="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/20 text-primary hover:bg-primary/30 disabled:opacity-50 disabled:cursor-wait"
        >
          {busy ? 'Saving...' : 'Save policy'}
        </button>
        <button
          type="button"
          onClick={() => setDraft(settings)}
          disabled={busy}
          className="px-3 py-1.5 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 disabled:opacity-50"
        >
          Discard changes
        </button>
      </div>

      <div className="space-y-2">
        <h3 className="font-medium text-sm">Games</h3>
        <p className="text-xs text-muted-foreground">
          A game is a separate program you run yourself. Register it by the id it uses on Bison
          Relay - <span className="font-mono">poker</span> for dcrpoker - and this installation
          recognises its traffic. Anything from a game you have not registered is ignored rather
          than shown.
        </p>

        <div className="flex flex-wrap items-end gap-2 p-3 rounded-lg bg-muted/10 border border-border/50">
          <label className="text-xs space-y-1 flex-1 min-w-32">
            <span className="text-muted-foreground block">Game id</span>
            <input
              value={newGame}
              onChange={(e) => setNewGame(e.target.value)}
              placeholder="poker"
              className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm font-mono"
            />
          </label>
          <label className="text-xs space-y-1 flex-1 min-w-32">
            <span className="text-muted-foreground block">Name (optional)</span>
            <input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="Poker"
              className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
            />
          </label>
          <button
            type="button"
            onClick={registerGame}
            disabled={busy || !newGame.trim()}
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/20 text-primary hover:bg-primary/30 disabled:opacity-50 disabled:cursor-wait"
          >
            Register
          </button>
        </div>

        {games.length === 0 && (
          <div className="p-3 rounded-lg bg-muted/10 border border-border/50 text-sm text-muted-foreground">
            No games registered.
          </div>
        )}
        {games.map((g) => (
          <div
            key={g.id}
            className="flex items-start justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50"
          >
            <div className="min-w-0 space-y-1">
              <span className="font-medium block">
                {g.name}
                {g.name !== g.id && (
                  <span className="text-muted-foreground font-normal font-mono text-xs"> {g.id}</span>
                )}
              </span>
              <span className="text-xs text-muted-foreground block">
                {g.ready ? 'Connected.' : 'Registered. Nothing has connected under this id yet.'}
              </span>
              {settings.gameTokens?.[g.id] && (
                <div className="text-xs space-y-1 pt-1">
                  <span className="text-muted-foreground block">
                    Connection token - paste this into the game. It is the game's identity, not a
                    password; removing the game revokes it.
                  </span>
                  <span className="font-mono text-xs break-all block">
                    {settings.gameTokens[g.id]}
                  </span>
                </div>
              )}
            </div>
            <button
              type="button"
              onClick={() => toggleGame(g.id)}
              disabled={busy}
              className="shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 disabled:opacity-50 disabled:cursor-wait"
            >
              Remove
            </button>
          </div>
        ))}
      </div>

      <GamingSpendApprovals />
    </div>
  );
};
