// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Ticket, Activity, Lock, TrendingUp } from 'lucide-react';
import { StakingInfo } from '../services/api';

interface StakingStatsProps {
  data?: StakingInfo;
}

export const StakingStats = ({ data }: StakingStatsProps) => {
  const formatDCR = (amount: number) => {
    if (amount === 0) return 'N/A';
    return amount.toLocaleString('en-US', { 
      minimumFractionDigits: 2, 
      maximumFractionDigits: 2 
    });
  };

  const formatNumber = (num: number) => {
    if (num === 0) return 'N/A';
    return num.toLocaleString('en-US');
  };

  const formatPercent = (percent: number) => {
    if (percent === 0) return 'N/A';
    return percent.toFixed(2) + '%';
  };

  const stakingItems = [
    {
      label: 'Ticket Price',
      value: data ? (data.ticketPrice === 0 ? 'N/A' : `${formatDCR(data.ticketPrice)} DCR`) : 'Loading...',
      icon: Ticket,
      color: 'text-primary',
    },
    {
      label: 'Pool Size',
      value: data ? (data.poolSize === 0 ? 'N/A' : `${formatNumber(data.poolSize)} tickets`) : 'Loading...',
      icon: Activity,
      color: 'text-blue-500',
    },
    {
      label: 'Locked DCR',
      value: data ? (data.lockedDCR === 0 ? 'N/A' : `${formatDCR(data.lockedDCR)} DCR`) : 'Loading...',
      icon: Lock,
      color: 'text-purple-500',
    },
    {
      label: 'Participation Rate',
      value: data ? formatPercent(data.participationRate) : 'Loading...',
      icon: TrendingUp,
      color: 'text-success',
    },
  ];

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Activity className="h-6 w-6 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Staking Statistics</h3>
          <p className="text-sm text-muted-foreground">Network staking metrics</p>
        </div>
      </div>
      <div className="space-y-4">
        {stakingItems.map((item, index) => {
          const Icon = item.icon;
          return (
            <div key={index} className="flex items-center justify-between p-3 rounded-lg bg-muted/30 hover:bg-muted/50 transition-all duration-200">
              <div className="flex items-center gap-3">
                <div className={`p-2 rounded-lg bg-primary/10 ${item.color}`}>
                  <Icon className="w-4 h-4" />
                </div>
                <span className="text-sm text-muted-foreground">{item.label}</span>
              </div>
              <span className="font-medium">{item.value}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
};

