// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { Landmark } from 'lucide-react';
import { Area, AreaChart, ResponsiveContainer } from 'recharts';
import { BalanceSample, getTreasuryBalanceHistory, getTreasuryInfo } from '../../services/treasuryApi';
import { getBisonrelayRates } from '../../services/bisonrelayApi';

const usdCompact = (v: number) =>
  new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    notation: 'compact',
    maximumFractionDigits: 2,
  }).format(v);

const dcr = (v: number) =>
  v.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

const ago = (rfc3339: string): string => {
  if (!rfc3339) return '';
  const t = Date.parse(rfc3339);
  if (!Number.isFinite(t)) return '';
  const s = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (s < 60) return 'just now';
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  return h < 24 ? `${h}h ago` : `${Math.floor(h / 24)}d ago`;
};

export const TreasuryValueCard = () => {
  const [balance, setBalance] = useState<number | null>(null);
  const [rate, setRate] = useState<{ usd: number; source: string; at: string } | null>(null);
  const [series, setSeries] = useState<BalanceSample[]>([]);

  useEffect(() => {
    const load = () => {
      getTreasuryInfo()
        .then((i) => setBalance(i.balance))
        .catch(() => {});
      getBisonrelayRates()
        .then((r) => setRate({ usd: r.dcr_usd, source: r.source, at: r.updated_at }))
        .catch(() => {});
    };
    load();
    getTreasuryBalanceHistory()
      .then(setSeries)
      .catch(() => {});
    const id = window.setInterval(load, 60000);
    return () => window.clearInterval(id);
  }, []);

  const hasRate = !!rate && rate.usd > 0;
  const usd = balance !== null && hasRate ? balance * rate!.usd : null;
  const spark = useMemo(() => series.map((s) => ({ balance: s.balance })), [series]);

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 flex flex-col animate-fade-in">
      <div className="flex items-start justify-between">
        <div className="min-w-0">
          <p className="text-sm text-muted-foreground mb-1">Treasury Value</p>
          <h3 className="text-3xl font-bold">
            {usd !== null
              ? usdCompact(usd)
              : balance !== null
                ? `${dcr(balance)} DCR`
                : '…'}
          </h3>
          <p className="text-xs text-muted-foreground mt-1">
            {hasRate
              ? `@ $${rate!.usd.toFixed(2)} / DCR${rate!.source ? ` · ${rate!.source}` : ''}${
                  ago(rate!.at) ? ` · ${ago(rate!.at)}` : ''
                }`
              : 'USD rate unavailable'}
          </p>
        </div>
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20 shrink-0">
          <Landmark className="h-5 w-5 text-primary" />
        </div>
      </div>

      {spark.length > 1 && (
        <div className="h-20 mt-4 -mx-1">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={spark} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
              <defs>
                <linearGradient id="gTreasuryVal" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="hsl(173 80% 50%)" stopOpacity={0.5} />
                  <stop offset="100%" stopColor="hsl(173 80% 50%)" stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <Area
                type="monotone"
                dataKey="balance"
                stroke="hsl(173 80% 50%)"
                strokeWidth={2}
                fill="url(#gTreasuryVal)"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}

      {balance !== null && (
        <p className="text-xs text-muted-foreground mt-3">{dcr(balance)} DCR on-chain</p>
      )}
    </div>
  );
};
