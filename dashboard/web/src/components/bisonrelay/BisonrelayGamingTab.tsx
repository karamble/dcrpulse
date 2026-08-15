// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { GamingSpendApprovals } from './GamingSpendApprovals';
import {
  AlertCircle,
  Gamepad2,
  HandCoins,
  History,
  LayoutGrid,
  Loader2,
  Radio,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react';
import { dataOf, useAsyncResource } from '../../hooks/useAsyncResource';
import { useActionMap } from '../../hooks/useActionMap';
import { useGamingSpends } from '../../hooks/useGamingSpends';
import { BrSidebar, navigateTo, type BrSidebarItem } from './BrSidebar';
import { ConfirmActionModal } from './BisonrelayUserSubNav';
import { GamingTablesCard } from './GamingTablesCard';
import { GamingCredentialModal } from './GamingCredentialModal';
import { useGamingStates } from './useGamingStates';
import { formatAtomsTrimmed } from '../../utils/amounts';
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
import { GamingCreateTable } from './GamingCreateTable';
import { GamingLockedCoin } from './GamingLockedCoin';
import { CapValue, GamingCapField, capError, capFromDcr, capToDcr } from './GamingCapField';

// blankPolicy is what an unedited card starts from. The server mints the real
// defaults on registration; this only keeps the inputs controlled until it
// answers.
// PolicyDraft is what somebody has typed and not yet saved. Caps are held as
// the field's own state rather than as numbers, because a cap of zero means no
// cap at all to the server and an empty box must never be able to produce one.
interface PolicyDraft {
  account?: string;
  approvalTimeoutSecs?: number;
  perTable?: CapValue;
  perDay?: CapValue;
}

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
type GamingSection = 'approvals' | 'games' | 'tables' | 'bridge' | 'history';

// The rail's sections are deep-linkable, matching the Settings and Stats tabs
// beside this one. Approvals is the landing section rather than a place to
// navigate to: it is the only surface here with a deadline.
const readHashSection = (): GamingSection => {
  const h = window.location.hash.replace(/^#/, '');
  const part = h.split('/')[1] ?? '';
  if (part === 'games' || part === 'tables' || part === 'bridge' || part === 'history') {
    return part;
  }
  return 'approvals';
};

const railItems: BrSidebarItem[] = [
  { id: 'approvals', label: 'Approvals', hash: 'gaming', icon: HandCoins },
  { id: 'games', label: 'Games', hash: 'gaming/games', icon: Gamepad2 },
  { id: 'tables', label: 'Tables', hash: 'gaming/tables', icon: LayoutGrid },
  { id: 'bridge', label: 'Bridge', hash: 'gaming/bridge', icon: Radio },
  { id: 'history', label: 'History', hash: 'gaming/history', icon: History },
];

export const BisonrelayGamingTab = () => {
  const [section, setSection] = useState<GamingSection>(readHashSection);
  useEffect(() => {
    const onHash = () => setSection(readHashSection());
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  // Four independent reads, so one failing says nothing about the other three.
  // They used to be four sequential awaits behind one try, three of which
  // swallowed their error - which is how a wallet that had not finished
  // starting made a bound account render as "no account bound".
  const settingsRes = useAsyncResource(getGamingSettings, 'Could not load the gaming policy');
  const gamesRes = useAsyncResource(getGamingGames, 'Could not read the registered games');
  const accountsRes = useAsyncResource(getAccounts, 'Could not read the wallet accounts');
  const bridgeRes = useAsyncResource(getGamingBridgeInfo, 'Could not reach the gaming bridge');

  const settings = dataOf(settingsRes.state) ?? null;
  const games: GamingGame[] = dataOf(gamesRes.state) ?? [];
  const accounts: AccountInfo[] = dataOf(accountsRes.state) ?? [];
  const bridge: GamingBridgeInfo | null = dataOf(bridgeRes.state) ?? null;

  // draft is a sparse patch per game, never a clone of the whole settings
  // object. An unedited field has no entry, so a reload cannot overwrite an
  // edit and no action can carry another game's half-typed caps along with it.
  const [draft, setDraft] = useState<Record<string, PolicyDraft>>({});
  const [newGame, setNewGame] = useState('');
  const [newName, setNewName] = useState('');
  const acts = useActionMap<void>();
  // panelErr is a failure that belongs to the whole tab rather than to one
  // control. Everything else is recorded against the thing that was pressed.
  const [panelErr, setPanelErr] = useState<string | null>(null);
  // issued is held only while the panel is open. The private key in it is not
  // stored anywhere else, here or on the appliance, so closing the panel is the
  // last time anybody sees it.
  const [issued, setIssued] = useState<GamingCredentialMaterial | null>(null);
  const [newTable, setNewTable] = useState<{ id: string; label: string } | null>(null);
  const [confirming, setConfirming] = useState<{
    kind: 'remove' | 'replace';
    id: string;
    name: string;
  } | null>(null);

  const feed = useGamingSpends();
  const gameStates = useGamingStates(games);
  // The soonest deadline is the one worth putting in front of somebody.
  const soonest = feed.pending[0] ?? null;
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!soonest) return;
    const t = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, [soonest]);
  const waitLeft = soonest
    ? (() => {
        const left = soonest.expiresAt - (Math.floor(Date.now() / 1000) + feed.offset);
        if (left <= 0) return 'lapsed';
        return `${Math.floor(left / 60)}:${String(Math.max(0, left % 60)).padStart(2, '0')}`;
      })()
    : null;

  const stored = (id: string): GamePolicy => settings?.policies[id] ?? blankPolicy();
  const capOf = (id: string, which: 'perTable' | 'perDay'): CapValue =>
    draft[id]?.[which] ??
    capFromDcr(which === 'perTable' ? stored(id).perTableCapDcr : stored(id).perDayCapDcr);

  const effective = (id: string): GamePolicy => ({
    ...stored(id),
    account: draft[id]?.account ?? stored(id).account,
    approvalTimeoutSecs: draft[id]?.approvalTimeoutSecs ?? stored(id).approvalTimeoutSecs,
    perTableCapDcr: capToDcr(capOf(id, 'perTable')),
    perDayCapDcr: capToDcr(capOf(id, 'perDay')),
  });

  const dirty = (id: string) => Object.keys(draft[id] ?? {}).length > 0;
  // A cap that has been chosen as a limit but left blank is not saveable, and
  // saying so beats writing a zero that the server reads as no limit at all.
  const capProblem = (id: string) =>
    capError(capOf(id, 'perTable')) ?? capError(capOf(id, 'perDay'));

  // Every write merges over what is stored and overlays one key, so the base is
  // the same whichever button was pressed. That is what stops the bridge toggle
  // discarding a cap somebody was halfway through typing.
  const apply = async (key: string, next: GamingSettings, onOk?: () => void) => {
    setPanelErr(null);
    await acts.run(
      key,
      async () => {
        await setGamingSettings(next);
        await settingsRes.refresh();
        await gamesRes.refresh();
        onOk?.();
      },
      { fallback: 'Save failed' },
    );
  };

  // Registering and removing save against what is stored, not the draft: they
  // are their own act, and carrying half-typed caps along with them would
  // commit edits nobody pressed Save for.
  // The inputs are cleared only once the server has taken the id. Clearing
  // them first threw away what somebody typed every time the id was refused,
  // leaving them an error and an empty box to retype from.
  const registerGame = () => {
    if (!settings) return;
    const id = newGame.trim().toLowerCase();
    if (!id) return;
    const name = newName.trim();
    void apply(
      'settings:register',
      {
        ...settings,
        registeredGames: [...settings.registeredGames, id],
        // A policy the server has never seen mints the defaults; naming it
        // here only carries the label the operator just typed.
        policies: name
          ? { ...settings.policies, [id]: { ...(settings.policies[id] ?? blankPolicy()), name } }
          : settings.policies,
      },
      () => {
        setNewGame('');
        setNewName('');
      },
    );
  };

  // Issuing and revoking are their own acts against what is stored, like
  // registering and removing. A credential cannot travel through the settings
  // save in any case: the private key is answered once and never read back.
  const issueCredential = (id: string) =>
    acts.run(
      `game:${id}:issue`,
      async () => {
        setIssued(await issueGamingCredential(id));
        await settingsRes.refresh();
      },
      { fallback: 'Could not issue a credential' },
    );

  const revokeCredential = (id: string) =>
    acts.run(
      `game:${id}:revoke`,
      async () => {
        await revokeGamingCredential(id);
        await settingsRes.refresh();
        await gamesRes.refresh();
      },
      { fallback: 'Could not revoke' },
    );

  const toggleGame = (id: string) => {
    if (!settings) return;
    void apply(`game:${id}:remove`, {
      ...settings,
      registeredGames: settings.registeredGames.filter((g) => g !== id),
    });
  };

  const saveGame = (id: string) => {
    if (!settings) return;
    void apply(`game:${id}:save`, {
      ...settings,
      policies: { ...settings.policies, [id]: effective(id) },
    });
  };

  // setPolicy edits one game's draft patch, leaving every other game's alone.
  const setPolicy = (id: string, patch: PolicyDraft) =>
    setDraft((d) => ({ ...d, [id]: { ...(d[id] ?? {}), ...patch } }));

  const revertGame = (id: string) =>
    setDraft((d) => {
      const next = { ...d };
      delete next[id];
      return next;
    });

  // Everything that writes settings sends the whole object, so two in flight
  // means the loser silently overwrites the winner. Those exclude each other;
  // nothing else on the page does.
  const settingsWriteRunning = acts.runningKeys.some(
    (k) =>
      k === 'settings:enabled' || k === 'settings:register' || /^game:.+:(save|remove)$/.test(k),
  );

  if (settingsRes.state.status === 'error') {
    return (
      <div className="space-y-3 p-6">
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">
            The gaming policy could not be read: {settingsRes.state.error}
          </span>
        </div>
        <button
          type="button"
          onClick={() => void settingsRes.refresh()}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-muted/30 border border-border text-foreground text-xs font-semibold hover:bg-muted/50"
        >
          <RefreshCw className="h-3.5 w-3.5" />
          Try again
        </button>
      </div>
    );
  }

  if (!settings) {
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
          policy, and each can only ever touch the one account you bind for it.
        </p>
      </div>

      {/* A request lapses in about two minutes, so it follows the operator
       * between sections rather than waiting to be found in one of them. */}
      {soonest && (
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between p-2 rounded-lg bg-warning/10 border border-warning/30">
          <span className="text-xs text-warning">
            {feed.pending.length === 1
              ? `${soonest.game} is asking for ${formatAtomsTrimmed(soonest.amountAtoms)} DCR`
              : `${feed.pending.length} requests waiting`}
            {waitLeft !== null && ` - ${waitLeft} left`}
          </span>
          {section !== 'approvals' && (
            <button
              type="button"
              onClick={() => navigateTo('gaming')}
              className="shrink-0 px-3 py-1.5 rounded-lg text-xs font-semibold bg-warning/20 text-warning hover:bg-warning/30"
            >
              Answer
            </button>
          )}
        </div>
      )}

      <div className="flex flex-col md:flex-row gap-4">
        <BrSidebar items={railItems} active={section} />
        <div className="flex-1 min-w-0 space-y-4">
          {panelErr && (
            <div className="p-3 rounded-lg bg-destructive/10 border border-destructive/30 text-sm text-destructive break-words">
              {panelErr}
            </div>
          )}
          {settingsRes.state.status === 'stale' && (
            <div className="p-2 rounded-lg bg-muted/20 border border-border/50 text-xs text-muted-foreground break-words">
              The policy shown is the last one this page read successfully. The most recent attempt
              failed: {settingsRes.state.error}
            </div>
          )}

          {section === 'approvals' && (
            <GamingSpendApprovals
              policies={settings.policies}
              bridgeEnabled={settings.enabled}
              gameCount={games.length}
            />
          )}

          {section === 'history' && (
            <GamingSpendApprovals
              mode="history"
              policies={settings.policies}
              bridgeEnabled={settings.enabled}
              gameCount={games.length}
            />
          )}

          {section === 'tables' && (
            <GamingTablesCard
              games={games}
              states={gameStates.states}
              onRefresh={() => void gameStates.refreshAll()}
              refreshing={gameStates.refreshing}
            />
          )}

          {section === 'bridge' && (
            <div className="flex items-center justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
              <div>
                <span className="font-medium block">Gaming bridge</span>
                <span className="text-sm text-muted-foreground block">
                  {settings.enabled
                    ? 'On. Registered games route traffic and stake under their own policies.'
                    : 'Off. Nothing routes and nothing can be staked.'}
                </span>
                <span className="text-xs text-muted-foreground block pt-1">
                  Every buy-in asks you, with your wallet passphrase. There is no setting that pays
                  automatically - this dashboard never holds the passphrase.
                </span>
                {acts.get('settings:enabled').phase === 'failed' && (
                  <span className="text-xs text-destructive block pt-1 break-words">
                    {(acts.get('settings:enabled') as { error: string }).error}
                  </span>
                )}
              </div>
              <button
                type="button"
                onClick={() =>
                  void apply('settings:enabled', { ...settings, enabled: !settings.enabled })
                }
                disabled={settingsWriteRunning}
                className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                  settings.enabled
                    ? 'bg-success/20 text-success hover:bg-success/30'
                    : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
                } disabled:opacity-50 disabled:cursor-not-allowed`}
              >
                {acts.isRunning('settings:enabled')
                  ? settings.enabled
                    ? 'Turning off'
                    : 'Turning on'
                  : settings.enabled
                    ? 'On'
                    : 'Off'}
              </button>
            </div>
          )}

          {section === 'games' && (
            <div className="space-y-2">
              <h3 className="font-medium text-sm">Games</h3>
              <p className="text-xs text-muted-foreground">
                A game is a separate program you run yourself. Register it by the id it uses on
                Bison Relay - <span className="font-mono">poker</span> for dcrpoker - and this
                installation recognises its traffic. Anything from a game you have not registered is
                ignored rather than shown.
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
                  disabled={settingsWriteRunning || !newGame.trim()}
                  className="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/20 text-primary hover:bg-primary/30 disabled:opacity-50 disabled:cursor-wait"
                >
                  {acts.isRunning('settings:register') ? 'Registering' : 'Register'}
                </button>
                {acts.get('settings:register').phase === 'failed' && (
                  <span className="w-full text-xs text-destructive break-words">
                    {(acts.get('settings:register') as { error: string }).error}
                  </span>
                )}
              </div>

              {/* "No games registered" is a claim about what is stored, so it is
               * reachable only from an answer. A failed read says that instead:
               * this list being empty because nobody could be asked is a different
               * thing from it being empty because there is nobody. */}
              {gamesRes.state.status === 'error' ? (
                <div className="space-y-2 p-3 rounded-lg bg-muted/10 border border-border/50">
                  <div className="flex items-start gap-2 text-sm text-destructive">
                    <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                    <span className="break-words">
                      The registered games could not be read: {gamesRes.state.error}
                    </span>
                  </div>
                  <button
                    type="button"
                    onClick={() => void gamesRes.refresh()}
                    className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                  >
                    <RefreshCw className="h-3 w-3" />
                    Try again
                  </button>
                </div>
              ) : (
                games.length === 0 && (
                  <div className="p-3 rounded-lg bg-muted/10 border border-border/50 text-sm text-muted-foreground">
                    No games registered.
                  </div>
                )
              )}
              {games.map((g) => {
                const p = effective(g.id);
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
                          {g.ready
                            ? 'Connected.'
                            : 'Registered. Nothing has connected under this id yet.'}
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
                            {g.ready && (
                              <button
                                type="button"
                                onClick={() => setNewTable({ id: g.id, label: g.name })}
                                className="px-2.5 py-1 rounded-lg text-xs font-medium bg-primary/10 text-primary hover:bg-primary/20"
                              >
                                New table
                              </button>
                            )}
                            <button
                              type="button"
                              onClick={() =>
                                settings.gameCredentials?.[g.id]
                                  ? setConfirming({ kind: 'replace', id: g.id, name: g.name })
                                  : void issueCredential(g.id)
                              }
                              disabled={acts.isRunning(`game:${g.id}:issue`)}
                              className="px-2.5 py-1 rounded-lg text-xs font-medium bg-primary/10 text-primary hover:bg-primary/20 disabled:opacity-50 disabled:cursor-wait"
                            >
                              {acts.isRunning(`game:${g.id}:issue`)
                                ? 'Issuing'
                                : settings.gameCredentials?.[g.id]
                                  ? 'Replace credential'
                                  : 'Generate credential'}
                            </button>
                            {settings.gameCredentials?.[g.id] && (
                              <button
                                type="button"
                                onClick={() => void revokeCredential(g.id)}
                                disabled={acts.isRunning(`game:${g.id}:revoke`)}
                                className="px-2.5 py-1 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 disabled:opacity-50 disabled:cursor-wait"
                              >
                                {acts.isRunning(`game:${g.id}:revoke`) ? 'Revoking' : 'Revoke'}
                              </button>
                            )}
                          </div>
                          {(['issue', 'revoke', 'remove', 'save'] as const).map((verb) => {
                            const a = acts.get(`game:${g.id}:${verb}`);
                            return a.phase === 'failed' ? (
                              <span
                                key={verb}
                                className="text-xs text-destructive block break-words pt-1"
                              >
                                {a.error}
                              </span>
                            ) : null;
                          })}
                        </div>
                      </div>
                      <button
                        type="button"
                        onClick={() => setConfirming({ kind: 'remove', id: g.id, name: g.name })}
                        disabled={settingsWriteRunning}
                        className="shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 disabled:opacity-50 disabled:cursor-wait"
                      >
                        {acts.isRunning(`game:${g.id}:remove`) ? 'Removing' : 'Remove'}
                      </button>
                    </div>

                    <div className="grid gap-3 sm:grid-cols-2">
                      <label className="text-xs space-y-1 sm:col-span-2">
                        <span className="text-muted-foreground block">
                          Account - the only one {g.name} can reach
                        </span>
                        {/* The bound account is always an option, whether or not the
                         * wallet answered. Without this a wallet that had not
                         * finished starting left the select with nothing matching,
                         * so the browser painted "No account bound" over a binding
                         * that was stored - and the warning below it disagreed with
                         * the control beside it. */}
                        <select
                          value={p.account}
                          onChange={(e) => setPolicy(g.id, { account: e.target.value })}
                          disabled={accountsRes.state.status === 'error'}
                          className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm disabled:opacity-60"
                        >
                          <option value="">No account bound</option>
                          {p.account.trim() &&
                            !accounts.some((a) => a.accountName === p.account) && (
                              <option value={p.account}>{p.account}</option>
                            )}
                          {accounts.map((a) => (
                            <option key={a.accountNumber} value={a.accountName}>
                              {a.accountName} ({fmtDcr(a.spendableBalance)} spendable)
                            </option>
                          ))}
                        </select>
                        {accountsRes.state.status === 'error' && (
                          <span className="text-muted-foreground block break-words">
                            The wallet's accounts could not be read, so balances are not shown and
                            this cannot be changed here: {accountsRes.state.error}. What is stored
                            is what is selected.
                          </span>
                        )}
                        {p.account.trim() ? (
                          <span className="text-muted-foreground block">
                            Keep this separate from your main account. Only what you move into it is
                            ever at stake for {g.name}, and what it loses is not drawn from another
                            game's.
                          </span>
                        ) : (
                          <span className="text-warning block">
                            No account bound - {g.name} can stake nothing.
                          </span>
                        )}
                      </label>

                      <GamingCapField
                        label="Max buy-in per table (DCR)"
                        value={capOf(g.id, 'perTable')}
                        onChange={(v) => setPolicy(g.id, { perTable: v })}
                      />

                      <GamingCapField
                        label="Max staked per day (DCR)"
                        value={capOf(g.id, 'perDay')}
                        onChange={(v) => setPolicy(g.id, { perDay: v })}
                      />

                      <label className="text-xs space-y-1">
                        <span className="text-muted-foreground block">Approval wait (seconds)</span>
                        {/* Clamped when the field is left, not while it is being
                         * typed: backspacing through it should not pass through a
                         * zero-second window on the way to a new number. */}
                        <input
                          type="number"
                          min={10}
                          max={600}
                          value={p.approvalTimeoutSecs}
                          onChange={(e) =>
                            setPolicy(g.id, { approvalTimeoutSecs: Number(e.target.value) })
                          }
                          onBlur={(e) => {
                            const n = Number(e.target.value);
                            const ok = Number.isFinite(n) ? Math.min(600, Math.max(10, n)) : 120;
                            setPolicy(g.id, { approvalTimeoutSecs: ok });
                          }}
                          className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
                        />
                      </label>
                    </div>

                    <p className="text-xs text-muted-foreground">
                      {p.account.trim()
                        ? `${g.name} may stake ${
                            p.perTableCapDcr > 0
                              ? `at most ${p.perTableCapDcr} DCR at any one table`
                              : 'any amount at a table'
                          }, ${
                            p.perDayCapDcr > 0
                              ? `and at most ${p.perDayCapDcr} DCR a day`
                              : 'with no daily limit'
                          }, from account "${p.account}". Each buy-in still asks you, and lapses after ${
                            p.approvalTimeoutSecs
                          } seconds.`
                        : `${g.name} can stake nothing until an account is bound.`}
                      {/* The server raises a day cap that is below the table cap, and
                       * it does that after reading zero as "no limit" - so asking
                       * for no daily limit while a per-table cap is set stores the
                       * table cap as the day's. Say what will actually be saved. */}
                      {p.perTableCapDcr > 0 && p.perDayCapDcr === 0 && (
                        <span className="text-warning block pt-1">
                          A daily limit below the per-table cap is raised to match it, and "no
                          limit" counts as below - so this will save as {p.perTableCapDcr} DCR a
                          day, not as unlimited.
                        </span>
                      )}
                    </p>

                    {dirty(g.id) && (
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-xs text-warning">Unsaved changes</span>
                        <button
                          type="button"
                          onClick={() => saveGame(g.id)}
                          disabled={settingsWriteRunning || !!capProblem(g.id)}
                          className="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/20 text-primary hover:bg-primary/30 disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          {acts.isRunning(`game:${g.id}:save`) ? 'Saving' : `Save ${g.name}`}
                        </button>
                        <button
                          type="button"
                          onClick={() => revertGame(g.id)}
                          disabled={acts.isRunning(`game:${g.id}:save`)}
                          className="px-3 py-1.5 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 disabled:opacity-50"
                        >
                          Revert
                        </button>
                        {capProblem(g.id) && (
                          <span className="text-xs text-destructive">{capProblem(g.id)}</span>
                        )}
                      </div>
                    )}

                    <GamingLockedCoin game={g.id} name={g.name} />
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      {issued && (
        <GamingCredentialModal
          issued={issued}
          bridge={bridge}
          bridgeError={bridgeRes.state.status === 'error' ? bridgeRes.state.error : null}
          onClose={() => setIssued(null)}
        />
      )}

      {confirming?.kind === 'remove' && (
        <ConfirmActionModal
          title={`Remove ${confirming.name}?`}
          confirmLabel={`Remove ${confirming.name}`}
          confirmWord={confirming.id}
          tone="destructive"
          onClose={() => setConfirming(null)}
          onConfirm={async () => toggleGame(confirming.id)}
          body={
            <>
              <p>
                Removing {confirming.name} drops its credential, so the machine running it stops
                being admitted at once.
              </p>
              <p>
                It erases the policy too - the account binding
                {stored(confirming.id).account ? ` (${stored(confirming.id).account})` : ''} and
                both caps. Registering the id again does not bring any of it back.
              </p>
              {(() => {
                const st = gameStates.states[confirming.id]?.state;
                const held = (st?.locks ?? []).filter((l) => !l.spent);
                const sum = held.reduce((n, l) => n + l.atoms, 0);
                if (!st) {
                  return (
                    <p className="text-warning">
                      This console could not check what {confirming.name} has locked on the chain.
                      If it holds any, removing the card takes away the only route to it.
                    </p>
                  );
                }
                return sum > 0 ? (
                  <p className="text-warning">
                    {confirming.name} has {formatAtomsTrimmed(sum)} DCR locked on the chain across{' '}
                    {held.length} {held.length === 1 ? 'output' : 'outputs'}. This console reaches
                    that coin only through this game's card, so removing it takes the route away.
                  </p>
                ) : (
                  <p>{confirming.name} has nothing locked on the chain right now.</p>
                );
              })()}
              <p>Not affected: nothing on the chain moves, and the account is untouched.</p>
            </>
          }
        />
      )}

      {confirming?.kind === 'replace' && (
        <ConfirmActionModal
          title={`Replace ${confirming.name}'s credential?`}
          confirmLabel="Replace it"
          tone="destructive"
          onClose={() => setConfirming(null)}
          onConfirm={async () => {
            await issueCredential(confirming.id);
          }}
          body={
            <>
              <p>
                A new credential replaces the old one the moment it is issued, so anything running
                as {confirming.name} is cut off until it is given the new one - including a game
                that is in the middle of a table.
              </p>
              <p>
                The private key is shown once and never again. Have somewhere to put it before you
                continue.
              </p>
            </>
          }
        />
      )}

      {newTable && (
        <GamingCreateTable
          game={newTable.id}
          label={newTable.label}
          onClose={() => setNewTable(null)}
        />
      )}
    </div>
  );
};
