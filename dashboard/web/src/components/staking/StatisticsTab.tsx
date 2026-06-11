// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, Award, Coins, Lock, Percent, Ticket, TrendingUp, XCircle } from 'lucide-react';
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { TicketRecord, listTickets } from '../../services/api';

const formatDcr = (v: number) => v.toFixed(8);
const formatDcr4 = (v: number) => v.toFixed(4);

interface StatCardProps {
  label: string;
  value: string;
  sub?: string;
  icon: React.ReactNode;
  tone?: 'default' | 'success' | 'warning' | 'destructive';
}

const StatCard = ({ label, value, sub, icon, tone = 'default' }: StatCardProps) => {
  const toneClass =
    tone === 'success'
      ? 'text-success'
      : tone === 'warning'
        ? 'text-warning'
        : tone === 'destructive'
          ? 'text-destructive'
          : '';
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

export const StatisticsTab = () => {
  const [tickets, setTickets] = useState<TicketRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const load = async () => {
      try {
        const list = await listTickets();
        setTickets(list);
        setError(null);
      } catch (err: any) {
        setError(err?.message || 'Failed to load tickets');
      } finally {
        setLoading(false);
      }
    };
    load();
    const id = window.setInterval(load, 30000);
    return () => window.clearInterval(id);
  }, []);

  const stats = useMemo(() => {
    const bought = tickets.length;
    const voted = tickets.filter((t) => t.status === 'VOTED');
    const revokedOrMissed = tickets.filter((t) => t.status === 'REVOKED' || t.status === 'MISSED');
    const active = tickets.filter(
      (t) => t.status === 'UNMINED' || t.status === 'IMMATURE' || t.status === 'LIVE',
    );
    const totalReward = voted.reduce((s, t) => s + t.reward, 0);
    const totalCommitted = active.reduce((s, t) => s + t.ticketPrice, 0);
    const denom = voted.length + revokedOrMissed.length;
    const voteSuccess = denom > 0 ? (voted.length / denom) * 100 : null;
    const avgReward = voted.length > 0 ? totalReward / voted.length : null;
    return {
      bought,
      voted: voted.length,
      revokedOrMissed: revokedOrMissed.length,
      voteSuccess,
      totalReward,
      totalCommitted,
      avgReward,
      votedRecords: voted,
    };
  }, [tickets]);

  const chartData = useMemo(() => {
    if (stats.votedRecords.length === 0) return [];
    const sorted = [...stats.votedRecords]
      .filter((t) => t.spenderTime > 0)
      .sort((a, b) => a.spenderTime - b.spenderTime);
    // Bucket by UTC day, accumulate.
    const byDay = new Map<string, number>();
    let running = 0;
    for (const t of sorted) {
      running += t.reward;
      const day = new Date(t.spenderTime * 1000).toISOString().slice(0, 10);
      byDay.set(day, running);
    }
    return Array.from(byDay, ([day, cumulative]) => ({ day, cumulative }));
  }, [stats.votedRecords]);

  return (
    <div className="space-y-6">
      {loading && tickets.length === 0 && (
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
          Loading statistics…
        </div>
      )}
      {error && (
        <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-center gap-2">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        <StatCard
          label="Tickets Bought"
          value={stats.bought.toLocaleString()}
          sub="All-time, all statuses"
          icon={<Ticket className="h-4 w-4 text-primary" />}
        />
        <StatCard
          label="Tickets Voted"
          value={stats.voted.toLocaleString()}
          sub="Completed PoS votes"
          icon={<Award className="h-4 w-4 text-success" />}
          tone="success"
        />
        <StatCard
          label="Revoked + Missed"
          value={stats.revokedOrMissed.toLocaleString()}
          sub="Did not vote successfully"
          icon={<XCircle className="h-4 w-4 text-destructive" />}
          tone="destructive"
        />
        <StatCard
          label="Vote Success"
          value={stats.voteSuccess === null ? '-' : `${stats.voteSuccess.toFixed(2)}%`}
          sub="voted / (voted + revoked + missed)"
          icon={<Percent className="h-4 w-4 text-primary" />}
        />
        <StatCard
          label="Total Reward"
          value={`${formatDcr4(stats.totalReward)} DCR`}
          sub="Sum across all voted tickets"
          icon={<TrendingUp className="h-4 w-4 text-success" />}
          tone="success"
        />
        <StatCard
          label="Total Stake Committed"
          value={`${formatDcr4(stats.totalCommitted)} DCR`}
          sub="Currently locked in active tickets"
          icon={<Lock className="h-4 w-4 text-primary" />}
        />
        <StatCard
          label="Avg Reward per Vote"
          value={stats.avgReward === null ? '-' : `${formatDcr(stats.avgReward)} DCR`}
          sub="Total reward / tickets voted"
          icon={<Coins className="h-4 w-4 text-primary" />}
        />
      </div>

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
        <div className="flex items-center gap-2 mb-4">
          <TrendingUp className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Cumulative Stake Reward</h3>
        </div>
        {chartData.length === 0 ? (
          <div className="text-sm text-muted-foreground">
            No voted tickets yet. Vote rewards will appear here as tickets vote.
          </div>
        ) : (
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData} margin={{ top: 10, right: 20, bottom: 0, left: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.06)" />
                <XAxis dataKey="day" stroke="rgba(255,255,255,0.5)" fontSize={11} />
                <YAxis stroke="rgba(255,255,255,0.5)" fontSize={11} width={70} />
                <Tooltip
                  contentStyle={{
                    background: 'rgba(20,20,28,0.95)',
                    border: '1px solid rgba(255,255,255,0.1)',
                    borderRadius: 8,
                    fontSize: 12,
                  }}
                  formatter={(v: number) => [`${v.toFixed(8)} DCR`, 'Cumulative']}
                />
                <Line
                  type="monotone"
                  dataKey="cumulative"
                  stroke="hsl(217 91% 62%)"
                  strokeWidth={2.5}
                  dot={false}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>
    </div>
  );
};
