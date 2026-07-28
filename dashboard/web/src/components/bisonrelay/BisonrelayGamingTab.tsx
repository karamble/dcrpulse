// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { GamingIdentityBackup } from './GamingIdentityBackup';
import { GamingSpendApprovals } from './GamingSpendApprovals';
import { useGamePanel } from './GamePanelProvider';
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

// BisonrelayGamingTab configures the bridge that stands between installed games
// and the wallet. Games are untrusted: they run beside a wallet, dcrlnd and a
// Bison Relay identity, so nothing here hands them credentials. They reach one
// account, under caps, through the host.
//
// Account scope is enforced by this policy rather than by the wallet, because
// dcrwallet accounts share one seed and one unlock passphrase. It is a boundary
// above the wallet, never a cryptographic one below it.
export const BisonrelayGamingTab = () => {
  const panel = useGamePanel();
  const [settings, setSettings] = useState<GamingSettings | null>(null);
  const [draft, setDraft] = useState<GamingSettings | null>(null);
  const [games, setGames] = useState<GamingGame[]>([]);
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
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

  const toggleGame = (id: string) => {
    if (!draft) return;
    const has = draft.installedGames.includes(id);
    const installedGames = has
      ? draft.installedGames.filter((g) => g !== id)
      : [...draft.installedGames, id];
    apply({ ...draft, installedGames });
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

        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Stake mode</span>
          <select
            value={draft.mode}
            onChange={(e) => setDraft({ ...draft, mode: e.target.value as 'approval' | 'autopay' })}
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          >
            <option value="approval">Approval: ask me before every buy-in</option>
            <option value="autopay">Auto: stake under the caps without asking</option>
          </select>
        </label>

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

        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Max tables funded at once</span>
          <input
            type="number"
            min={1}
            max={10}
            value={draft.maxOpenTables}
            onChange={(e) => setDraft({ ...draft, maxOpenTables: Number(e.target.value) || 1 })}
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          />
          <span className="text-muted-foreground block">
            Stakes overlap, so this is what bounds your real exposure - not the per-table cap.
          </span>
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
          Adding a game lets this installation recognise its traffic on Bison Relay. Anything from a
          game you have not added is ignored rather than shown.
        </p>
        {games.length === 0 && (
          <div className="p-3 rounded-lg bg-muted/10 border border-border/50 text-sm text-muted-foreground">
            No games available.
          </div>
        )}
        {games.map((g) => (
          <div
            key={g.id}
            className="flex items-start justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50"
          >
            <div className="min-w-0">
              <span className="font-medium block">
                {g.name}
                <span className="text-muted-foreground font-normal text-xs"> v{g.protocolVersion}</span>
              </span>
              <span className="text-sm text-muted-foreground block">{g.description}</span>
              {g.installed && !g.ready && (
                <span className="text-xs text-warning block mt-1">
                  Added, but its service is not running yet.
                </span>
              )}
            </div>
            <button
              type="button"
              onClick={() => toggleGame(g.id)}
              disabled={busy}
              className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                g.installed
                  ? 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
                  : 'bg-primary/20 text-primary hover:bg-primary/30'
              } disabled:opacity-50 disabled:cursor-wait`}
            >
              {g.installed ? 'Remove' : 'Add'}
            </button>
            {/* Installed and ready are different answers: a game that is added
                but not up has nothing listening, and a Play button there would
                send a click nowhere. */}
            {g.installed && g.ready && (
              <button
                type="button"
                onClick={() => { void panel.open(g.id); }}
                disabled={panel.opening}
                className="px-3 py-1.5 rounded-md text-sm font-medium bg-primary/20 text-primary hover:bg-primary/30 disabled:opacity-50 disabled:cursor-wait"
              >
                {panel.opening ? 'Opening...' : 'Play'}
              </button>
            )}
          </div>
        ))}
      </div>

      {/* Only games that are actually up: the seed lives in the sandbox's
          volume and only the game itself can read it. */}
      <GamingIdentityBackup games={games.filter((g) => g.installed && g.ready).map((g) => g.id)} />

      <GamingSpendApprovals />
    </div>
  );
};
