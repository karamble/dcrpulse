// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import {
  ArrowDownUp,
  Banknote,
  Coins,
  Hash,
  Landmark,
  Maximize2,
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
import { getAllTSpends, getTreasuryStats, TSpendRecord } from '../../services/treasuryStorage';
import { BalanceSample, getTreasuryBalanceHistory, getTreasuryInfo } from '../../services/treasuryApi';

const dcr = (v: number) =>
  v.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

// Vibrant chart palette using literal hsl() (recharts SVG attrs do NOT resolve
// `hsl(var(--x))`, so the theme tokens are inlined here as concrete colors).
const C = {
  spend: 'hsl(217 91% 62%)', // primary blue
  balance: 'hsl(173 80% 50%)', // teal
  year: 'hsl(265 90% 68%)', // violet
  inflow: 'hsl(150 75% 48%)', // green
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

const ChartCard = ({
  icon: Icon,
  title,
  caption,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  caption?: string;
  children: React.ReactNode;
}) => (
  <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
    <div className="flex items-center gap-2 mb-4">
      <Icon className="h-5 w-5 text-primary" />
      <h3 className="text-lg font-semibold">{title}</h3>
    </div>
    <div className="h-64">{children}</div>
    {caption && <p className="text-xs text-muted-foreground mt-3">{caption}</p>}
  </div>
);

// Balance at the end of the given calendar year, taken from the nearest sample
// on or before Dec 31 of that year (0 before the series starts).
const balanceAtYearEnd = (series: BalanceSample[], year: number): number => {
  const cutoff = Date.UTC(year + 1, 0, 1) / 1000;
  let bal = 0;
  for (const s of series) {
    if (s.time < cutoff) bal = s.balance;
    else break;
  }
  return bal;
};

export const TreasuryStats = () => {
  const [tspends, setTspends] = useState<TSpendRecord[]>(() => getAllTSpends());
  const [stats, setStats] = useState(() => getTreasuryStats());
  const [balance, setBalance] = useState<number | null>(null);
  const [series, setSeries] = useState<BalanceSample[]>([]);

  useEffect(() => {
    const refreshLocal = () => {
      setTspends(getAllTSpends());
      setStats(getTreasuryStats());
    };
    refreshLocal();
    getTreasuryInfo()
      .then((i) => setBalance(i.balance))
      .catch(() => {});
    getTreasuryBalanceHistory()
      .then(setSeries)
      .catch(() => {});
    const id = window.setInterval(refreshLocal, 30000);
    return () => window.clearInterval(id);
  }, []);

  const largest = useMemo(() => tspends.reduce((m, t) => Math.max(m, t.amount), 0), [tspends]);
  const thisYearSpent = useMemo(() => {
    const y = new Date().getUTCFullYear();
    return tspends
      .filter((t) => new Date(t.timestamp).getUTCFullYear() === y)
      .reduce((s, t) => s + t.amount, 0);
  }, [tspends]);

  const cumulativeData = useMemo(() => {
    const sorted = [...tspends].sort(
      (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
    );
    let running = 0;
    return sorted.map((t) => {
      running += t.amount;
      return { date: t.timestamp.slice(0, 10), cumulative: running };
    });
  }, [tspends]);

  const perYearData = useMemo(() => {
    const byYear = new Map<string, { amount: number; count: number }>();
    for (const t of tspends) {
      const y = String(new Date(t.timestamp).getUTCFullYear());
      const e = byYear.get(y) ?? { amount: 0, count: 0 };
      e.amount += t.amount;
      e.count += 1;
      byYear.set(y, e);
    }
    return Array.from(byYear, ([year, v]) => ({ year, amount: v.amount, count: v.count })).sort(
      (a, b) => a.year.localeCompare(b.year),
    );
  }, [tspends]);

  const balanceData = useMemo(
    () =>
      series.map((s) => ({
        date: new Date(s.time * 1000).toISOString().slice(0, 7),
        balance: s.balance,
      })),
    [series],
  );

  const flowData = useMemo(() => {
    if (series.length === 0) return [];
    return perYearData.map((p) => {
      const y = Number(p.year);
      const outflow = p.amount;
      const net = balanceAtYearEnd(series, y) - balanceAtYearEnd(series, y - 1);
      const inflow = Math.max(0, net + outflow);
      return { year: String(y), inflow, outflow };
    });
  }, [series, perYearData]);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 lg:grid-cols-3 gap-4">
        <StatCard
          label="Treasury Balance"
          value={balance === null ? '…' : `${dcr(balance)} DCR`}
          sub="Current on-chain balance"
          icon={<Landmark className="h-4 w-4 text-primary" />}
        />
        <StatCard
          label="Total Spent"
          value={`${dcr(stats.totalSpent)} DCR`}
          sub="Lifetime, all TSpends"
          icon={<TrendingUp className="h-4 w-4 text-warning" />}
          tone="warning"
        />
        <StatCard
          label="Payments"
          value={stats.count.toLocaleString()}
          sub="Approved TSpends"
          icon={<Hash className="h-4 w-4 text-primary" />}
        />
        <StatCard
          label="Average Payment"
          value={`${dcr(stats.averageAmount)} DCR`}
          sub="Total spent / count"
          icon={<Coins className="h-4 w-4 text-primary" />}
        />
        <StatCard
          label="Largest Payment"
          value={`${dcr(largest)} DCR`}
          sub="Single biggest TSpend"
          icon={<Maximize2 className="h-4 w-4 text-primary" />}
        />
        <StatCard
          label="Spent This Year"
          value={`${dcr(thisYearSpent)} DCR`}
          sub={String(new Date().getUTCFullYear())}
          icon={<Banknote className="h-4 w-4 text-primary" />}
        />
      </div>

      {tspends.length === 0 ? (
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
          No TSpend history yet. Run the historical scan to populate statistics.
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
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
                />
              </AreaChart>
            </ResponsiveContainer>
          </ChartCard>

          <ChartCard icon={Banknote} title="Spend per Year">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={perYearData} margin={{ top: 10, right: 20, bottom: 0, left: 0 }}>
                <defs>{fadeGrad('gYear', C.year, 0.95)}</defs>
                <CartesianGrid strokeDasharray="3 3" stroke={gridStroke} />
                <XAxis dataKey="year" tick={axisTick} stroke="rgba(148,163,184,0.3)" />
                <YAxis tick={axisTick} stroke="rgba(148,163,184,0.3)" width={70} />
                <Tooltip
                  cursor={{ fill: 'rgba(148,163,184,0.08)' }}
                  contentStyle={tooltipStyle}
                  formatter={(v: number, _n, p: any) => [
                    `${dcr(v)} DCR (${p?.payload?.count ?? 0} payments)`,
                    'Spent',
                  ]}
                />
                <Bar dataKey="amount" fill="url(#gYear)" stroke={C.year} radius={[6, 6, 0, 0]} />
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
                  />
                </AreaChart>
              </ResponsiveContainer>
            </ChartCard>
          )}

          {flowData.length > 0 && (
            <ChartCard
              icon={ArrowDownUp}
              title="Inflow vs Outflow per Year"
              caption="Inflow is derived from the net balance change plus outflow (not a separate TAdds scan)."
            >
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={flowData} margin={{ top: 10, right: 20, bottom: 0, left: 0 }}>
                  <defs>
                    {fadeGrad('gIn', C.inflow, 0.95)}
                    {fadeGrad('gOut', C.outflow, 0.95)}
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke={gridStroke} />
                  <XAxis dataKey="year" tick={axisTick} stroke="rgba(148,163,184,0.3)" />
                  <YAxis tick={axisTick} stroke="rgba(148,163,184,0.3)" width={70} />
                  <Tooltip
                    cursor={{ fill: 'rgba(148,163,184,0.08)' }}
                    contentStyle={tooltipStyle}
                    formatter={(v: number, n: string) => [`${dcr(v)} DCR`, n]}
                  />
                  <Legend wrapperStyle={{ fontSize: 12 }} />
                  <Bar dataKey="inflow" name="Inflow" fill="url(#gIn)" stroke={C.inflow} radius={[6, 6, 0, 0]} />
                  <Bar dataKey="outflow" name="Outflow" fill="url(#gOut)" stroke={C.outflow} radius={[6, 6, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </ChartCard>
          )}
        </div>
      )}
    </div>
  );
};
