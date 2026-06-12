// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType, useCallback, useEffect, useRef, useState } from 'react';
import { toYMDTime } from '../../utils/date';
import {
  Activity,
  AlertCircle,
  ArrowDownRight,
  ArrowUpRight,
  Atom,
  BarChart3,
  CheckCircle2,
  Coins,
  FileText,
  Hash,
  Inbox,
  Layers,
  Loader2,
  MessageSquare,
  Network,
  Radio,
  Server,
  Shield,
  Signal,
  Trash2,
  TrendingUp,
  Users,
  Wifi,
  XCircle,
} from 'lucide-react';
import {
  BisonrelayAuthoredPostStats,
  BisonrelayPayStatsBreakdown,
  BisonrelayPayStatsUser,
  BisonrelayQuantile,
  BisonrelayRunningTip,
  BisonrelayStatsContact,
  BisonrelayStatsNetwork,
  BisonrelayStatsOverview,
  BisonrelayStatsPayments,
  BisonrelayStatsPosts,
  clearBisonrelayPayStats,
  getBisonrelayIdentity,
  getBisonrelayRunningTips,
  getBisonrelayStatsContacts,
  getBisonrelayStatsNetwork,
  getBisonrelayStatsOverview,
  getBisonrelayStatsPayments,
  getBisonrelayStatsPosts,
} from '../../services/bisonrelayApi';
import { avatarDataUrl } from './bisonrelayAvatar';
import { ConfirmActionModal } from './BisonrelayUserSubNav';

type Section = 'overview' | 'payments' | 'network' | 'contacts' | 'content';

const readHashSection = (): Section => {
  const h = window.location.hash.replace(/^#/, '');
  if (!h.startsWith('stats')) return 'overview';
  const rest = h.slice('stats'.length);
  if (rest === '/payments') return 'payments';
  if (rest === '/network') return 'network';
  if (rest === '/contacts') return 'contacts';
  if (rest === '/content') return 'content';
  return 'overview';
};

const navigateTo = (hash: string): void => {
  window.location.hash = hash;
};

// ---- formatting helpers --------------------------------------------------

// clientdb records payment totals in milli-atoms (1 DCR = 1e11 matoms); see
// bisonrelay client/clientdb/paystats.go RecordUserPayEvent. brclientd's
// /stats JSON exposes them as "_matoms" fields. UI shows DCR rounded to a
// sensible scale; sub-matom precision is meaningless for users.
export const formatDCR = (matoms: number, digits = 4): string => {
  const dcr = matoms / 1e11;
  if (!matoms) return '0';
  if (Math.abs(dcr) < 0.0001) return `${matoms} matoms`;
  return dcr.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '');
};

export const formatBytes = (n: number): string => {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
};

const formatNs = (ns: number): string => {
  if (!ns) return '–';
  if (ns < 1e3) return `${ns}ns`;
  if (ns < 1e6) return `${(ns / 1e3).toFixed(1)}µs`;
  if (ns < 1e9) return `${(ns / 1e6).toFixed(1)}ms`;
  return `${(ns / 1e9).toFixed(2)}s`;
};

const formatDuration = (sec: number): string => {
  if (sec < 60) return `${Math.floor(sec)}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ${Math.floor(sec % 60)}s`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`;
  const days = Math.floor(sec / 86400);
  const hours = Math.floor((sec % 86400) / 3600);
  return `${days}d ${hours}h`;
};

export const relativeTime = (iso?: string): string => {
  if (!iso) return '–';
  const t = Date.parse(iso);
  if (!t || Number.isNaN(t) || t < 0) return '–';
  const now = Date.now();
  const delta = Math.max(0, Math.floor((now - t) / 1000));
  if (delta < 60) return 'just now';
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
  return `${Math.floor(delta / 86400)}d ago`;
};

// Treat dates within a day of the epoch as zero. BR serialises uninitialized
// time.Time as "0001-01-01T00:00:00Z" which Date.parse turns into negatives;
// guard against that for the freshness checks.
export const isMeaningfulDate = (iso?: string): boolean => {
  if (!iso) return false;
  const t = Date.parse(iso);
  return Number.isFinite(t) && t > 86400000;
};

// ---- 30s polling hook ----------------------------------------------------

const usePolledStats = <T,>(loader: () => Promise<T>): {
  data: T | null;
  err: string | null;
  refresh: () => Promise<void>;
} => {
  const [data, setData] = useState<T | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const dataRef = useRef<T | null>(null);

  const refresh = useCallback(async () => {
    try {
      const d = await loader();
      dataRef.current = d;
      setData(d);
      setErr(null);
    } catch (e: any) {
      // Keep showing the last data when a refresh fails; brclientd is
      // expected to be unresponsive while a backup holds the clientdb
      // lock. Only a failed initial load surfaces the error banner.
      if (dataRef.current !== null) return;
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load stats');
    }
  }, [loader]);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 30000);
    return () => clearInterval(id);
  }, [refresh]);

  return { data, err, refresh };
};

// ---- charting primitives (inline SVG, no chart library) ------------------

export const MiniBars = ({
  sent,
  received,
}: {
  sent: number;
  received: number;
}) => {
  const max = Math.max(sent, received, 1);
  const sentPct = (sent / max) * 100;
  const recvPct = (received / max) * 100;
  return (
    <div className="flex items-center gap-1.5">
      <div className="flex-1 h-1.5 rounded-full bg-muted/30 overflow-hidden">
        <div
          className="h-full bg-rose-500/70 transition-[width] duration-300"
          style={{ width: `${sentPct}%` }}
          title={`Sent ${formatDCR(sent)} DCR`}
        />
      </div>
      <div className="flex-1 h-1.5 rounded-full bg-muted/30 overflow-hidden">
        <div
          className="h-full bg-emerald-500/70 transition-[width] duration-300"
          style={{ width: `${recvPct}%` }}
          title={`Received ${formatDCR(received)} DCR`}
        />
      </div>
    </div>
  );
};

// QuantileBar shows the latency distribution as a horizontal scale with
// p50/p75/p90/p99 markers. Total width represents the largest quantile.
const QuantileBar = ({ quantiles }: { quantiles: BisonrelayQuantile[] }) => {
  if (!quantiles || quantiles.length === 0) {
    return <p className="text-xs text-muted-foreground italic">No samples yet.</p>;
  }
  const max = Math.max(...quantiles.map((q) => q.max_ns), 1);
  return (
    <div className="space-y-1.5">
      {quantiles.map((q) => {
        const pct = (q.max_ns / max) * 100;
        return (
          <div key={q.rel} className="flex items-center gap-2 text-[11px]">
            <span className="w-10 shrink-0 text-muted-foreground">{q.rel}</span>
            <div className="flex-1 h-1.5 rounded-full bg-muted/30 overflow-hidden">
              <div
                className="h-full bg-gradient-to-r from-primary/60 to-primary transition-[width] duration-300"
                style={{ width: `${pct}%` }}
              />
            </div>
            <span className="w-16 shrink-0 text-right font-mono text-muted-foreground">
              {formatNs(q.max_ns)}
            </span>
          </div>
        );
      })}
    </div>
  );
};

// FlowDonut shows what fraction of total spend was eaten by fees. Two-arc
// SVG: principal (primary) + fees (amber). Compact, no axis labels.
const FlowDonut = ({
  principal,
  fees,
  size = 110,
}: {
  principal: number;
  fees: number;
  size?: number;
}) => {
  const total = principal + fees;
  if (total === 0) {
    return (
      <div
        className="rounded-full border-2 border-muted/40 flex items-center justify-center text-[10px] text-muted-foreground"
        style={{ width: size, height: size }}
      >
        no data
      </div>
    );
  }
  const stroke = 14;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const feesFrac = fees / total;
  const feesLen = c * feesFrac;
  return (
    <svg width={size} height={size} className="-rotate-90">
      <circle
        cx={size / 2}
        cy={size / 2}
        r={r}
        fill="none"
        stroke="currentColor"
        strokeOpacity="0.15"
        strokeWidth={stroke}
      />
      <circle
        cx={size / 2}
        cy={size / 2}
        r={r}
        fill="none"
        stroke="rgb(245 158 11)"
        strokeWidth={stroke}
        strokeDasharray={`${feesLen} ${c}`}
      />
    </svg>
  );
};

// ---- page shell ----------------------------------------------------------

export const BisonrelayStats = () => {
  const [section, setSection] = useState<Section>(readHashSection);

  useEffect(() => {
    const onHashChange = () => setSection(readHashSection());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  const content = (() => {
    if (section === 'payments') return <PaymentsView />;
    if (section === 'network') return <NetworkView />;
    if (section === 'contacts') return <ContactsView />;
    if (section === 'content') return <ContentView />;
    return <OverviewView />;
  })();

  return (
    <div className="flex flex-col md:flex-row gap-4">
      <StatsSidebar active={section} />
      <div className="flex-1 min-w-0">{content}</div>
    </div>
  );
};

const sidebarItems: { id: Section; label: string; hash: string; icon: ComponentType<{ className?: string }> }[] = [
  { id: 'overview', label: 'Overview', hash: 'stats', icon: Activity },
  { id: 'payments', label: 'Payments', hash: 'stats/payments', icon: Coins },
  { id: 'network', label: 'Network', hash: 'stats/network', icon: Network },
  { id: 'contacts', label: 'Contacts', hash: 'stats/contacts', icon: Users },
  { id: 'content', label: 'Content', hash: 'stats/content', icon: BarChart3 },
];

const StatsSidebar = ({ active }: { active: Section }) => (
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

// ---- shared layout pieces ------------------------------------------------

const Loading = ({ what }: { what: string }) => (
  <div className="flex items-center gap-2 text-sm text-muted-foreground">
    <Loader2 className="h-4 w-4 animate-spin" />
    <span>Loading {what}…</span>
  </div>
);

const ErrorBanner = ({ msg }: { msg: string }) => (
  <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 flex items-start gap-2 text-sm text-destructive">
    <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
    <span className="break-words">{msg}</span>
  </div>
);

export const HeroCard = ({
  icon: Icon,
  label,
  value,
  hint,
  tone = 'primary',
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: string;
  hint?: string;
  tone?: 'primary' | 'emerald' | 'rose' | 'amber';
}) => {
  const toneClass = {
    primary: 'from-primary/15 to-primary/5 text-primary',
    emerald: 'from-emerald-500/15 to-emerald-500/5 text-emerald-400',
    rose: 'from-rose-500/15 to-rose-500/5 text-rose-400',
    amber: 'from-amber-500/15 to-amber-500/5 text-amber-400',
  }[tone];
  return (
    <div
      className={`relative overflow-hidden rounded-xl border border-border/50 bg-gradient-to-br ${toneClass} backdrop-blur-sm p-4`}
    >
      <div className="flex items-center justify-between mb-1">
        <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
          {label}
        </span>
        <Icon className="h-4 w-4 opacity-80" />
      </div>
      <div className="text-2xl font-semibold text-foreground tabular-nums">{value}</div>
      {hint && <div className="text-[10px] text-muted-foreground mt-1">{hint}</div>}
    </div>
  );
};

export const SectionCard = ({
  title,
  icon: Icon,
  children,
  action,
}: {
  title: string;
  icon?: ComponentType<{ className?: string }>;
  children: React.ReactNode;
  action?: React.ReactNode;
}) => (
  <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5 space-y-4">
    <div className="flex items-center justify-between gap-2">
      <h3 className="text-sm font-semibold flex items-center gap-2">
        {Icon && <Icon className="h-4 w-4 text-primary" />}
        {title}
      </h3>
      {action}
    </div>
    {children}
  </div>
);

// backupBtnCls is the small bordered action-button style shared by the BR
// stats and settings cards.
export const backupBtnCls =
  'inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-muted/30 border border-border text-foreground text-xs font-semibold transition-colors hover:bg-muted/50 disabled:opacity-50 disabled:cursor-not-allowed';

// AvatarDisplay renders the local user's avatar read-only in the identity
// strip; changing or clearing it lives in the BR Settings tab.
const AvatarDisplay = ({ nick }: { nick: string }) => {
  const [avatar, setAvatar] = useState('');

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

  const url = avatarDataUrl(avatar);
  return (
    <div className="relative shrink-0 h-14 w-14 rounded-full overflow-hidden border border-primary/30 bg-gradient-to-br from-primary/30 to-primary/10 flex items-center justify-center">
      {url ? (
        <img src={url} alt="Your avatar" className="h-full w-full object-cover" />
      ) : (
        <span className="text-lg font-semibold text-primary">
          {(nick || '?').slice(0, 2).toUpperCase()}
        </span>
      )}
    </div>
  );
};

// ---- 1) Overview --------------------------------------------------------

const OverviewView = () => {
  const { data, err } = usePolledStats(getBisonrelayStatsOverview);
  const [uptime, setUptime] = useState<number>(0);

  // Local tick so the "connected for X" counter advances between fetches.
  useEffect(() => {
    if (!data?.connected_at || !isMeaningfulDate(data.connected_at)) {
      setUptime(0);
      return;
    }
    const start = Date.parse(data.connected_at);
    const tick = () => setUptime(Math.max(0, Math.floor((Date.now() - start) / 1000)));
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [data?.connected_at]);

  if (err) return <ErrorBanner msg={err} />;
  if (!data) return <Loading what="overview" />;

  const totalMoved = data.total_sent_matoms + data.total_received_matoms;
  const stageOk = data.stage === 'ready';

  return (
    <div className="space-y-4">
      {/* Identity strip */}
      <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5 flex items-center gap-4">
        <AvatarDisplay nick={data.nick || ''} />
        <div className="flex-1 min-w-0">
          <div className="text-lg font-semibold text-foreground truncate">
            {data.nick || '(unnamed)'}
          </div>
          <div className="text-[10px] text-muted-foreground font-mono break-all">
            {data.identity}
          </div>
        </div>
        <div
          className={`shrink-0 inline-flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-full border ${
            stageOk
              ? 'border-emerald-500/40 text-emerald-400 bg-emerald-500/10'
              : 'border-amber-500/40 text-amber-400 bg-amber-500/10'
          }`}
        >
          <Signal className="h-3 w-3" />
          {stageOk ? 'Connected' : data.stage}
        </div>
      </div>

      {/* Big-number tiles */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <HeroCard
          icon={Users}
          label="Contacts"
          value={String(data.contacts_count)}
          hint={`${data.subscribers_count} follow you`}
        />
        <HeroCard
          icon={FileText}
          label="Posts authored"
          value={String(data.posts_authored)}
          hint={`Subscribed to ${data.subscriptions_count}`}
          tone="emerald"
        />
        <HeroCard
          icon={Coins}
          label="DCR moved"
          value={formatDCR(totalMoved, 4)}
          hint={`${formatDCR(data.total_fees_matoms, 5)} in fees`}
          tone="amber"
        />
        <HeroCard
          icon={Wifi}
          label="Uptime"
          value={uptime > 0 ? formatDuration(uptime) : '–'}
          hint={
            data.rmq_p50_ns > 0
              ? `p50 RTT ${formatNs(data.rmq_p50_ns)}`
              : 'awaiting samples'
          }
          tone="primary"
        />
      </div>

      {/* Top contacts + connection */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="lg:col-span-2">
          <SectionCard title="Top contacts by activity" icon={TrendingUp}>
            {data.top_contacts.length === 0 ? (
              <p className="text-xs text-muted-foreground italic">
                No payment activity yet. Tips, paid post fetches, and shared-file
                purchases all surface here.
              </p>
            ) : (
              <div className="space-y-2.5">
                {data.top_contacts.map((c) => (
                  <div key={c.uid} className="grid grid-cols-[1fr_auto] gap-3 items-center">
                    <div className="min-w-0">
                      <div className="text-sm font-medium truncate">
                        {c.nick || c.uid.slice(0, 12)}
                      </div>
                      <div className="mt-1">
                        <MiniBars sent={c.sent_matoms} received={c.received_matoms} />
                      </div>
                    </div>
                    <div className="text-right text-[11px] tabular-nums">
                      <div className="text-rose-400 flex items-center justify-end gap-1">
                        <ArrowUpRight className="h-3 w-3" />
                        {formatDCR(c.sent_matoms)}
                      </div>
                      <div className="text-emerald-400 flex items-center justify-end gap-1">
                        <ArrowDownRight className="h-3 w-3" />
                        {formatDCR(c.received_matoms)}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </SectionCard>
        </div>
        <SectionCard title="Server" icon={Server}>
          <div className="space-y-2 text-xs">
            <div>
              <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                LN node
              </div>
              <div className="font-mono break-all text-[11px] text-foreground/90">
                {data.server_node || '–'}
              </div>
            </div>
            <div className="grid grid-cols-2 gap-2 pt-1">
              <div>
                <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                  p50 RTT
                </div>
                <div className="font-mono text-foreground/90">
                  {formatNs(data.rmq_p50_ns)}
                </div>
              </div>
              <div>
                <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                  Connected
                </div>
                <div className="font-mono text-foreground/90">
                  {data.connected_at && isMeaningfulDate(data.connected_at)
                    ? relativeTime(data.connected_at)
                    : '–'}
                </div>
              </div>
            </div>
          </div>
        </SectionCard>
      </div>

    </div>
  );
};

// ---- 2) Payments --------------------------------------------------------

const PaymentsView = () => {
  const { data, err, refresh } = usePolledStats(getBisonrelayStatsPayments);
  const [openUid, setOpenUid] = useState<string | null>(null);
  const [clearUid, setClearUid] = useState<string | null>(null);
  // Tip attempts the daemon is actively driving (retries can span days);
  // polled separately from the aggregate stats.
  const [runningTips, setRunningTips] = useState<BisonrelayRunningTip[]>([]);

  useEffect(() => {
    const load = () => getBisonrelayRunningTips().then(setRunningTips).catch(() => {});
    load();
    const id = window.setInterval(load, 30000);
    return () => window.clearInterval(id);
  }, []);

  if (err) return <ErrorBanner msg={err} />;
  if (!data) return <Loading what="payment stats" />;

  const totals = data.users.reduce(
    (acc, u) => {
      acc.sent += u.sent_matoms;
      acc.recv += u.received_matoms;
      acc.fees += u.fees_matoms;
      return acc;
    },
    { sent: 0, recv: 0, fees: 0 },
  );
  const principal = totals.sent - totals.fees;
  // Cheap to compute every render; the user count rarely exceeds ~20.
  // useMemo would have to live above the early returns to be Rules-of-Hooks
  // safe, but it's not worth the indirection for a trivial sort.
  const sortedUsers = [...data.users].sort(
    (a, b) => (b.sent_matoms + b.received_matoms) - (a.sent_matoms + a.received_matoms),
  );
  const openUser = sortedUsers.find((u) => u.uid === openUid) ?? null;

  return (
    <div className="space-y-4">
      {runningTips.length > 0 && (
        <SectionCard title="Tips in flight" icon={Coins}>
          <div className="space-y-1.5">
            {runningTips.map((t) => (
              <div
                key={`${t.uid}-${t.tag}`}
                className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground"
              >
                <span className="font-medium text-foreground/90">{t.nick || t.uid.slice(0, 12)}</span>
                <span className="tabular-nums">{formatDCR(t.amount_matoms, 8)}</span>
                <span className="opacity-50">·</span>
                <span>
                  {t.next_action.replace(/_/g, ' ')} at{' '}
                  {toYMDTime(new Date(t.next_action_time))}
                </span>
              </div>
            ))}
          </div>
          <p className="text-[10px] text-muted-foreground pt-2">
            The daemon keeps retrying these tips until they complete or run
            out of attempts; both sides must be online for an attempt to
            settle.
          </p>
        </SectionCard>
      )}

      {/* Totals + fees donut + latency strip */}
      <div className="grid grid-cols-1 lg:grid-cols-[2fr_1fr] gap-4">
        <SectionCard title="All-time totals" icon={Coins}>
          <div className="grid grid-cols-3 gap-3">
            <div>
              <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                Sent
              </div>
              <div className="text-xl font-semibold text-rose-400 tabular-nums">
                {formatDCR(totals.sent, 5)}
              </div>
            </div>
            <div>
              <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                Received
              </div>
              <div className="text-xl font-semibold text-emerald-400 tabular-nums">
                {formatDCR(totals.recv, 5)}
              </div>
            </div>
            <div>
              <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                Fees paid
              </div>
              <div className="text-xl font-semibold text-amber-400 tabular-nums">
                {formatDCR(totals.fees, 5)}
              </div>
            </div>
          </div>
          <div className="flex items-center gap-4 pt-2">
            <div className="text-primary">
              <FlowDonut principal={principal} fees={totals.fees} />
            </div>
            <div className="text-xs space-y-1.5">
              <div className="flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full bg-primary/50" />
                <span className="text-muted-foreground">Principal</span>
                <span className="font-mono text-foreground/90">
                  {formatDCR(principal, 5)} DCR
                </span>
              </div>
              <div className="flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full bg-amber-500" />
                <span className="text-muted-foreground">Fees</span>
                <span className="font-mono text-foreground/90">
                  {formatDCR(totals.fees, 5)} DCR
                </span>
              </div>
              {totals.sent > 0 && (
                <div className="text-[10px] text-muted-foreground pt-1">
                  Fees are {((totals.fees / totals.sent) * 100).toFixed(2)}% of sent
                </div>
              )}
            </div>
          </div>
        </SectionCard>
        <SectionCard title="Server RTT distribution" icon={Activity}>
          <QuantileBar quantiles={data.rmq_rtt_quantiles ?? []} />
        </SectionCard>
      </div>

      {/* Per-contact table */}
      <SectionCard title="By contact" icon={Users}>
        {sortedUsers.length === 0 ? (
          <p className="text-xs text-muted-foreground italic">
            No payments recorded yet.
          </p>
        ) : (
          <div className="space-y-1.5">
            {sortedUsers.map((u) => (
              <div
                key={u.uid}
                className={`w-full grid grid-cols-[1fr_auto] items-center rounded-lg transition-colors ${
                  openUid === u.uid ? 'bg-primary/10' : 'hover:bg-muted/20'
                }`}
              >
                <button
                  type="button"
                  onClick={() => setOpenUid(openUid === u.uid ? null : u.uid)}
                  className="min-w-0 grid grid-cols-[1fr_auto] gap-3 items-center px-3 py-2 text-left"
                >
                  <div className="min-w-0">
                    <div className="text-sm font-medium truncate">
                      {u.nick || u.uid.slice(0, 12)}
                    </div>
                    <div className="mt-1">
                      <MiniBars sent={u.sent_matoms} received={u.received_matoms} />
                    </div>
                  </div>
                  <div className="text-right text-[11px] tabular-nums shrink-0">
                    <div className="text-rose-400">{formatDCR(u.sent_matoms)}</div>
                    <div className="text-emerald-400">{formatDCR(u.received_matoms)}</div>
                  </div>
                </button>
                <button
                  type="button"
                  onClick={() => setClearUid(u.uid)}
                  title="Reset payment stats"
                  className="mr-2 px-2 py-1 rounded text-xs text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}
      </SectionCard>

      {/* Detail drawer */}
      {openUser && (
        <SectionCard
          title={`Breakdown for ${openUser.nick || openUser.uid.slice(0, 12)}`}
          icon={Layers}
          action={
            <button
              type="button"
              onClick={() => setOpenUid(null)}
              className="text-xs text-muted-foreground hover:text-foreground"
            >
              Close
            </button>
          }
        >
          <PaymentBreakdownDetail breakdowns={openUser.breakdowns ?? []} />
        </SectionCard>
      )}

      {clearUid && (
        <ConfirmActionModal
          title={`Reset payment stats for ${
            data.users.find((u) => u.uid === clearUid)?.nick || clearUid.slice(0, 12)
          }?`}
          body="Permanently clears the recorded sent, received, and fee totals plus the per-event breakdown for this contact. Funds, chat history, and the contact itself are not affected. The contact disappears from this list until new payments are recorded."
          confirmLabel="Reset stats"
          onClose={() => setClearUid(null)}
          onConfirm={() => clearBisonrelayPayStats(clearUid)}
          onSuccess={() => {
            if (openUid === clearUid) setOpenUid(null);
            void refresh();
          }}
        />
      )}
    </div>
  );
};

export const PaymentBreakdownDetail = ({
  breakdowns,
}: {
  breakdowns: BisonrelayPayStatsBreakdown[];
}) => {
  // Split into "sent" (positive cost we paid out) and "received" (negative
  // values in BR's convention) — but BR's PayStatsSummary uses positive
  // totals for both, with the prefix being the payment-event class. We
  // surface them as a single ranked list keyed by abs(total) since the
  // prefix string already tells the user the direction.
  const sorted = [...breakdowns].sort((a, b) => b.total - a.total);
  const max = sorted.length > 0 ? Math.max(...sorted.map((b) => Math.abs(b.total)), 1) : 1;

  if (sorted.length === 0) {
    return <p className="text-xs text-muted-foreground italic">No breakdown lines.</p>;
  }

  return (
    <div className="space-y-1.5">
      {sorted.map((b) => {
        const pct = (Math.abs(b.total) / max) * 100;
        return (
          <div key={b.prefix} className="grid grid-cols-[1fr_auto] gap-3 items-center">
            <div className="min-w-0">
              <div className="text-[11px] text-foreground/90 truncate">{b.prefix}</div>
              <div className="h-1.5 mt-0.5 rounded-full bg-muted/30 overflow-hidden">
                <div
                  className="h-full bg-primary/70"
                  style={{ width: `${pct}%` }}
                />
              </div>
            </div>
            <div className="text-[11px] font-mono tabular-nums">
              {formatDCR(b.total, 6)}
            </div>
          </div>
        );
      })}
    </div>
  );
};

// ---- 3) Network ----------------------------------------------------------

const NetworkView = () => {
  const { data, err } = usePolledStats(getBisonrelayStatsNetwork);

  if (err) return <ErrorBanner msg={err} />;
  if (!data) return <Loading what="network stats" />;

  const policy = data.policy ?? ({} as BisonrelayStatsNetwork['policy']);
  const ratePerMB =
    policy.push_pay_rate_bytes > 0
      ? (policy.push_pay_rate_matoms * (1024 * 1024)) / policy.push_pay_rate_bytes
      : 0;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <SectionCard title="Server" icon={Server}>
          <div className="space-y-3">
            <div>
              <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                LN node pubkey
              </div>
              <div className="font-mono text-[11px] text-foreground/90 break-all">
                {data.server_node || '–'}
              </div>
            </div>
            {data.recommended_peer && (
              <div>
                <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                  Recommended hub
                </div>
                <div className="font-mono text-[11px] text-foreground/90 break-all">
                  {data.recommended_peer}
                </div>
              </div>
            )}
            <div className="grid grid-cols-2 gap-3 pt-1">
              <div>
                <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                  Stage
                </div>
                <div className="text-sm font-semibold capitalize">
                  {data.stage.replace(/-/g, ' ')}
                </div>
              </div>
              <div>
                <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
                  Connected
                </div>
                <div className="text-sm font-mono">
                  {data.connected_at && isMeaningfulDate(data.connected_at)
                    ? relativeTime(data.connected_at)
                    : '–'}
                </div>
              </div>
            </div>
          </div>
        </SectionCard>

        <SectionCard title="Server policy" icon={Shield}>
          <div className="grid grid-cols-2 gap-3">
            <PolicyTile
              label="Push fee"
              value={`${formatDCR(ratePerMB, 6)} DCR / MB`}
              hint={`min ${formatDCR(policy.push_pay_rate_min_matoms, 8)}`}
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
        </SectionCard>
      </div>

      {data.queues && (
        <SectionCard title="Outbound queues" icon={Radio}>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <PolicyTile
              label="RMQ waiting"
              value={String(data.queues.rmq_waiting)}
              hint="messages queued to send"
            />
            <PolicyTile
              label="RMQ sending"
              value={String(data.queues.rmq_sending)}
              hint="being paid / sent / acked"
            />
            <PolicyTile
              label="Send queue"
              value={String(data.queues.sendq_items)}
              hint={`${data.queues.sendq_dests} destinations`}
            />
            <PolicyTile
              label="RV subscriptions"
              value={data.queues.rvs_up_to_date ? 'synced' : 'syncing'}
              hint="with the relay server"
            />
          </div>
          <p className="text-[10px] text-muted-foreground pt-2">
            Sustained queue buildup means messages are not reaching the relay
            server (connectivity or payment trouble).
          </p>
        </SectionCard>
      )}

      <SectionCard title="RMQ round-trip latency" icon={Radio}>
        <QuantileBar quantiles={data.rmq_quantiles ?? []} />
        <p className="text-[10px] text-muted-foreground pt-2">
          Distribution of round-trip times for messages sent to the relay server.
          Lower is better. Samples reset on reconnect.
        </p>
      </SectionCard>
    </div>
  );
};

export const PolicyTile = ({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) => (
  <div className="rounded-lg border border-border/50 bg-background/40 p-3">
    <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
      {label}
    </div>
    <div className="text-sm font-semibold text-foreground tabular-nums mt-0.5">
      {value}
    </div>
    {hint && <div className="text-[10px] text-muted-foreground mt-0.5">{hint}</div>}
  </div>
);

// ---- 4) Contacts --------------------------------------------------------

export type RatchetHealth = 'green' | 'amber' | 'red' | 'idle';

export const ratchetHealth = (c: BisonrelayStatsContact): RatchetHealth => {
  if (!c.ratchet) return 'idle';
  if (!c.ratchet.will_ratchet) return 'red';
  const lastTs =
    c.ratchet.last_dec_time && isMeaningfulDate(c.ratchet.last_dec_time)
      ? Date.parse(c.ratchet.last_dec_time)
      : 0;
  if (!lastTs) return 'amber';
  const ageDays = (Date.now() - lastTs) / 86400_000;
  if (ageDays > 30) return 'amber';
  return 'green';
};

const ContactsView = () => {
  const { data, err } = usePolledStats(getBisonrelayStatsContacts);
  const [openUid, setOpenUid] = useState<string | null>(null);

  if (err) return <ErrorBanner msg={err} />;
  if (!data) return <Loading what="contact stats" />;

  const sorted = [...data].sort((a, b) => {
    // Most-recently-decrypted first, fall back to first-created.
    const aTs = a.ratchet?.last_dec_time
      ? Date.parse(a.ratchet.last_dec_time)
      : 0;
    const bTs = b.ratchet?.last_dec_time
      ? Date.parse(b.ratchet.last_dec_time)
      : 0;
    if (bTs !== aTs) return bTs - aTs;
    return (Date.parse(b.first_created) || 0) - (Date.parse(a.first_created) || 0);
  });
  const openContact = sorted.find((c) => c.uid === openUid) ?? null;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-4 gap-3">
        <HeroCard icon={Users} label="Total" value={String(data.length)} />
        <HeroCard
          icon={CheckCircle2}
          label="Healthy"
          value={String(sorted.filter((c) => ratchetHealth(c) === 'green').length)}
          tone="emerald"
        />
        <HeroCard
          icon={AlertCircle}
          label="Idle / amber"
          value={String(sorted.filter((c) => ratchetHealth(c) === 'amber').length)}
          tone="amber"
        />
        <HeroCard
          icon={XCircle}
          label="Awaiting peer"
          value={String(sorted.filter((c) => ratchetHealth(c) === 'red').length)}
          tone="rose"
        />
      </div>

      <SectionCard title="Per-contact health" icon={Shield}>
        {sorted.length === 0 ? (
          <p className="text-xs text-muted-foreground italic">No contacts yet.</p>
        ) : (
          <div className="divide-y divide-border/30">
            {sorted.map((c) => {
              const health = ratchetHealth(c);
              const tone = {
                green: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30',
                amber: 'bg-amber-500/15 text-amber-400 border-amber-500/30',
                red: 'bg-rose-500/15 text-rose-400 border-rose-500/30',
                idle: 'bg-muted/30 text-muted-foreground border-border/50',
              }[health];
              const label = {
                green: 'Active',
                amber: 'Idle',
                red: 'Awaiting peer',
                idle: 'Offline',
              }[health];
              return (
                <button
                  key={c.uid}
                  type="button"
                  onClick={() => setOpenUid(openUid === c.uid ? null : c.uid)}
                  className={`w-full grid grid-cols-[1fr_auto_auto] gap-3 items-center px-2 py-2.5 text-left rounded-md transition-colors ${
                    openUid === c.uid ? 'bg-primary/10' : 'hover:bg-muted/20'
                  }`}
                >
                  <div className="min-w-0">
                    <div className="text-sm font-medium truncate">
                      {c.nick_alias || c.nick || c.uid.slice(0, 12)}
                    </div>
                    <div className="text-[10px] text-muted-foreground mt-0.5">
                      KX since {relativeTime(c.first_created)}
                    </div>
                  </div>
                  <span className="text-[10px] text-muted-foreground font-mono tabular-nums text-right whitespace-nowrap">
                    {c.ratchet && isMeaningfulDate(c.ratchet.last_enc_time) && (
                      <span className="block">sent {relativeTime(c.ratchet.last_enc_time)}</span>
                    )}
                    {c.ratchet && isMeaningfulDate(c.ratchet.last_dec_time) && (
                      <span className="block">heard {relativeTime(c.ratchet.last_dec_time)}</span>
                    )}
                    {(c.ratchet?.nb_saved_keys ?? 0) > 0 && (
                      <span className="block text-amber-400">
                        {c.ratchet?.nb_saved_keys} saved keys
                      </span>
                    )}
                  </span>
                  <span className="flex items-center gap-1.5">
                    {c.ignored && (
                      <span className="inline-flex items-center text-[10px] uppercase tracking-wide px-2 py-0.5 rounded-full border bg-muted/30 text-muted-foreground border-border/50">
                        Ignored
                      </span>
                    )}
                    <span
                      className={`inline-flex items-center gap-1 text-[10px] uppercase tracking-wide px-2 py-0.5 rounded-full border ${tone}`}
                    >
                      {label}
                    </span>
                  </span>
                </button>
              );
            })}
          </div>
        )}
      </SectionCard>

      {openContact && (
        <SectionCard
          title={`Ratchet details — ${openContact.nick_alias || openContact.nick || openContact.uid.slice(0, 12)}`}
          icon={Hash}
          action={
            <button
              type="button"
              onClick={() => setOpenUid(null)}
              className="text-xs text-muted-foreground hover:text-foreground"
            >
              Close
            </button>
          }
        >
          <RatchetDetail contact={openContact} />
        </SectionCard>
      )}
    </div>
  );
};

const RatchetDetail = ({ contact }: { contact: BisonrelayStatsContact }) => {
  const r = contact.ratchet;
  return (
    <div className="grid grid-cols-2 gap-3 text-xs">
      <Detail label="Identity" mono>{contact.uid}</Detail>
      <Detail label="Ignored">{contact.ignored ? 'Yes' : 'No'}</Detail>
      <Detail label="KX since">{relativeTime(contact.first_created)}</Detail>
      <Detail label="Last completed KX">
        {isMeaningfulDate(contact.last_completed_kx)
          ? relativeTime(contact.last_completed_kx)
          : '–'}
      </Detail>
      <Detail label="Last handshake attempt">
        {isMeaningfulDate(contact.last_handshake_attempt)
          ? relativeTime(contact.last_handshake_attempt!)
          : '–'}
      </Detail>
      <Detail label="Saved keys">{r ? `${r.nb_saved_keys}` : '–'}</Detail>
      <Detail label="Will ratchet">{r ? (r.will_ratchet ? 'Yes' : 'No') : '–'}</Detail>
      <Detail label="Last encrypted">
        {r?.last_enc_time && isMeaningfulDate(r.last_enc_time)
          ? relativeTime(r.last_enc_time)
          : '–'}
      </Detail>
      <Detail label="Last decrypted">
        {r?.last_dec_time && isMeaningfulDate(r.last_dec_time)
          ? relativeTime(r.last_dec_time)
          : '–'}
      </Detail>
      <Detail label="Send RV" mono>{r?.send_rv_plain || '–'}</Detail>
      <Detail label="Recv RV" mono>{r?.recv_rv_plain || '–'}</Detail>
      <Detail label="Drain RV" mono>{r?.drain_rv_plain || '–'}</Detail>
    </div>
  );
};

export const Detail = ({
  label,
  children,
  mono,
}: {
  label: string;
  children: React.ReactNode;
  mono?: boolean;
}) => (
  <div className="space-y-0.5">
    <div className="text-[10px] uppercase tracking-wide text-muted-foreground">
      {label}
    </div>
    <div
      className={`text-foreground/90 break-all ${
        mono ? 'font-mono text-[10px]' : 'text-xs'
      }`}
    >
      {children}
    </div>
  </div>
);

// ---- 5) Content ---------------------------------------------------------

const ContentView = () => {
  const { data, err } = usePolledStats(getBisonrelayStatsPosts);

  if (err) return <ErrorBanner msg={err} />;
  if (!data) return <Loading what="content stats" />;

  const totalHearts = data.authored.reduce((s, p) => s + p.hearts, 0);
  const totalComments = data.authored.reduce((s, p) => s + p.comments, 0);

  // Inline sort (no useMemo) so this stays above the early returns'
  // hook order. Authored posts list is small enough that recomputing
  // on each render is free.
  const ranked: BisonrelayAuthoredPostStats[] = [...data.authored].sort(
    (a, b) => b.hearts + b.comments - (a.hearts + a.comments),
  );

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <HeroCard icon={FileText} label="Posts authored" value={String(data.authored.length)} />
        <HeroCard
          icon={Atom}
          label="Atoms received"
          value={String(totalHearts)}
          tone="primary"
        />
        <HeroCard
          icon={MessageSquare}
          label="Comments received"
          value={String(totalComments)}
          tone="emerald"
        />
        <HeroCard
          icon={Inbox}
          label="Subscribers"
          value={String(data.subscribers_count)}
          hint={`You follow ${data.subscriptions_count}`}
        />
      </div>

      {(data.subscribers ?? []).length > 0 && (
        <SectionCard title="Your subscribers" icon={Inbox}>
          <div className="flex flex-wrap gap-2">
            {(data.subscribers ?? []).map((s) => (
              <span
                key={s.uid}
                title={s.uid}
                className="px-2.5 py-1 rounded-full bg-muted/20 text-xs text-foreground/90"
              >
                {s.nick || s.uid.slice(0, 12)}
              </span>
            ))}
          </div>
          <p className="text-[10px] text-muted-foreground pt-2">
            These contacts receive your new posts and relayed comments.
          </p>
        </SectionCard>
      )}

      <SectionCard title="Top posts by engagement" icon={TrendingUp}>
        {ranked.length === 0 ? (
          <p className="text-xs text-muted-foreground italic">
            You haven't published any posts yet.
          </p>
        ) : (
          <div className="space-y-1.5">
            {ranked.map((p) => {
              const max = Math.max(...ranked.map((q) => q.hearts + q.comments), 1);
              const score = p.hearts + p.comments;
              const pct = (score / max) * 100;
              return (
                <div
                  key={p.pid}
                  className="grid grid-cols-[1fr_auto] gap-3 items-center px-3 py-2 rounded-lg hover:bg-muted/20 transition-colors"
                >
                  <div className="min-w-0">
                    <div className="text-sm font-medium truncate">
                      {p.title || '(untitled post)'}
                    </div>
                    <div className="text-[10px] text-muted-foreground mt-0.5">
                      Published {relativeTime(p.date)}
                      {p.last_status_ts && isMeaningfulDate(p.last_status_ts) && (
                        <>
                          <span className="mx-1.5 opacity-50">·</span>
                          Last activity {relativeTime(p.last_status_ts)}
                        </>
                      )}
                    </div>
                    <div className="h-1.5 mt-1 rounded-full bg-muted/30 overflow-hidden">
                      <div
                        className="h-full bg-gradient-to-r from-rose-500/60 to-rose-500"
                        style={{ width: `${pct}%` }}
                      />
                    </div>
                  </div>
                  <div className="text-right shrink-0">
                    <div className="text-[11px] text-primary tabular-nums inline-flex items-center gap-1">
                      <Atom className="h-3 w-3" />
                      {p.hearts}
                    </div>
                    <div className="text-[11px] text-emerald-400 tabular-nums inline-flex items-center gap-1">
                      <MessageSquare className="h-3 w-3" />
                      {p.comments}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </SectionCard>
    </div>
  );
};

export type BisonrelayPaymentsTotalsRow = BisonrelayPayStatsUser;
export type BisonrelayStatsOverviewRow = BisonrelayStatsOverview;
export type BisonrelayStatsPaymentsRow = BisonrelayStatsPayments;
export type BisonrelayStatsNetworkRow = BisonrelayStatsNetwork;
export type BisonrelayStatsPostsRow = BisonrelayStatsPosts;
