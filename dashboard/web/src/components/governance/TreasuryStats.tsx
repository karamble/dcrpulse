// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import {
  ArrowDownUp,
  Banknote,
  Coins,
  Gift,
  Hash,
  Hourglass,
  Landmark,
  TrendingUp,
} from 'lucide-react';
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import {
  getAllTAdds,
  getAllTSpends,
  getTBaseByMonth,
  getTreasuryStats,
  TAddRecord,
  TSpendRecord,
} from '../../services/treasuryStorage';
import {
  BalanceSample,
  getTreasuryOutlook,
  getTreasuryRunway,
  TreasuryOutlook,
  TreasuryRunway,
} from '../../services/treasuryApi';
import { getDexRates } from '../../services/dcrdexApi';
import { flowsByMonth, monthlyRows, recentSpend, yearlyRows } from '../../services/treasuryFlows';
import { parseDcrAmount, toDcr } from '../../utils/amounts';
import { useVisiblePoll } from '../../hooks/useVisiblePoll';

const dcr = (v: number) =>
  v.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

// runwayText renders whole months as years and months.
export const runwayText = (months: number): string => {
  const y = Math.floor(months / 12);
  const m = months % 12;
  const parts = [y > 0 ? `${y} year${y === 1 ? '' : 's'}` : '', m > 0 || y === 0 ? `${m} month${m === 1 ? '' : 's'}` : ''];
  return parts.filter(Boolean).join(' ');
};

const usd = (dcrAmount: number, rate: number | null) =>
  rate === null ? undefined : `$${Math.round(dcrAmount * rate).toLocaleString('en-US')} at today's rate`;

// Vibrant chart palette using literal hsl() (recharts SVG attrs do NOT resolve
// `hsl(var(--x))`, so the theme tokens are inlined here as concrete colors).
const C = {
  spend: 'hsl(217 91% 62%)', // primary blue
  balance: 'hsl(173 80% 50%)', // teal
  year: 'hsl(265 90% 68%)', // violet
  inflow: 'hsl(150 75% 48%)', // green
  contribution: 'hsl(45 95% 55%)', // amber
  outflow: 'hsl(350 90% 63%)', // rose
};

const axisTick = { fill: 'rgba(226,232,240,0.7)', fontSize: 11 } as const;
const gridStroke = 'rgba(148,163,184,0.12)';
const tooltipStyle = {
  background: 'rgba(15,18,28,0.96)',
  border: '1px solid rgba(148,163,184,0.25)',
  borderRadius: 10,
  fontSize: 12,
  boxShadow: '0 8px 30px rgba(0,0,0,0.5)',
} as const;

// Vertical fade gradient (bright top -> transparent bottom) for area/bar fills.
const fadeGrad = (id: string, color: string, top = 0.55) => (
  <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
    <stop offset="0%" stopColor={color} stopOpacity={top} />
    <stop offset="100%" stopColor={color} stopOpacity={0.02} />
  </linearGradient>
);

interface StatCardProps {
  label: string;
  value: string;
  sub?: string;
  icon: React.ReactNode;
  tone?: 'default' | 'success' | 'warning';
}

const StatCard = ({ label, value, sub, icon, tone = 'default' }: StatCardProps) => {
  const toneClass = tone === 'success' ? 'text-success' : tone === 'warning' ? 'text-warning' : '';
  return (
    <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm text-muted-foreground font-medium">{label}</span>
        {icon}
      </div>
      <div className={`text-2xl font-bold ${toneClass}`}>{value}</div>
      {sub && <div className="text-xs text-muted-foreground mt-1">{sub}</div>}
    </div>
  );
};

// RunwayCard projects how long the treasury lasts at a monthly spend: the
// measured twelve-month average, or an amount the viewer types.
export const RunwayCard = ({ measuredAtoms }: { measuredAtoms: number }) => {
  const [input, setInput] = useState('');
  const [run, setRun] = useState<TreasuryRunway | null>(null);
  const typed = input.trim() === '' ? null : parseDcrAmount(input);
  const spendAtoms = typed === null ? measuredAtoms : typed.error ? null : typed.atoms;

  useEffect(() => {
    if (spendAtoms === null || spendAtoms <= 0) return;
    let current = true;
    const timer = setTimeout(() => {
      getTreasuryRunway(spendAtoms)
        .then((r) => current && setRun(r))
        .catch(() => {});
    }, 400);
    return () => {
      current = false;
      clearTimeout(timer);
    };
  }, [spendAtoms]);

  const idle = typed === null && measuredAtoms <= 0;
  const value = idle
    ? 'Not spending'
    : run === null
      ? '…'
      : run.beyond
        ? `${run.projectionMonths / 12}+ years`
        : runwayText(run.months);
  const source = typed === null ? "the last 12 months' average" : 'your amount';
  return (
    <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm text-muted-foreground font-medium">Runway</span>
        <Hourglass className="h-4 w-4 text-primary" />
      </div>
      <div className="text-2xl font-bold">{value}</div>
      <div className="text-xs text-muted-foreground mt-1">
        {idle
          ? 'Nothing spent in the last 12 full months'
          : run === null
            ? null
            : `${run.exhaustedMonth ? `Until ${run.exhaustedMonth}, at` : 'At'} ${dcr(toDcr(run.monthlySpendAtoms))} DCR a month (${source}), with the block reward shrinking on dcrd's schedule; next month nets ${run.firstMonthNetAtoms >= 0 ? '+' : ''}${dcr(toDcr(run.firstMonthNetAtoms))} DCR`}
      </div>
      <div className="mt-2 flex items-center gap-2">
        <input
          aria-label="Monthly spend in DCR"
          inputMode="decimal"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={measuredAtoms > 0 ? `${dcr(toDcr(measuredAtoms))} (last 12 months)` : 'Monthly spend, DCR'}
          className="w-full min-w-0 bg-background border border-border rounded-md px-2 py-1 text-xs text-foreground focus:outline-none focus:border-primary"
        />
        {typed !== null && (
          <button type="button" onClick={() => setInput('')} className="text-xs text-primary whitespace-nowrap hover:underline">
            Use measured
          </button>
        )}
      </div>
      {typed?.error && <div className="text-xs text-destructive mt-1">{typed.error}</div>}
    </div>
  );
};

// Series of the inflow vs outflow chart the legend can hide.
type FlowSeries = 'blockReward' | 'contributions' | 'spends';

const ChartCard = ({
  icon: Icon,
  title,
  caption,
  action,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  caption?: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}) => (
  <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
    <div className="flex items-center gap-2 mb-4">
      <Icon className="h-5 w-5 text-primary" />
      <h3 className="text-lg font-semibold">{title}</h3>
      {action && <div className="ml-auto">{action}</div>}
    </div>
    <div className="h-64">{children}</div>
    {caption && <p className="text-xs text-muted-foreground mt-3">{caption}</p>}
  </div>
);

interface TreasuryStatsProps {
  balanceAtoms: number | null;
  series: BalanceSample[];
  // Bumped when a scan or snapshot sync writes new records to localStorage.
  refreshKey?: number;
}

// The balance and series come from GovernanceDashboard's shared treasury poll;
// every flow comes from the records the scans stored.
export const TreasuryStats = ({ balanceAtoms, series, refreshKey = 0 }: TreasuryStatsProps) => {
  const [tspends, setTspends] = useState<TSpendRecord[]>(() => getAllTSpends());
  const [tadds, setTadds] = useState<TAddRecord[]>(() => getAllTAdds());
  const [tbase, setTbase] = useState<Record<string, number>>(() => getTBaseByMonth());
  const [stats, setStats] = useState(() => getTreasuryStats());
  const [rate, setRate] = useState<number | null>(null);
  const [outlook, setOutlook] = useState<TreasuryOutlook | null>(null);
  // 'all' shows the per-year flow chart; a year shows that year's monthly view.
  const [flowYear, setFlowYear] = useState('all');

  const refreshLocal = () => {
    setTspends(getAllTSpends());
    setTadds(getAllTAdds());
    setTbase(getTBaseByMonth());
    setStats(getTreasuryStats());
  };
  useEffect(() => {
    refreshLocal();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshKey]);
  useVisiblePoll(refreshLocal, 30000, { immediate: false });
  useEffect(() => {
    getDexRates()
      .then((r) => setRate(r.dcr > 0 ? r.dcr : null))
      .catch(() => {});
    getTreasuryOutlook().then(setOutlook).catch(() => {});
  }, []);

  const byMonth = useMemo(() => flowsByMonth(tspends, tadds, tbase), [tspends, tadds, tbase]);
  const yearly = useMemo(() => yearlyRows(byMonth, new Date()), [byMonth]);
  const flowRows = useMemo(
    () =>
      (flowYear === 'all' ? yearly : monthlyRows(byMonth, Number(flowYear), new Date())).map((r) => ({
        label: r.label,
        blockReward: toDcr(r.blockRewardAtoms),
        contributions: toDcr(r.contributionsAtoms),
        spends: toDcr(r.spendsAtoms),
      })),
    [flowYear, yearly, byMonth],
  );
  const spent12 = useMemo(() => recentSpend(tspends, new Date()), [tspends]);
  const [hidden, setHidden] = useState<Set<FlowSeries>>(() => new Set());
  const toggleSeries = (key: FlowSeries) =>
    setHidden((prev) => {
      const next = new Set(prev);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  const contributedAtoms = useMemo(() => tadds.reduce((s, t) => s + t.amountAtoms, 0), [tadds]);
  const outlookAtoms = outlook?.months.reduce((s, m) => s + m.tbaseAtoms, 0) ?? null;

  const cumulativeData = useMemo(() => {
    const sorted = [...tspends].sort(
      (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
    );
    let running = 0;
    return sorted.map((t) => {
      running += t.amountAtoms + t.feeAtoms;
      return { date: t.timestamp.slice(0, 10), cumulative: toDcr(running) };
    });
  }, [tspends]);

  const balanceData = useMemo(
    () =>
      series.map((s) => ({
        date: new Date(s.time * 1000).toISOString().slice(0, 7),
        balance: s.balance,
      })),
    [series],
  );

  const balanceDcr = balanceAtoms === null ? null : toDcr(balanceAtoms);
  const spentDcr = toDcr(stats.totalSpentAtoms);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 lg:grid-cols-3 gap-4">
        <StatCard
          label="Treasury Balance"
          value={balanceDcr === null ? '…' : `${dcr(balanceDcr)} DCR`}
          sub={balanceDcr === null ? 'Current on-chain balance' : usd(balanceDcr, rate) ?? 'Current on-chain balance'}
          icon={<Landmark className="h-4 w-4 text-primary" />}
        />
        <RunwayCard measuredAtoms={spent12.monthlyAtoms} />
        <StatCard
          label="Spent, Last 12 Months"
          value={`${dcr(toDcr(spent12.outflowAtoms))} DCR`}
          sub={usd(toDcr(spent12.outflowAtoms), rate) ?? 'Payments plus their fees'}
          icon={<Banknote className="h-4 w-4 text-warning" />}
          tone="warning"
        />
        <StatCard
          label="Block Reward, Next 12 Months"
          value={outlookAtoms === null ? '…' : `${dcr(toDcr(outlookAtoms))} DCR`}
          sub={outlook ? `Projected at ${outlook.targetBlockSeconds / 60}-minute blocks` : 'Projected'}
          icon={<Coins className="h-4 w-4 text-success" />}
          tone="success"
        />
        <StatCard
          label="Total Spent"
          value={`${dcr(spentDcr)} DCR`}
          sub={`${stats.count.toLocaleString()} payments, fees included`}
          icon={<Hash className="h-4 w-4 text-primary" />}
        />
        <StatCard
          label="Contributions"
          value={`${dcr(toDcr(contributedAtoms))} DCR`}
          sub={`${tadds.length.toLocaleString()} voluntary, beyond the block reward`}
          icon={<Gift className="h-4 w-4 text-primary" />}
        />
      </div>

      {stats.lastSyncHeight === 0 ? (
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
          No treasury history yet. Scan new blocks to populate statistics.
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <ChartCard
            icon={ArrowDownUp}
            title={flowYear === 'all' ? 'Inflow vs Outflow per Year' : `Inflow vs Outflow ${flowYear}`}
            caption={`Block reward, contributions and spends (fees included) as recorded in each block, through block ${stats.lastSyncHeight.toLocaleString()}.`}
            action={
              <select
                value={flowYear}
                onChange={(e) => setFlowYear(e.target.value)}
                className="bg-background border border-border rounded-lg px-2 py-1 text-xs text-foreground focus:outline-none focus:border-primary"
              >
                <option value="all">All time</option>
                {yearly.map((r) => (
                  <option key={r.label} value={r.label}>
                    {r.label}
                  </option>
                ))}
              </select>
            }
          >
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={flowRows} margin={{ top: 10, right: 20, bottom: 0, left: 0 }}>
                <defs>
                  {fadeGrad('gIn', C.inflow, 0.95)}
                  {fadeGrad('gAdd', C.contribution, 0.95)}
                  {fadeGrad('gOut', C.outflow, 0.95)}
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke={gridStroke} />
                <XAxis dataKey="label" tick={axisTick} stroke="rgba(148,163,184,0.3)" />
                <YAxis tick={axisTick} stroke="rgba(148,163,184,0.3)" width={70} />
                <Tooltip
                  cursor={{ fill: 'rgba(148,163,184,0.08)' }}
                  contentStyle={tooltipStyle}
                  formatter={(v: number, n: string) => [`${dcr(v)} DCR`, n]}
                />
                <Legend
                  wrapperStyle={{ fontSize: 12, cursor: 'pointer' }}
                  onClick={(e) => toggleSeries(e.dataKey as FlowSeries)}
                  formatter={(name: string, e) => (
                    <span style={hidden.has(e.dataKey as FlowSeries) ? { opacity: 0.4, textDecoration: 'line-through' } : undefined}>
                      {name}
                    </span>
                  )}
                />
                <Bar
                  dataKey="blockReward"
                  name="Block reward"
                  stackId="in"
                  hide={hidden.has('blockReward')}
                  fill="url(#gIn)"
                  stroke={C.inflow}
                  radius={hidden.has('contributions') ? [6, 6, 0, 0] : undefined}
                  isAnimationActive={false}
                />
                <Bar dataKey="contributions" name="Contributions" stackId="in" hide={hidden.has('contributions')} fill="url(#gAdd)" stroke={C.contribution} radius={[6, 6, 0, 0]} isAnimationActive={false} />
                <Bar dataKey="spends" name="Spends" hide={hidden.has('spends')} fill="url(#gOut)" stroke={C.outflow} radius={[6, 6, 0, 0]} isAnimationActive={false} />
              </BarChart>
            </ResponsiveContainer>
          </ChartCard>

          {balanceData.length > 0 && (
            <ChartCard icon={Landmark} title="Treasury Balance Over Time">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={balanceData} margin={{ top: 10, right: 20, bottom: 0, left: 0 }}>
                  <defs>{fadeGrad('gBal', C.balance)}</defs>
                  <CartesianGrid strokeDasharray="3 3" stroke={gridStroke} />
                  <XAxis dataKey="date" tick={axisTick} stroke="rgba(148,163,184,0.3)" />
                  <YAxis tick={axisTick} stroke="rgba(148,163,184,0.3)" width={70} />
                  <Tooltip
                    contentStyle={tooltipStyle}
                    formatter={(v: number) => [`${dcr(v)} DCR`, 'Balance']}
                  />
                  <Area
                    type="monotone"
                    dataKey="balance"
                    stroke={C.balance}
                    strokeWidth={2.5}
                    fill="url(#gBal)"
                    isAnimationActive={false}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </ChartCard>
          )}

          {cumulativeData.length > 0 && (
            <ChartCard icon={TrendingUp} title="Cumulative Treasury Spend">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={cumulativeData} margin={{ top: 10, right: 20, bottom: 0, left: 0 }}>
                  <defs>{fadeGrad('gSpend', C.spend)}</defs>
                  <CartesianGrid strokeDasharray="3 3" stroke={gridStroke} />
                  <XAxis dataKey="date" tick={axisTick} stroke="rgba(148,163,184,0.3)" />
                  <YAxis tick={axisTick} stroke="rgba(148,163,184,0.3)" width={70} />
                  <Tooltip
                    contentStyle={tooltipStyle}
                    formatter={(v: number) => [`${dcr(v)} DCR`, 'Cumulative']}
                  />
                  <Area
                    type="monotone"
                    dataKey="cumulative"
                    stroke={C.spend}
                    strokeWidth={2.5}
                    fill="url(#gSpend)"
                    isAnimationActive={false}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </ChartCard>
          )}
        </div>
      )}
    </div>
  );
};
