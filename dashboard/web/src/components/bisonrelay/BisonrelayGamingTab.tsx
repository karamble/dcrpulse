// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { GamingSpendApprovals } from './GamingSpendApprovals';
import { Loader2, ShieldCheck } from 'lucide-react';
import {
  GamePolicy,
  GamingBridgeInfo,
  GamingCredentialMaterial,
  GamingGame,
  GamingSettings,
  getGamingBridgeInfo,
  getGamingGames,
  getGamingSettings,
  issueGamingCredential,
  revokeGamingCredential,
  setGamingSettings,
} from '../../services/gamingApi';
import { AccountInfo, getAccounts } from '../../services/api';

// blankPolicy is what an unedited card starts from. The server mints the real
// defaults on registration; this only keeps the inputs controlled until it
// answers.
const blankPolicy = (): GamePolicy => ({
  name: '',
  account: '',
  perTableCapDcr: 0,
  perDayCapDcr: 0,
  approvalTimeoutSecs: 120,
});

const fmtDcr = (v: number) => `${v.toLocaleString(undefined, { maximumFractionDigits: 8 })} DCR`;

const fmtWhen = (unix: number) => (unix ? new Date(unix * 1000).toLocaleDateString() : 'unknown');

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
  // issued is held only while the panel is open. The private key in it is not
  // stored anywhere else, here or on the appliance, so closing the panel is the
  // last time anybody sees it.
  const [issued, setIssued] = useState<GamingCredentialMaterial | null>(null);
  const [bridge, setBridge] = useState<GamingBridgeInfo | null>(null);

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
    try {
      setBridge(await getGamingBridgeInfo());
    } catch {
      /* the listener may not have come up */
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
      registeredGames: [...settings.registeredGames, id],
      // A policy the server has never seen mints the defaults; naming it
      // here only carries the label the operator just typed.
      policies: name
        ? { ...settings.policies, [id]: { ...(settings.policies[id] ?? blankPolicy()), name } }
        : settings.policies,
    });
    setNewGame('');
    setNewName('');
  };

  // Issuing and revoking are their own acts against what is stored, like
  // registering and removing. A credential cannot travel through the settings
  // save in any case: the private key is answered once and never read back.
  const issueCredential = async (id: string) => {
    setBusy(true);
    setError(null);
    try {
      setIssued(await issueGamingCredential(id));
      setSettings(await getGamingSettings());
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Could not issue a credential');
    } finally {
      setBusy(false);
    }
  };

  const revokeCredential = async (id: string) => {
    setBusy(true);
    setError(null);
    try {
      await revokeGamingCredential(id);
      setSettings(await getGamingSettings());
      setGames(await getGamingGames());
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Could not revoke');
    } finally {
      setBusy(false);
    }
  };

  const toggleGame = (id: string) => {
    if (!settings) return;
    apply({
      ...settings,
      registeredGames: settings.registeredGames.filter((g) => g !== id),
    });
  };

  // setPolicy edits one game's draft policy, leaving every other game's alone.
  const setPolicy = (id: string, patch: Partial<GamePolicy>) => {
    setDraft((d) =>
      d ? { ...d, policies: { ...d.policies, [id]: { ...(d.policies[id] ?? blankPolicy()), ...patch } } } : d,
    );
  };

  if (!settings || !draft) {
    return (
      <div className="flex items-center gap-2 p-6 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading gaming policy...
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-medium flex items-center gap-2">
          <ShieldCheck className="h-4 w-4" />
          Gaming
        </h2>
        <p className="text-sm text-muted-foreground">
          Games are separate programs that play over Bison Relay against other people, staking real
          funds. They never hold your wallet keys: everything a game stakes passes through its own
          policy below, and it can only ever touch the one account you bind for it.
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
              ? 'On. Registered games route traffic and stake under their own policies.'
              : 'Off. Nothing routes and nothing can be staked.'}
          </span>
        </div>
        <button
          type="button"
          onClick={() => apply({ ...settings, enabled: !settings.enabled })}
          disabled={busy}
          className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
            settings.enabled
              ? 'bg-success/20 text-success hover:bg-success/30'
              : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
          } disabled:opacity-50 disabled:cursor-not-allowed`}
        >
          {settings.enabled ? 'On' : 'Off'}
        </button>
      </div>

      <p className="text-xs text-muted-foreground">
        Every buy-in asks you, with your wallet passphrase. There is no setting that pays
        automatically - this dashboard never holds the passphrase.
      </p>

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
        {games.map((g) => {
          const p = draft.policies[g.id] ?? {
            name: '',
            account: '',
            perTableCapDcr: 0,
            perDayCapDcr: 0,
            approvalTimeoutSecs: 120,
          };
          return (
            <div
              key={g.id}
              className="space-y-3 p-3 rounded-lg bg-muted/10 border border-border/50"
            >
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0 space-y-1">
                  <span className="font-medium block">
                    {g.name}
                    {g.name !== g.id && (
                      <span className="text-muted-foreground font-normal font-mono text-xs">
                        {' '}
                        {g.id}
                      </span>
                    )}
                  </span>
                  <span className="text-xs text-muted-foreground block">
                    {g.ready ? 'Connected.' : 'Registered. Nothing has connected under this id yet.'}
                  </span>
                  <div className="text-xs space-y-1 pt-1">
                    {settings.gameCredentials?.[g.id] ? (
                      <span className="text-muted-foreground block">
                        Credential issued {fmtWhen(settings.gameCredentials[g.id].issuedAt)}
                        <span className="font-mono break-all">
                          {' '}
                          {settings.gameCredentials[g.id].fingerprint.slice(0, 16)}
                        </span>
                      </span>
                    ) : (
                      <span className="text-muted-foreground block">
                        No credential yet, so nothing can connect as this game.
                      </span>
                    )}
                    <div className="flex flex-wrap gap-2 pt-1">
                      <button
                        type="button"
                        onClick={() => issueCredential(g.id)}
                        disabled={busy}
                        className="px-2.5 py-1 rounded-lg text-xs font-medium bg-primary/10 text-primary hover:bg-primary/20 disabled:opacity-50 disabled:cursor-wait"
                      >
                        {settings.gameCredentials?.[g.id] ? 'Regenerate' : 'Generate credential'}
                      </button>
                      {settings.gameCredentials?.[g.id] && (
                        <button
                          type="button"
                          onClick={() => revokeCredential(g.id)}
                          disabled={busy}
                          className="px-2.5 py-1 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 disabled:opacity-50 disabled:cursor-wait"
                        >
                          Revoke
                        </button>
                      )}
                    </div>
                  </div>
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

              <div className="grid gap-3 sm:grid-cols-2">
                <label className="text-xs space-y-1 sm:col-span-2">
                  <span className="text-muted-foreground block">
                    Account - the only one {g.name} can reach
                  </span>
                  <select
                    value={p.account}
                    onChange={(e) => setPolicy(g.id, { account: e.target.value })}
                    className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
                  >
                    <option value="">No account bound</option>
                    {accounts.map((a) => (
                      <option key={a.accountNumber} value={a.accountName}>
                        {a.accountName} ({fmtDcr(a.spendableBalance)} spendable)
                      </option>
                    ))}
                  </select>
                  {p.account.trim() ? (
                    <span className="text-muted-foreground block">
                      Keep this separate from your main account. Only what you move into it is ever
                      at stake for {g.name}, and what it loses is not drawn from another game's.
                    </span>
                  ) : (
                    <span className="text-warning block">
                      No account bound - {g.name} can stake nothing.
                    </span>
                  )}
                </label>

                <label className="text-xs space-y-1">
                  <span className="text-muted-foreground block">Max buy-in per table (DCR)</span>
                  <input
                    type="number"
                    min={0}
                    step="0.001"
                    value={p.perTableCapDcr}
                    onChange={(e) =>
                      setPolicy(g.id, { perTableCapDcr: Number(e.target.value) || 0 })
                    }
                    className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
                  />
                </label>

                <label className="text-xs space-y-1">
                  <span className="text-muted-foreground block">Max staked per day (DCR)</span>
                  <input
                    type="number"
                    min={0}
                    step="0.001"
                    value={p.perDayCapDcr}
                    onChange={(e) => setPolicy(g.id, { perDayCapDcr: Number(e.target.value) || 0 })}
                    className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
                  />
                </label>

                <label className="text-xs space-y-1">
                  <span className="text-muted-foreground block">Approval wait (seconds)</span>
                  <input
                    type="number"
                    min={10}
                    max={600}
                    value={p.approvalTimeoutSecs}
                    onChange={(e) =>
                      setPolicy(g.id, { approvalTimeoutSecs: Number(e.target.value) || 0 })
                    }
                    className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
                  />
                </label>
              </div>
            </div>
          );
        })}
      </div>

      {issued && (
        <div className="space-y-3 p-4 rounded-lg bg-primary/5 border border-primary/30">
          <div className="space-y-1">
            <span className="font-medium block">Credential for {issued.game}</span>
            <span className="text-xs text-muted-foreground block">
              Copy all three into the game's connection wizard now. The private key is not stored
              here and is not shown again - if you lose it, generate another, which replaces this
              one.
            </span>
          </div>

          <div className="text-xs space-y-1">
            <span className="text-muted-foreground block">
              Port {bridge?.port || '8443'}. The address is whatever this machine is reachable at
              from wherever you run the game.
            </span>
          </div>

          {[
            ['Client certificate', issued.certPem],
            ['Client private key', issued.keyPem],
            ['Bridge certificate to pin', issued.bridgeCertPem],
          ].map(([label, value]) => (
            <label key={label} className="text-xs space-y-1 block">
              <span className="text-muted-foreground block">{label}</span>
              <textarea
                readOnly
                value={value}
                rows={5}
                onFocus={(e) => e.currentTarget.select()}
                className="w-full px-2 py-1.5 rounded-lg bg-background border border-border font-mono text-[11px]"
              />
            </label>
          ))}

          <button
            type="button"
            onClick={() => setIssued(null)}
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30"
          >
            I have copied it
          </button>
        </div>
      )}

      <GamingSpendApprovals />
    </div>
  );
};
