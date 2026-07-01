// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType, ReactNode, useCallback, useEffect, useRef, useState } from 'react';
import {
  ALargeSmall,
  AlertCircle,
  Bell,
  Camera,
  CheckCircle2,
  Download,
  Filter,
  Gauge,
  Info,
  KeyRound,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  RotateCw,
  Rss,
  SlidersHorizontal,
  Trash2,
  User,
  Wifi,
  X,
} from 'lucide-react';
import {
  BisonrelayBackupStatus,
  BisonrelayConnectionState,
  BisonrelayContact,
  BisonrelayContentFilter,
  BisonrelayGC,
  BisonrelayKXAttempt,
  BisonrelayKXSearch,
  BisonrelayMediateID,
  BisonrelayVersion,
  BRBehavior,
  cancelBisonrelayMediateID,
  deleteBisonrelayFilter,
  getBisonrelayBackupStatus,
  getBisonrelayConnection,
  getBisonrelayContacts,
  getBisonrelayFilters,
  getBisonrelayIdentity,
  getBisonrelayKXList,
  getBisonrelayKXSearches,
  getBisonrelayMediateIDs,
  getBisonrelayBehaviorSettings,
  getBisonrelayRates,
  getBisonrelayVersion,
  listBisonrelayGCs,
  prepareBisonrelayBackup,
  resetAllBisonrelaySessions,
  setBisonrelayAvatar,
  setBisonrelayBehaviorSettings,
  setBisonrelayConnection,
  subscribeAllBisonrelayPosts,
  upsertBisonrelayFilter,
} from '../../services/bisonrelayApi';
import {
  PolicyTile,
  SectionCard,
  backupBtnCls,
  formatBytes,
  formatDCR,
  relativeTime,
} from './BisonrelayStats';
import { avatarDataUrl } from './bisonrelayAvatar';
import { BR_TEXT_SCALES, BrTextScale, setBrTextScale, useBrTextScale } from './brTextScale';
import { ChannelList } from '../lightning/channels/ChannelList';
import { RequestLiquidityModal } from '../lightning/channels/RequestLiquidityModal';
import { setBrNotifPrefs, useBrNotifPrefs } from './brNotifPrefs';

// ---- Section routing --------------------------------------------------------

type SettingsSection =
  | 'account'
  | 'appearance'
  | 'notifications'
  | 'sessions'
  | 'connection'
  | 'behavior'
  | 'advanced'
  | 'filters'
  | 'backup'
  | 'about';

const readHashSection = (): SettingsSection => {
  const h = window.location.hash.replace(/^#/, '');
  if (!h.startsWith('settings')) return 'account';
  const rest = h.slice('settings'.length);
  if (rest === '/appearance') return 'appearance';
  if (rest === '/notifications') return 'notifications';
  if (rest === '/sessions') return 'sessions';
  if (rest === '/connection') return 'connection';
  if (rest === '/behavior') return 'behavior';
  if (rest === '/advanced') return 'advanced';
  if (rest === '/filters') return 'filters';
  if (rest === '/backup') return 'backup';
  if (rest === '/about') return 'about';
  return 'account';
};

const navigateTo = (hash: string): void => {
  window.location.hash = hash;
};

// ---- Account: avatar ------------------------------------------------------

// AvatarControl lets the user upload/replace or clear their avatar. The
// avatar lives on the BR identity, so it is fetched separately. BR caps the
// raw image at 200 KiB and broadcasts the update to all contacts.
const AVATAR_MAX_BYTES = 200 * 1024;

const fileToBase64 = (file: File): Promise<string> =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const s = String(reader.result);
      const comma = s.indexOf(',');
      resolve(comma >= 0 ? s.slice(comma + 1) : s);
    };
    reader.onerror = () => reject(new Error('could not read file'));
    reader.readAsDataURL(file);
  });

const AvatarControl = ({ nick }: { nick: string }) => {
  const [avatar, setAvatar] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    let alive = true;
    getBisonrelayIdentity()
      .then((id) => {
        if (alive) setAvatar(id.avatar || '');
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  const onPick = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    if (file.size > AVATAR_MAX_BYTES) {
      setErr('Image must be 200 KiB or smaller.');
      return;
    }
    setErr(null);
    setBusy(true);
    try {
      const b64 = await fileToBase64(file);
      await setBisonrelayAvatar(b64);
      setAvatar(b64);
    } catch (e2: any) {
      setErr(e2?.response?.data || e2?.message || 'Could not update avatar.');
    } finally {
      setBusy(false);
    }
  };

  const onClear = async () => {
    setErr(null);
    setBusy(true);
    try {
      await setBisonrelayAvatar('');
      setAvatar('');
    } catch (e2: any) {
      setErr(e2?.response?.data || e2?.message || 'Could not clear avatar.');
    } finally {
      setBusy(false);
    }
  };

  const url = avatarDataUrl(avatar);
  return (
    <div className="flex items-start gap-4">
      <div className="relative shrink-0">
        <input
          ref={fileRef}
          type="file"
          accept="image/png,image/jpeg,image/gif,image/webp"
          className="hidden"
          onChange={onPick}
        />
        <button
          type="button"
          onClick={() => fileRef.current?.click()}
          disabled={busy}
          aria-label="Change avatar"
          title="Change avatar (PNG/JPEG/GIF/WebP, max 200 KiB)"
          className="group relative h-14 w-14 rounded-full overflow-hidden border border-primary/30 bg-gradient-to-br from-primary/30 to-primary/10 flex items-center justify-center"
        >
          {url ? (
            <img src={url} alt="Your avatar" className="h-full w-full object-cover" />
          ) : (
            <span className="text-lg font-semibold text-primary">
              {(nick || '?').slice(0, 2).toUpperCase()}
            </span>
          )}
          <span className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
            {busy ? (
              <Loader2 className="h-4 w-4 text-white animate-spin" />
            ) : (
              <Camera className="h-4 w-4 text-white" />
            )}
          </span>
        </button>
        {url && !busy && (
          <button
            type="button"
            onClick={onClear}
            aria-label="Remove avatar"
            title="Remove avatar"
            className="absolute -top-1 -right-1 h-5 w-5 rounded-full bg-destructive text-destructive-foreground flex items-center justify-center border border-background shadow"
          >
            <X className="h-3 w-3" />
          </button>
        )}
      </div>
      <div className="min-w-0">
        <p className="text-sm font-medium">Avatar</p>
        <p className="text-xs text-muted-foreground mt-1">
          Click the avatar to upload a new image (PNG/JPEG/GIF/WebP, max 200
          KiB). The change is broadcast to all contacts.
        </p>
        {err && <p className="text-xs text-destructive mt-1">{err}</p>}
      </div>
    </div>
  );
};

const AccountCard = () => {
  const [nick, setNick] = useState('');
  const [subBusy, setSubBusy] = useState(false);
  const [subResult, setSubResult] = useState<string | null>(null);
  const [subErr, setSubErr] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    getBisonrelayIdentity()
      .then((id) => {
        if (alive) setNick(id.nick || '');
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  const onSubscribeAll = async () => {
    if (subBusy) return;
    setSubBusy(true);
    setSubErr(null);
    setSubResult(null);
    try {
      await subscribeAllBisonrelayPosts();
      setSubResult('Post subscriptions requested for all contacts.');
    } catch (e: any) {
      const body = e?.response?.data;
      setSubErr(typeof body === 'string' ? body : e?.message || 'Subscribe failed');
    } finally {
      setSubBusy(false);
    }
  };

  return (
    <SectionCard title="Account" icon={User}>
      <AvatarControl nick={nick} />
      <div className="flex items-start justify-between gap-4 pt-3 border-t border-border/50">
        <div className="min-w-0">
          <p className="text-sm font-medium">Subscribe to all posts</p>
          <p className="text-xs text-muted-foreground mt-1">
            Requests a post subscription from every contact so their future
            posts show up in your feed. Runs through the send queue; with
            many contacts it can take a moment.
          </p>
          {subResult && (
            <p className="text-xs text-emerald-400 mt-1 flex items-center gap-1.5">
              <CheckCircle2 className="h-3.5 w-3.5" /> {subResult}
            </p>
          )}
          {subErr && <p className="text-xs text-destructive mt-1">{subErr}</p>}
        </div>
        <button
          type="button"
          onClick={onSubscribeAll}
          disabled={subBusy}
          className={backupBtnCls}
        >
          {subBusy ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Rss className="h-3.5 w-3.5" />
          )}
          Subscribe all
        </button>
      </div>
    </SectionCard>
  );
};

// ---- Appearance -------------------------------------------------------------

const AppearanceCard = () => {
  const { scale } = useBrTextScale();

  return (
    <SectionCard title="Appearance" icon={ALargeSmall}>
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="text-sm font-medium">Font size</p>
          <p className="text-xs text-muted-foreground mt-1">
            Scales the text of the Bison Relay section only - the rest of the
            dashboard is unaffected. Stored in this browser.
          </p>
        </div>
        <select
          value={scale}
          onChange={(e) => setBrTextScale(e.target.value as BrTextScale)}
          className="shrink-0 rounded-lg bg-background/60 border border-border px-2 py-1.5 text-xs text-foreground focus:outline-none"
        >
          {BR_TEXT_SCALES.map((s) => (
            <option key={s.id} value={s.id}>
              {s.label}
            </option>
          ))}
        </select>
      </div>
      <p className="text-xs text-muted-foreground">
        The quick brown fox jumps over the lazy dog. 0123456789
      </p>
    </SectionCard>
  );
};

// ---- Notifications ----------------------------------------------------------

// Toggle mirrors the wallet Settings > Privacy & Security switch styling.
// PendingBadge marks a setting whose saved value differs from the value the
// running Bison Relay daemon booted with, so it applies only after a restart.
const PendingBadge = ({ show }: { show: boolean }) =>
  show ? (
    <span className="ml-2 shrink-0 rounded-full bg-amber-500/15 px-2 py-0.5 text-[10px] font-medium text-amber-400 align-middle">
      pending restart
    </span>
  ) : null;

const Toggle = ({
  label,
  description,
  checked,
  onChange,
  pending,
}: {
  label: string;
  description: string;
  checked: boolean;
  onChange: (next: boolean) => void;
  pending?: boolean;
}) => (
  <div className="flex items-start justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
    <div>
      <span className="font-medium block">
        {label}
        <PendingBadge show={!!pending} />
      </span>
      <span className="text-sm text-muted-foreground block">{description}</span>
    </div>
    <button
      type="button"
      onClick={() => onChange(!checked)}
      className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
        checked
          ? 'bg-success/20 text-success hover:bg-success/30'
          : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
      }`}
    >
      {checked ? 'On' : 'Off'}
    </button>
  </div>
);

const NotificationsCard = () => {
  const prefs = useBrNotifPrefs();

  return (
    <SectionCard title="Notifications" icon={Bell}>
      <p className="text-xs text-muted-foreground">
        Controls the in-app notification indicators of the Bison Relay
        section. Messages keep arriving either way - switching a type off
        only hides its unread markers. Stored in this browser.
      </p>
      <div className="space-y-2">
        <Toggle
          label="Direct messages"
          description="Show unread bubbles for private messages in the chat list and on the navigation badge."
          checked={prefs.dms}
          onChange={(v) => setBrNotifPrefs({ ...prefs, dms: v })}
        />
        <Toggle
          label="Group chat messages"
          description="Show unread bubbles for group chats in the chat list and the navigation dot."
          checked={prefs.gcMessages}
          onChange={(v) => setBrNotifPrefs({ ...prefs, gcMessages: v })}
        />
        <Toggle
          label="Feed posts"
          description="Mark posts with new activity in the feed."
          checked={prefs.feedPosts}
          onChange={(v) => setBrNotifPrefs({ ...prefs, feedPosts: v })}
        />
      </div>
    </SectionCard>
  );
};

// ---- Sessions (KX resets) -------------------------------------------------

// SessionsCard initiates a KX (ratchet) reset with contacts. Needed when the
// local key state diverged from what the network last saw - typically after
// restoring a backup older than the node's last session. Initiation only:
// each reset completes in the background whenever that contact comes online.
const SessionsCard = () => {
  const [busy, setBusy] = useState<'all' | 'stale' | null>(null);
  const [result, setResult] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const onReset = async (kind: 'all' | 'stale') => {
    if (busy) return;
    setBusy(kind);
    setErr(null);
    setResult(null);
    try {
      const res = await resetAllBisonrelaySessions(kind === 'stale' ? 30 : 0);
      setResult(
        `KX reset initiated with ${res.count} contact${res.count === 1 ? '' : 's'}. ` +
          'Each reset completes in the background once that contact comes online.',
      );
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Reset failed');
    } finally {
      setBusy(null);
    }
  };

  return (
    <SectionCard
      title="Sessions"
      icon={RotateCw}
      action={
        <button
          type="button"
          onClick={() => onReset('all')}
          disabled={busy !== null}
          className={backupBtnCls}
        >
          {busy === 'all' ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <RotateCw className="h-3.5 w-3.5" />
          )}
          {busy === 'all' ? 'Resetting...' : 'Reset all sessions'}
        </button>
      }
    >
      <p className="text-xs text-muted-foreground">
        Re-keys the end-to-end encrypted session with contacts. After
        restoring a backup this happens automatically; use it manually if
        messages with some contacts stop flowing in either direction.
        Individual contacts can be reset from their profile menu instead.
      </p>
      <div className="flex items-center justify-between gap-4">
        <p className="text-xs text-muted-foreground">
          Only re-key contacts with no message received for 30+ days:
        </p>
        <button
          type="button"
          onClick={() => onReset('stale')}
          disabled={busy !== null}
          className={backupBtnCls}
        >
          {busy === 'stale' ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <RotateCw className="h-3.5 w-3.5" />
          )}
          Reset stale sessions
        </button>
      </div>
      {result && (
        <div className="flex items-start gap-2 text-xs">
          <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400 shrink-0 mt-0.5" />
          <span className="text-muted-foreground">{result}</span>
        </div>
      )}
      {err && (
        <div className="flex items-start gap-2 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 shrink-0 mt-0.5" />
          <span className="break-all">{err}</span>
        </div>
      )}
    </SectionCard>
  );
};

// ---- Key exchange diagnostics ----------------------------------------------

const KXListCard = () => {
  const [kxs, setKxs] = useState<BisonrelayKXAttempt[] | null>(null);
  const [mediations, setMediations] = useState<BisonrelayMediateID[]>([]);
  const [searches, setSearches] = useState<BisonrelayKXSearch[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setBusy(true);
    try {
      setKxs(await getBisonrelayKXList());
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load key exchanges');
    } finally {
      setBusy(false);
    }
    // Best-effort: older daemons lack these endpoints.
    getBisonrelayMediateIDs().then(setMediations).catch(() => {});
    getBisonrelayKXSearches().then(setSearches).catch(() => {});
  }, []);

  const cancelMediation = async (mediator: string, target: string) => {
    try {
      await cancelBisonrelayMediateID(mediator, target);
      setMediations((prev) =>
        prev.filter((m) => !(m.mediator === mediator && m.target === target)),
      );
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Cancel failed');
    }
  };

  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <SectionCard
      title="Key exchanges"
      icon={KeyRound}
      action={
        <button type="button" onClick={refresh} disabled={busy} className={backupBtnCls}>
          {busy ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <RefreshCw className="h-3.5 w-3.5" />
          )}
          Refresh
        </button>
      }
    >
      <p className="text-xs text-muted-foreground">
        Key exchanges still in flight (new contacts and session resets). An
        entry disappears once the other side completes its part.
      </p>
      {err && <p className="text-xs text-destructive">{err}</p>}
      {kxs && kxs.length === 0 && (
        <p className="text-xs text-muted-foreground">No key exchanges in progress.</p>
      )}
      {kxs && kxs.length > 0 && (
        <div className="space-y-1.5">
          {kxs.map((kx) => (
            <div
              key={kx.initial_rv}
              className="flex flex-wrap items-center gap-2 rounded-lg bg-muted/10 border border-border/50 px-3 py-2 text-xs"
            >
              <span className="font-medium">
                {kx.peer_nick || (kx.peer_id ? `${kx.peer_id.slice(0, 12)}...` : '(incoming)')}
              </span>
              <span className="px-1.5 py-0.5 rounded bg-muted/30 text-muted-foreground">
                {kx.stage}
              </span>
              {kx.is_for_reset && (
                <span className="px-1.5 py-0.5 rounded bg-warning/15 text-warning">reset</span>
              )}
              <span className="ml-auto text-muted-foreground">
                {relativeTime(new Date(kx.timestamp * 1000).toISOString())}
              </span>
            </div>
          ))}
        </div>
      )}
      {mediations.length > 0 && (
        <div className="space-y-1.5 pt-2 border-t border-border/40">
          <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
            Mediated introductions
          </div>
          {mediations.map((m) => (
            <div
              key={`${m.mediator}-${m.target}`}
              className="flex flex-wrap items-center gap-2 rounded-lg bg-muted/10 border border-border/50 px-3 py-2 text-xs"
            >
              <span className="font-medium">{m.target_nick || `${m.target.slice(0, 12)}...`}</span>
              <span className="text-muted-foreground">
                via {m.mediator_nick || `${m.mediator.slice(0, 12)}...`}
              </span>
              {m.manual && (
                <span className="px-1.5 py-0.5 rounded bg-muted/30 text-muted-foreground">
                  manual
                </span>
              )}
              <span className="ml-auto text-muted-foreground">{relativeTime(m.date)}</span>
              <button
                type="button"
                onClick={() => cancelMediation(m.mediator, m.target)}
                className="px-2 py-0.5 rounded text-destructive/90 hover:bg-destructive/10 transition-colors"
              >
                Cancel
              </button>
            </div>
          ))}
        </div>
      )}
      {searches.length > 0 && (
        <div className="space-y-1.5 pt-2 border-t border-border/40">
          <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
            KX searches
          </div>
          {searches.map((s) => (
            <div
              key={s.target}
              className="flex items-center gap-2 rounded-lg bg-muted/10 border border-border/50 px-3 py-2 text-xs"
            >
              <span className="font-medium">{s.nick || `${s.target.slice(0, 12)}...`}</span>
              <span className="text-muted-foreground">
                searching for this user across the network
              </span>
            </div>
          ))}
        </div>
      )}
    </SectionCard>
  );
};

// ---- Connection -------------------------------------------------------------

// ---- Behavior ---------------------------------------------------------------

// NumberRow is a labelled integer input (days / level) with a pending-restart
// badge, committed on blur or Enter.
const NumberRow = ({
  label,
  description,
  value,
  suffix,
  min,
  pending,
  onCommit,
}: {
  label: string;
  description: string;
  value: number;
  suffix: string;
  min: number;
  pending: boolean;
  onCommit: (v: number) => void;
}) => {
  const [draft, setDraft] = useState(String(value));
  useEffect(() => setDraft(String(value)), [value]);
  const commit = () => {
    const n = Math.floor(Number(draft));
    if (Number.isFinite(n) && n >= min && n !== value) onCommit(n);
    else setDraft(String(value));
  };
  return (
    <div className="space-y-1 pt-3 border-t border-border/40">
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium flex items-center">
          {label}
          <PendingBadge show={pending} />
        </span>
        <div className="flex items-center gap-2 shrink-0">
          <input
            type="number"
            min={min}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commit}
            onKeyDown={(e) => {
              if (e.key === 'Enter') (e.target as HTMLInputElement).blur();
            }}
            className="w-20 rounded-lg border border-border bg-background px-2 py-1 text-right text-sm"
          />
          <span className="text-xs text-muted-foreground w-9">{suffix}</span>
        </div>
      </div>
      <p className="text-sm text-muted-foreground">{description}</p>
    </div>
  );
};

const BehaviorCard = () => {
  const [saved, setSaved] = useState<BRBehavior | null>(null);
  const [effective, setEffective] = useState<BRBehavior | null>(null);
  const [ignoreDraft, setIgnoreDraft] = useState('');
  const [err, setErr] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const r = await getBisonrelayBehaviorSettings();
      setSaved(r.saved);
      setEffective(r.effective);
      setIgnoreDraft(r.saved.idleRemoveIgnore.join('\n'));
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load behavior settings');
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const commit = useCallback(
    async (update: Partial<BRBehavior>) => {
      try {
        await setBisonrelayBehaviorSettings(update);
        await refresh();
      } catch (e: any) {
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not save setting');
      }
    },
    [refresh],
  );

  if (!saved || !effective) {
    return (
      <SectionCard title="Behavior" icon={SlidersHorizontal}>
        <p className="text-sm text-muted-foreground">{err || 'Loading...'}</p>
      </SectionCard>
    );
  }

  const s = saved;
  const eff = effective;
  const pend = (k: keyof BRBehavior) => JSON.stringify(s[k]) !== JSON.stringify(eff[k]);

  const commitIgnore = () => {
    const list = ignoreDraft
      .split('\n')
      .map((x) => x.trim())
      .filter(Boolean);
    if (JSON.stringify(list) !== JSON.stringify(s.idleRemoveIgnore)) {
      commit({ idleRemoveIgnore: list });
    }
  };

  return (
    <SectionCard title="Behavior" icon={SlidersHorizontal}>
      <p className="text-sm text-muted-foreground">
        How the Bison Relay client behaves. These are read only when the
        messaging daemon starts, so a change is saved now and takes effect after
        Bison Relay next restarts. A "pending restart" tag marks any setting that
        differs from the value the daemon is currently running.
      </p>

      <Toggle
        label="Send receive receipts"
        description="Acknowledge posts and comments you receive back to their authors, so they can see you got them."
        checked={s.sendReceiveReceipts}
        onChange={(next) => commit({ sendReceiveReceipts: next })}
        pending={pend('sendReceiveReceipts')}
      />
      <Toggle
        label="Auto-subscribe to posts"
        description="Automatically subscribe to a contact's posts the first time you key-exchange with them."
        checked={s.autoSubscribePosts}
        onChange={(next) => commit({ autoSubscribePosts: next })}
        pending={pend('autoSubscribePosts')}
      />
      <Toggle
        label="Track in-call chat history"
        description="Keep chat messages exchanged during live voice/video (RTDT) sessions. The in-call chat panel needs this on."
        checked={s.trackRtdtChat}
        onChange={(next) => commit({ trackRtdtChat: next })}
        pending={pend('trackRtdtChat')}
      />

      <NumberRow
        label="Idle contact auto-removal"
        description="Unsubscribe idle contacts from your posts and remove idle members from group chats you admin, after this many days with no message from them. 0 turns it off."
        value={s.idleRemoveDays}
        suffix="days"
        min={0}
        pending={pend('idleRemoveDays')}
        onCommit={(v) => commit({ idleRemoveDays: v })}
      />
      <NumberRow
        label="Auto-handshake interval"
        description="Periodically handshake with contacts you have not heard from, to keep the ratchet fresh. 0 turns it off."
        value={s.autoHandshakeDays}
        suffix="days"
        min={0}
        pending={pend('autoHandshakeDays')}
        onCommit={(v) => commit({ autoHandshakeDays: v })}
      />
      <NumberRow
        label="Group chat invite expiration"
        description="How long a group chat invitation you send stays valid."
        value={s.gcInviteDays}
        suffix="days"
        min={1}
        pending={pend('gcInviteDays')}
        onCommit={(v) => commit({ gcInviteDays: v })}
      />

      <div className="space-y-1 pt-3 border-t border-border/40">
        <span className="font-medium flex items-center">
          Idle-removal ignore list
          <PendingBadge show={pend('idleRemoveIgnore')} />
        </span>
        <p className="text-sm text-muted-foreground">
          One nick or hex identity per line. These contacts are never
          auto-removed even when idle.
        </p>
        <textarea
          value={ignoreDraft}
          onChange={(e) => setIgnoreDraft(e.target.value)}
          onBlur={commitIgnore}
          rows={3}
          spellCheck={false}
          className="w-full rounded-lg border border-border bg-background px-2 py-1 font-mono text-xs"
        />
      </div>

      {err && <p className="text-sm text-destructive">{err}</p>}
    </SectionCard>
  );
};

// ---- Advanced (tuning) ------------------------------------------------------

const AdvancedCard = () => {
  const [saved, setSaved] = useState<BRBehavior | null>(null);
  const [effective, setEffective] = useState<BRBehavior | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const r = await getBisonrelayBehaviorSettings();
      setSaved(r.saved);
      setEffective(r.effective);
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load advanced settings');
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const commit = useCallback(
    async (update: Partial<BRBehavior>) => {
      try {
        await setBisonrelayBehaviorSettings(update);
        await refresh();
      } catch (e: any) {
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not save setting');
      }
    },
    [refresh],
  );

  if (!saved || !effective) {
    return (
      <SectionCard title="Advanced" icon={Gauge}>
        <p className="text-sm text-muted-foreground">{err || 'Loading...'}</p>
      </SectionCard>
    );
  }

  const s = saved;
  const eff = effective;
  const pend = (k: keyof BRBehavior) => s[k] !== eff[k];
  const GroupLabel = ({ children }: { children: ReactNode }) => (
    <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide pt-3">{children}</p>
  );

  return (
    <SectionCard title="Advanced" icon={Gauge}>
      <p className="text-sm text-muted-foreground">
        Low-level Bison Relay client tuning. Most people should leave these at
        their defaults. Like the Behavior settings, changes are saved now and take
        effect after Bison Relay next restarts.
      </p>

      <GroupLabel>Performance</GroupLabel>
      <NumberRow
        label="Compression level"
        description="zlib level for outbound routed messages (0 = none, 9 = best). Higher costs more CPU for smaller relay push fees."
        value={s.compressLevel}
        suffix="0-9"
        min={0}
        pending={pend('compressLevel')}
        onCommit={(v) => commit({ compressLevel: Math.min(9, v) })}
      />
      <NumberRow
        label="Reconnect delay"
        description="Wait between reconnection attempts to the relay server."
        value={s.reconnectSecs}
        suffix="secs"
        min={1}
        pending={pend('reconnectSecs')}
        onCommit={(v) => commit({ reconnectSecs: v })}
      />
      <NumberRow
        label="GCMQ max lifetime"
        description="How long to wait for a group-chat message from a member before assuming it will not arrive."
        value={s.gcmqMaxLifetimeSecs}
        suffix="secs"
        min={1}
        pending={pend('gcmqMaxLifetimeSecs')}
        onCommit={(v) => commit({ gcmqMaxLifetimeSecs: v })}
      />
      <NumberRow
        label="GCMQ update delay"
        description="How often the group-chat message queue checks its rules to emit messages."
        value={s.gcmqUpdateSecs}
        suffix="secs"
        min={1}
        pending={pend('gcmqUpdateSecs')}
        onCommit={(v) => commit({ gcmqUpdateSecs: v })}
      />
      <NumberRow
        label="GCMQ initial delay"
        description="How long after initial subscriptions to start processing group-chat queue messages."
        value={s.gcmqInitialSecs}
        suffix="secs"
        min={1}
        pending={pend('gcmqInitialSecs')}
        onCommit={(v) => commit({ gcmqInitialSecs: v })}
      />

      <GroupLabel>Tipping</GroupLabel>
      <NumberRow
        label="Tip restart delay"
        description="Wait after startup before resuming pending tip attempts."
        value={s.tipRestartSecs}
        suffix="secs"
        min={1}
        pending={pend('tipRestartSecs')}
        onCommit={(v) => commit({ tipRestartSecs: v })}
      />
      <NumberRow
        label="Tip re-request invoice"
        description="Re-request an invoice from the recipient if one has not arrived yet."
        value={s.tipRerequestHours}
        suffix="hrs"
        min={1}
        pending={pend('tipRerequestHours')}
        onCommit={(v) => commit({ tipRerequestHours: v })}
      />
      <NumberRow
        label="Tip max lifetime"
        description="Stop attempting to pay a tip's invoice after this long from the initial attempt."
        value={s.tipMaxLifetimeHours}
        suffix="hrs"
        min={1}
        pending={pend('tipMaxLifetimeHours')}
        onCommit={(v) => commit({ tipMaxLifetimeHours: v })}
      />
      <NumberRow
        label="Tip pay-retry factor"
        description="Exponential backoff factor for retrying a tip payment."
        value={s.tipPayRetrySecs}
        suffix="secs"
        min={1}
        pending={pend('tipPayRetrySecs')}
        onCommit={(v) => commit({ tipPayRetrySecs: v })}
      />

      <GroupLabel>Key exchange</GroupLabel>
      <NumberRow
        label="Mediate-ID cooldown"
        description="Wait before requesting a new mediated introduction for the same target."
        value={s.mediateCooldownDays}
        suffix="days"
        min={1}
        pending={pend('mediateCooldownDays')}
        onCommit={(v) => commit({ mediateCooldownDays: v })}
      />
      <NumberRow
        label="Max auto-KX mediate requests"
        description="Maximum automatic mediated-introduction requests to a single user."
        value={s.maxAutoMediate}
        suffix="max"
        min={1}
        pending={pend('maxAutoMediate')}
        onCommit={(v) => commit({ maxAutoMediate: v })}
      />
      <NumberRow
        label="Unkxd warning interval"
        description="Minimum time between warnings about acting with contacts you have not key-exchanged with."
        value={s.unkxdWarnHours}
        suffix="hrs"
        min={1}
        pending={pend('unkxdWarnHours')}
        onCommit={(v) => commit({ unkxdWarnHours: v })}
      />

      {err && <p className="text-sm text-destructive">{err}</p>}
    </SectionCard>
  );
};

const ConnectionCard = () => {
  const [state, setState] = useState<BisonrelayConnectionState | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [liquidityOpen, setLiquidityOpen] = useState(false);
  const [liquidityPoint, setLiquidityPoint] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setState(await getBisonrelayConnection());
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load connection state');
    }
  }, []);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 10000);
    return () => clearInterval(id);
  }, [refresh]);

  const onToggle = async () => {
    if (!state || busy) return;
    setBusy(true);
    try {
      await setBisonrelayConnection(!state.online);
      await refresh();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not change connection');
    } finally {
      setBusy(false);
    }
  };

  const policy = state?.policy;
  // Push fees are quoted in milli-atoms per push_pay_rate_bytes; scale to a
  // per-MB figure for display, mirroring the Stats network view.
  const ratePerMB =
    policy && policy.push_pay_rate_bytes > 0
      ? (policy.push_pay_rate_matoms / policy.push_pay_rate_bytes) * 1024 * 1024
      : 0;
  const connected = state?.connected ?? false;

  return (
    <SectionCard
      title="Connection"
      icon={Wifi}
      action={
        state && (
          <button
            type="button"
            onClick={onToggle}
            disabled={busy}
            className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors disabled:opacity-50 ${
              state.online
                ? 'bg-emerald-500/15 text-emerald-400 hover:bg-emerald-500/25'
                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
            }`}
          >
            {busy ? 'Switching...' : state.online ? 'Online' : 'Offline'}
          </button>
        )
      }
    >
      <div className="flex items-center gap-2 text-xs">
        <span
          className={`inline-block h-2 w-2 rounded-full ${
            connected ? 'bg-emerald-400' : 'bg-muted-foreground/50'
          }`}
        />
        <span className="text-muted-foreground">
          {connected ? 'Connected to the relay server' : `Not connected (${state?.stage || '...'})`}
        </span>
      </div>
      <p className="text-xs text-muted-foreground">
        Going offline disconnects from the relay server until switched back
        on; messages keep queuing in your mailbox meanwhile. Remaining
        offline lasts until the messaging daemon restarts.
      </p>
      {policy && (policy.max_msg_size || 0) > 0 && (
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
          <PolicyTile
            label="Push fee"
            value={`${formatDCR(ratePerMB, 6)} DCR / MB`}
            hint={`min ${formatDCR(policy.push_pay_rate_min_matoms, 8)}`}
          />
          <PolicyTile
            label="Subscription fee"
            value={`${formatDCR(policy.sub_pay_rate || 0, 8)} DCR`}
            hint="per rendezvous"
          />
          <PolicyTile
            label="Retention"
            value={`${policy.expiration_days || 0} days`}
            hint="dropped after"
          />
          <PolicyTile
            label="Max msg size"
            value={formatBytes(policy.max_msg_size || 0)}
            hint="per-message cap"
          />
          <PolicyTile
            label="Max push invoices"
            value={String(policy.max_push_invoices || 0)}
            hint="concurrent"
          />
        </div>
      )}
      <div className="space-y-1 pt-2 border-t border-border/40">
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs text-muted-foreground">
            Receiving payments needs inbound capacity. Request an inbound
            Lightning channel from a liquidity provider for a small fee.
          </span>
          <button
            type="button"
            onClick={() => setLiquidityOpen(true)}
            className="shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 transition-colors"
          >
            Request Inbound Channel
          </button>
        </div>
        {liquidityPoint && (
          <p className="text-xs text-success font-mono break-all">
            Channel requested: {liquidityPoint}
          </p>
        )}
      </div>
      {err && <p className="text-xs text-destructive">{err}</p>}
      {liquidityOpen && (
        <RequestLiquidityModal
          onClose={() => setLiquidityOpen(false)}
          onSuccess={(cp) => setLiquidityPoint(cp)}
        />
      )}
    </SectionCard>
  );
};

// ---- Content filters --------------------------------------------------------

const emptyFilter: BisonrelayContentFilter = {
  id: 0,
  regexp: '',
  skip_pms: false,
  skip_gcms: false,
  skip_posts: false,
  skip_post_comments: false,
};

// Mirror Go's regexp.QuoteMeta: bridged lines like "[m] <kandiru>" are full of
// regex metachars, so literal-mode input is escaped before it is stored as a regex.
const quoteMeta = (s: string): string => s.replace(/[\\.+*?()|[\]{}^$]/g, '\\$&');

const FiltersCard = () => {
  const [filters, setFilters] = useState<BisonrelayContentFilter[] | null>(null);
  const [contacts, setContacts] = useState<BisonrelayContact[]>([]);
  const [gcs, setGcs] = useState<BisonrelayGC[]>([]);
  const [form, setForm] = useState<BisonrelayContentFilter | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [formErr, setFormErr] = useState<string | null>(null);
  const [literal, setLiteral] = useState(false);
  const [sample, setSample] = useState('');

  const refresh = useCallback(async () => {
    try {
      setFilters(await getBisonrelayFilters());
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load filters');
    }
  }, []);

  useEffect(() => {
    refresh();
    getBisonrelayContacts()
      .then(setContacts)
      .catch(() => {});
    listBisonrelayGCs()
      .then(setGcs)
      .catch(() => {});
  }, [refresh]);

  const nickFor = (uid?: string): string => {
    if (!uid) return '';
    const c = contacts.find((e) => e.id?.identity === uid);
    return c?.nick_alias || c?.id?.nick || `${uid.slice(0, 12)}...`;
  };
  const gcNameFor = (gcid?: string): string => {
    if (!gcid) return '';
    const g = gcs.find((e) => e.id === gcid);
    return g?.alias || g?.name || `${gcid.slice(0, 12)}...`;
  };

  const onSave = async () => {
    if (!form || busy) return;
    if (!form.regexp.trim()) {
      setFormErr('A pattern is required.');
      return;
    }
    const effective = literal ? quoteMeta(form.regexp.trim()) : form.regexp.trim();
    try {
      new RegExp(effective);
    } catch (e: any) {
      setFormErr(`Invalid pattern: ${e?.message || 'not a valid regular expression'}`);
      return;
    }
    setBusy(true);
    setFormErr(null);
    try {
      await upsertBisonrelayFilter({ ...form, regexp: effective });
      setForm(null);
      setLiteral(false);
      setSample('');
      await refresh();
    } catch (e: any) {
      const body = e?.response?.data;
      setFormErr(typeof body === 'string' ? body : e?.message || 'Could not save filter');
    } finally {
      setBusy(false);
    }
  };

  const onDelete = async (id: number) => {
    if (busy) return;
    setBusy(true);
    try {
      await deleteBisonrelayFilter(id);
      await refresh();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not delete filter');
    } finally {
      setBusy(false);
    }
  };

  const skipChips = (f: BisonrelayContentFilter): string[] => {
    const out: string[] = [];
    if (!f.skip_pms) out.push('PMs');
    if (!f.skip_gcms) out.push('GC messages');
    if (!f.skip_posts) out.push('posts');
    if (!f.skip_post_comments) out.push('comments');
    return out;
  };

  const pattern = form ? (literal ? quoteMeta(form.regexp.trim()) : form.regexp.trim()) : '';
  let patternErr = '';
  let sampleMatches: boolean | null = null;
  if (pattern) {
    try {
      const re = new RegExp(pattern);
      if (sample.trim()) sampleMatches = re.test(sample);
    } catch (e: any) {
      patternErr = e?.message || 'invalid regular expression';
    }
  }

  return (
    <SectionCard
      title="Content filters"
      icon={Filter}
      action={
        !form && (
          <button
            type="button"
            onClick={() => {
              setFormErr(null);
              setLiteral(false);
              setSample('');
              setForm({ ...emptyFilter });
            }}
            className={backupBtnCls}
          >
            <Plus className="h-3.5 w-3.5" />
            Add filter
          </button>
        )
      }
    >
      <p className="text-xs text-muted-foreground">
        Messages, posts and comments matching a filter pattern are hidden
        before they reach the UI. The pattern is a regular expression; enable
        "Match literal text" when adding a filter to match exact text instead. A
        filter applies to all contacts and group chats unless limited below.
      </p>
      {err && <p className="text-xs text-destructive">{err}</p>}
      {filters && filters.length === 0 && !form && (
        <p className="text-xs text-muted-foreground">No content filters configured.</p>
      )}
      {filters && filters.length > 0 && (
        <div className="space-y-1.5">
          {filters.map((f) => (
            <div
              key={f.id}
              className="flex flex-wrap items-center gap-2 rounded-lg bg-muted/10 border border-border/50 px-3 py-2 text-xs"
            >
              <code className="font-mono text-foreground break-all">{f.regexp}</code>
              <span className="text-muted-foreground">
                {f.uid ? `user: ${nickFor(f.uid)}` : f.gc ? `GC: ${gcNameFor(f.gc)}` : 'everywhere'}
              </span>
              <span className="text-muted-foreground">
                applies to {skipChips(f).join(', ') || 'nothing'}
              </span>
              <span className="ml-auto flex items-center gap-1">
                <button
                  type="button"
                  aria-label="Edit filter"
                  onClick={() => {
                    setFormErr(null);
                    setLiteral(false);
                    setSample('');
                    setForm({ ...f });
                  }}
                  className="p-1 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground"
                >
                  <Pencil className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  aria-label="Delete filter"
                  onClick={() => onDelete(f.id)}
                  disabled={busy}
                  className="p-1 rounded hover:bg-destructive/15 text-muted-foreground hover:text-destructive"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </span>
            </div>
          ))}
        </div>
      )}
      {form && (
        <div className="space-y-3 rounded-lg bg-muted/10 border border-border/50 p-3">
          <p className="text-xs font-medium">{form.id ? 'Edit filter' : 'New filter'}</p>
          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="text-[11px] text-muted-foreground">
                {literal ? 'Text to match (literal)' : 'Pattern (regular expression)'}
              </label>
              <label className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <input
                  type="checkbox"
                  checked={literal}
                  onChange={(e) => setLiteral(e.target.checked)}
                />
                Match literal text
              </label>
            </div>
            <input
              type="text"
              value={form.regexp}
              onChange={(e) => setForm({ ...form, regexp: e.target.value })}
              placeholder={literal ? 'e.g. [m] <kandiru>' : 'e.g. spam|casino'}
              className="w-full rounded-lg bg-background/60 border border-border px-3 py-1.5 text-xs font-mono text-foreground focus:outline-none focus:border-primary/50"
            />
            {patternErr && (
              <p className="text-[11px] text-destructive break-all mt-1">
                Invalid pattern: {patternErr}
              </p>
            )}
            {literal && !patternErr && form.regexp.trim() && (
              <p className="text-[11px] text-muted-foreground break-all mt-1">
                Stored as: <code className="font-mono text-foreground">{pattern}</code>
              </p>
            )}
            <div className="mt-2">
              <label className="text-[11px] text-muted-foreground block mb-1">
                Test against a sample message (optional)
              </label>
              <input
                type="text"
                value={sample}
                onChange={(e) => setSample(e.target.value)}
                placeholder="paste a real message to test"
                className="w-full rounded-lg bg-background/60 border border-border px-3 py-1.5 text-xs font-mono text-foreground focus:outline-none focus:border-primary/50"
              />
              {sample.trim() && !patternErr && sampleMatches !== null && (
                <p
                  className={`text-[11px] mt-1 ${
                    sampleMatches ? 'text-destructive' : 'text-muted-foreground'
                  }`}
                >
                  {sampleMatches
                    ? 'This message WOULD be hidden by this filter.'
                    : 'This message would NOT be hidden.'}
                </p>
              )}
              <p className="text-[10px] text-muted-foreground/70 mt-1">
                Preview uses the browser regex engine; exact for literal text.
              </p>
            </div>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="text-[11px] text-muted-foreground block mb-1">
                Limit to user (optional)
              </label>
              <select
                value={form.uid || ''}
                onChange={(e) =>
                  setForm({ ...form, uid: e.target.value || undefined, gc: undefined })
                }
                className="w-full rounded-lg bg-background/60 border border-border px-2 py-1.5 text-xs text-foreground focus:outline-none"
              >
                <option value="">All users</option>
                {contacts.map((c) => (
                  <option key={c.id?.identity} value={c.id?.identity}>
                    {c.nick_alias || c.id?.nick || c.id?.identity}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-[11px] text-muted-foreground block mb-1">
                Limit to group chat (optional)
              </label>
              <select
                value={form.gc || ''}
                onChange={(e) =>
                  setForm({ ...form, gc: e.target.value || undefined, uid: undefined })
                }
                className="w-full rounded-lg bg-background/60 border border-border px-2 py-1.5 text-xs text-foreground focus:outline-none"
              >
                <option value="">All group chats</option>
                {gcs.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.alias || g.name || g.id}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-2">
            {(
              [
                ['skip_pms', 'Private messages'],
                ['skip_gcms', 'GC messages'],
                ['skip_posts', 'Posts'],
                ['skip_post_comments', 'Post comments'],
              ] as const
            ).map(([key, label]) => (
              <label key={key} className="flex items-center gap-2 text-xs text-foreground">
                <input
                  type="checkbox"
                  checked={!form[key]}
                  onChange={(e) => setForm({ ...form, [key]: !e.target.checked })}
                />
                {label}
              </label>
            ))}
          </div>
          <p className="text-[11px] text-muted-foreground">
            Checked content types are filtered; unchecked ones are left
            untouched by this filter.
          </p>
          {formErr && <p className="text-xs text-destructive break-all">{formErr}</p>}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={() => {
                setForm(null);
                setLiteral(false);
                setSample('');
              }}
              className="px-3 py-1.5 rounded-lg text-xs text-muted-foreground hover:text-foreground"
            >
              Cancel
            </button>
            <button type="button" onClick={onSave} disabled={busy} className={backupBtnCls}>
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              Save filter
            </button>
          </div>
        </div>
      )}
    </SectionCard>
  );
};

// ---- Backup -----------------------------------------------------------------

// BackupCard drives the full-state backup download. brclientd needs a minute
// or more to build the tarball before any bytes flow, so the file is prepared
// server-side in a detached job: the card polls the job status (the job
// survives navigation), pushes the browser download automatically when the
// preparation it observed completes, and keeps the prepared file
// re-downloadable until the next prepare or a dashboard restart.
const BackupCard = () => {
  const [status, setStatus] = useState<BisonrelayBackupStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const aliveRef = useRef(true);
  const pollRef = useRef<number | null>(null);
  const prevStateRef = useRef<string | null>(null);
  const autoPushedRef = useRef(false);

  const stopPolling = () => {
    if (pollRef.current !== null) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
  };

  const pushDownload = () => {
    const a = document.createElement('a');
    a.href = '/api/br/backup';
    a.download = '';
    document.body.appendChild(a);
    a.click();
    a.remove();
  };

  const applyStatus = useCallback((s: BisonrelayBackupStatus) => {
    if (!aliveRef.current) return;
    // Push the download only when this mount observed the preparation
    // complete; returning to an already-ready card must not re-download.
    if (s.state === 'ready' && prevStateRef.current === 'preparing' && !autoPushedRef.current) {
      autoPushedRef.current = true;
      pushDownload();
    }
    prevStateRef.current = s.state;
    setStatus(s);
    if (s.state !== 'preparing') stopPolling();
  }, []);

  const startPolling = useCallback(() => {
    if (pollRef.current !== null) return;
    pollRef.current = window.setInterval(async () => {
      try {
        applyStatus(await getBisonrelayBackupStatus());
      } catch {
        // Transient poll errors keep the current state; the next tick retries.
      }
    }, 2000);
  }, [applyStatus]);

  useEffect(() => {
    aliveRef.current = true;
    getBisonrelayBackupStatus()
      .then((s) => {
        if (!aliveRef.current) return;
        prevStateRef.current = s.state;
        setStatus(s);
        if (s.state === 'preparing') startPolling();
      })
      .catch(() => {});
    return () => {
      aliveRef.current = false;
      stopPolling();
    };
  }, [startPolling]);

  // Local tick so the user sees the preparation advancing between polls.
  useEffect(() => {
    if (status?.state !== 'preparing' || !status.startedAt) {
      setElapsed(0);
      return;
    }
    const start = status.startedAt * 1000;
    const tick = () => setElapsed(Math.max(0, Math.floor((Date.now() - start) / 1000)));
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [status?.state, status?.startedAt]);

  const onPrepare = async () => {
    if (busy) return;
    setBusy(true);
    autoPushedRef.current = false;
    try {
      applyStatus(await prepareBisonrelayBackup());
      startPolling();
    } catch (err: any) {
      const body = err?.response?.data;
      applyStatus({
        state: 'error',
        error: typeof body === 'string' ? body : err?.message || 'Backup preparation failed',
      });
    } finally {
      if (aliveRef.current) setBusy(false);
    }
  };

  const state = status?.state ?? 'idle';

  return (
    <SectionCard
      title="Backup"
      icon={Download}
      action={
        state === 'preparing' ? (
          <button type="button" disabled className={backupBtnCls}>
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            Preparing... {elapsed}s
          </button>
        ) : state === 'ready' ? (
          <a href="/api/br/backup" download className={backupBtnCls}>
            <Download className="h-3.5 w-3.5" />
            Save backup file
          </a>
        ) : (
          <button type="button" onClick={onPrepare} disabled={busy} className={backupBtnCls}>
            <Download className="h-3.5 w-3.5" />
            {state === 'error' ? 'Try again' : 'Download backup'}
          </button>
        )
      }
    >
      <p className="text-xs text-muted-foreground">
        Downloads a consistent snapshot of the full Bison Relay state:
        identity, contacts, message history, posts and shared files. Restore
        it from the setup wizard on a fresh node. Sessions with contacts you
        message after taking the backup are re-keyed automatically on
        restore, so back up regularly to keep restores seamless.
      </p>
      {state === 'preparing' && (
        <div className="flex items-start gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin text-primary shrink-0 mt-0.5" />
          <span>
            Preparing the backup on the server - this can take a minute or
            two depending on the size of your message history and files. You
            can leave this page; the backup will be available here when it
            is ready.
          </span>
        </div>
      )}
      {state === 'ready' && (
        <div className="flex items-start gap-2 text-xs">
          <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400 shrink-0 mt-0.5" />
          <span className="text-muted-foreground">
            Backup ready ({formatBytes(status?.size || 0)}) - kept here until
            the next backup or a dashboard restart.{' '}
            <button
              type="button"
              onClick={onPrepare}
              disabled={busy}
              className="underline hover:text-foreground"
            >
              Prepare a fresh backup
            </button>
          </span>
        </div>
      )}
      {state === 'error' && status?.error && (
        <div className="flex items-start gap-2 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 shrink-0 mt-0.5" />
          <span className="break-all">{status.error}</span>
        </div>
      )}
    </SectionCard>
  );
};

// ---- About ------------------------------------------------------------------

const AboutCard = () => {
  const [version, setVersion] = useState<BisonrelayVersion | null>(null);
  const [policy, setPolicy] = useState<BisonrelayConnectionState['policy'] | null>(null);
  const [dcrUsd, setDcrUsd] = useState(0);
  const [liquidityOpen, setLiquidityOpen] = useState(false);
  const [liquidityPoint, setLiquidityPoint] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    getBisonrelayVersion()
      .then((v) => alive && setVersion(v))
      .catch(() => {});
    getBisonrelayConnection()
      .then((s) => alive && setPolicy(s.policy ?? null))
      .catch(() => {});
    getBisonrelayRates()
      .then((r) => alive && setDcrUsd(r.dcr_usd || 0))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  // Push fees are quoted in milli-atoms per push_pay_rate_bytes; scale to a
  // per-MB figure (the same formula the Connection card uses).
  const ratePerMbMatoms =
    policy && policy.push_pay_rate_bytes > 0
      ? (policy.push_pay_rate_matoms / policy.push_pay_rate_bytes) * 1024 * 1024
      : 0;

  // fmtCost renders a milli-atom cost as "X DCR (~$Y)", or just DCR when the
  // USD rate is not available yet, or "..." before the push rate has loaded.
  const fmtCost = (matoms: number): string => {
    if (matoms <= 0) return '...';
    const dcr = `${formatDCR(matoms, 6)} DCR`;
    if (dcrUsd <= 0) return dcr;
    const usd = (matoms / 1e11) * dcrUsd;
    const usdStr =
      usd >= 0.01 ? `$${usd.toFixed(2)}` : usd >= 0.0001 ? `$${usd.toFixed(4)}` : '<$0.0001';
    return `${dcr} (~${usdStr})`;
  };

  return (
    <div className="space-y-4">
      <SectionCard title="What using Bison Relay costs" icon={Info}>
        <div className="space-y-2 text-xs text-muted-foreground leading-relaxed">
          <p>
            Bison Relay runs over the Lightning Network, so small payments cover the relay's
            bandwidth and any paid content. The amounts are tiny, usually fractions of a cent.
          </p>
          <ul className="space-y-1 list-disc pl-4">
            <li>
              <span className="text-foreground">Sending</span> any message, post, comment, or file
              is routed by the relay server for a small per-MB push fee. Serving a file a contact
              downloads from you costs the same.
            </li>
            <li>
              <span className="text-foreground">Paid content</span>: downloading a file or fetching
              a paid post that a contact put a price on costs that price (paid to them) plus a small
              routing fee.
            </li>
            <li>
              <span className="text-foreground">Tips</span> you send are the tip amount plus a
              routing fee.
            </li>
          </ul>
          <div className="rounded-lg bg-muted/15 border border-border/40 p-2.5 space-y-1 text-foreground/90">
            <p className="text-[11px] uppercase tracking-wide text-muted-foreground">For example</p>
            <p>
              A typical chat message:{' '}
              <span className="font-mono">{fmtCost(policy?.push_pay_rate_min_matoms ?? 0)}</span>
            </p>
            <p>
              Sending 1 MB of data: <span className="font-mono">{fmtCost(ratePerMbMatoms)}</span>
            </p>
            <p>
              The full text of the Bible (~4 MB):{' '}
              <span className="font-mono">{fmtCost(ratePerMbMatoms * 4)}</span>
            </p>
            <p>
              Sending 1 GB of data:{' '}
              <span className="font-mono">{fmtCost(ratePerMbMatoms * 1024)}</span>
            </p>
          </div>
          <p>
            Your actual sent / received / fee totals and a per-type breakdown are under{' '}
            <a href="#stats/payments" className="text-primary hover:underline">
              Stats &gt; Payments
            </a>
            .
          </p>
        </div>
      </SectionCard>

      <SectionCard title="Receiving payments needs inbound liquidity" icon={Wifi}>
        <div className="space-y-2 text-xs text-muted-foreground leading-relaxed">
          <p>
            A Lightning channel can only <span className="text-foreground">receive</span> up to its
            inbound capacity. New channels start out mostly outbound (good for sending), so to be
            paid you need inbound capacity.
          </p>
          <p>You need inbound capacity to receive:</p>
          <ul className="space-y-1 list-disc pl-4">
            <li>payments for your storefront (simplestore) sales,</li>
            <li>tips from other users, and</li>
            <li>tips from tip bots like Oprah, which reward posts and comments.</li>
          </ul>
          <p>
            Without enough inbound capacity an incoming payment cannot be invoiced and fails with a
            low-capacity warning.
          </p>
          <div className="flex items-center justify-between gap-3 pt-1">
            <span>Get inbound capacity from a liquidity provider for a small fee.</span>
            <button
              type="button"
              onClick={() => setLiquidityOpen(true)}
              className="shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium bg-muted/20 text-muted-foreground hover:bg-muted/30 transition-colors"
            >
              Request Inbound Channel
            </button>
          </div>
          {liquidityPoint && (
            <p className="text-xs text-success font-mono break-all">
              Channel requested: {liquidityPoint}
            </p>
          )}
        </div>
      </SectionCard>

      <SectionCard title="About" icon={Info}>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 text-xs">
          <div>
            <p className="text-muted-foreground">Daemon</p>
            <p className="font-medium mt-0.5">
              {version ? `${version.appName} ${version.appVersion}` : '...'}
            </p>
          </div>
          <div>
            <p className="text-muted-foreground">Bison Relay library</p>
            <p className="font-medium mt-0.5">{version?.brClientVersion || '-'}</p>
          </div>
          <div>
            <p className="text-muted-foreground">Go runtime</p>
            <p className="font-medium mt-0.5">{version?.goRuntime || '-'}</p>
          </div>
        </div>
      </SectionCard>

      {liquidityOpen && (
        <RequestLiquidityModal
          onClose={() => setLiquidityOpen(false)}
          onSuccess={(cp) => setLiquidityPoint(cp)}
        />
      )}
    </div>
  );
};

// ---- Tab --------------------------------------------------------------------

const sidebarItems: {
  id: SettingsSection;
  label: string;
  hash: string;
  icon: ComponentType<{ className?: string }>;
}[] = [
  { id: 'account', label: 'Account', hash: 'settings', icon: User },
  { id: 'appearance', label: 'Appearance', hash: 'settings/appearance', icon: ALargeSmall },
  { id: 'notifications', label: 'Notifications', hash: 'settings/notifications', icon: Bell },
  { id: 'sessions', label: 'Sessions', hash: 'settings/sessions', icon: RotateCw },
  { id: 'connection', label: 'Connection', hash: 'settings/connection', icon: Wifi },
  { id: 'behavior', label: 'Behavior', hash: 'settings/behavior', icon: SlidersHorizontal },
  { id: 'advanced', label: 'Advanced', hash: 'settings/advanced', icon: Gauge },
  { id: 'filters', label: 'Filters', hash: 'settings/filters', icon: Filter },
  { id: 'backup', label: 'Backup', hash: 'settings/backup', icon: Download },
  { id: 'about', label: 'About', hash: 'settings/about', icon: Info },
];

const SettingsSidebar = ({ active }: { active: SettingsSection }) => (
  <aside className="md:w-44 shrink-0 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-2 md:self-start">
    <nav className="flex md:flex-col gap-1 overflow-x-auto overflow-y-hidden md:overflow-visible">
      {sidebarItems.map((item) => {
        const isActive = item.id === active;
        const Icon = item.icon;
        return (
          <button
            key={item.id}
            type="button"
            onClick={() => navigateTo(item.hash)}
            className={`shrink-0 whitespace-nowrap md:w-full px-3 py-2 rounded-md text-sm flex items-center gap-2 text-left transition-colors ${
              isActive
                ? 'bg-primary/20 text-primary font-semibold'
                : 'text-muted-foreground hover:bg-muted/30 hover:text-foreground'
            }`}
          >
            <Icon className="h-4 w-4 shrink-0" />
            <span>{item.label}</span>
          </button>
        );
      })}
    </nav>
  </aside>
);

export const BisonrelaySettingsTab = () => {
  const [section, setSection] = useState<SettingsSection>(readHashSection);

  useEffect(() => {
    const onHashChange = () => setSection(readHashSection());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  const content = (() => {
    if (section === 'appearance') return <AppearanceCard />;
    if (section === 'notifications') return <NotificationsCard />;
    if (section === 'sessions') {
      return (
        <>
          <SessionsCard />
          <KXListCard />
        </>
      );
    }
    if (section === 'connection')
      return (
        <>
          <ConnectionCard />
          <ChannelList />
        </>
      );
    if (section === 'behavior') return <BehaviorCard />;
    if (section === 'advanced') return <AdvancedCard />;
    if (section === 'filters') return <FiltersCard />;
    if (section === 'backup') return <BackupCard />;
    if (section === 'about') return <AboutCard />;
    return <AccountCard />;
  })();

  return (
    <div className="flex flex-col md:flex-row gap-4">
      <SettingsSidebar active={section} />
      <div className="flex-1 min-w-0 space-y-4">{content}</div>
    </div>
  );
};

export default BisonrelaySettingsTab;
