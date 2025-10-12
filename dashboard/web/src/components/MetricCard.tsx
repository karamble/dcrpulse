// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { LucideIcon } from 'lucide-react';
import { ReactNode } from 'react';

interface MetricCardProps {
  title: string;
  value: string | number;
  subtitle?: string | ReactNode;
  icon: LucideIcon;
  trend?: {
    value: string;
    isPositive: boolean;
  };
  usdValue?: string;
}

export const MetricCard = ({ title, value, subtitle, icon: Icon, trend, usdValue }: MetricCardProps) => {
  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 hover:border-primary/20 transition-all duration-300 group animate-fade-in">
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <p className="text-sm text-muted-foreground mb-1">{title}</p>
          <h3 className="text-3xl font-bold mb-1 group-hover:text-primary transition-colors">
            {value}
          </h3>
          {subtitle && (
            <p className="text-xs text-muted-foreground">{subtitle}</p>
          )}
          {usdValue && (
            <p className="text-sm font-medium text-success mt-2">{usdValue}</p>
          )}
          {trend && (
            <div className={`flex items-center gap-1 mt-2 text-xs ${trend.isPositive ? 'text-success' : 'text-red-500'}`}>
              <span>{trend.value}</span>
            </div>
          )}
        </div>
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20 transition-all duration-300">
          <Icon className="h-5 w-5 text-primary" />
        </div>
      </div>
    </div>
  );
};

