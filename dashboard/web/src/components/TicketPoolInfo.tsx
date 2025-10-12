// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { TrendingUp, Target, Activity } from 'lucide-react';

interface TicketPoolInfoProps {
  poolSize: number;
  currentDifficulty: number;
  estimatedMin: number;
  estimatedMax: number;
  estimatedExpected: number;
  allMempoolTix: number;
}

export const TicketPoolInfo = ({ 
  poolSize,
  currentDifficulty,
  estimatedMin,
  estimatedMax,
  estimatedExpected,
  allMempoolTix
}: TicketPoolInfoProps) => {
  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Target className="h-6 w-6 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Ticket Pool & Difficulty</h3>
          <p className="text-sm text-muted-foreground">Network staking statistics</p>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* Pool Size */}
        <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">Pool Size</span>
            <Activity className="h-4 w-4 text-primary" />
          </div>
          <div className="text-2xl font-bold">{poolSize.toLocaleString()}</div>
          <div className="text-xs text-muted-foreground mt-1">Live tickets in pool</div>
        </div>

        {/* Mempool Tickets */}
        <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">Mempool Tickets</span>
            <Activity className="h-4 w-4 text-warning" />
          </div>
          <div className="text-2xl font-bold">{allMempoolTix}</div>
          <div className="text-xs text-muted-foreground mt-1">Pending tickets</div>
        </div>

        {/* Current Difficulty */}
        <div className="p-4 rounded-lg bg-gradient-to-br from-primary/5 to-primary/10 border border-primary/20">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">Current Price</span>
            <TrendingUp className="h-4 w-4 text-primary" />
          </div>
          <div className="text-2xl font-bold">{currentDifficulty.toFixed(2)} DCR</div>
          <div className="text-xs text-muted-foreground mt-1">Ticket price now</div>
        </div>

        {/* Expected Next Difficulty */}
        <div className="p-4 rounded-lg bg-gradient-to-br from-primary/5 to-primary/10 border border-primary/20">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">Expected Next</span>
            <TrendingUp className="h-4 w-4 text-primary" />
          </div>
          <div className="text-2xl font-bold">{estimatedExpected.toFixed(2)} DCR</div>
          <div className="text-xs text-muted-foreground mt-1">
            {estimatedExpected > currentDifficulty ? '↑' : estimatedExpected < currentDifficulty ? '↓' : '='} 
            {' '}{Math.abs(estimatedExpected - currentDifficulty).toFixed(2)} DCR
          </div>
        </div>
      </div>

      {/* Estimated Range */}
      <div className="mt-4 p-4 rounded-lg bg-info/5 border border-info/10">
        <div className="flex items-center justify-between mb-3">
          <span className="text-sm font-medium text-info">Expected Price Range</span>
        </div>
        <div className="grid grid-cols-3 gap-4 text-center">
          <div>
            <div className="text-xs text-muted-foreground mb-1">Min</div>
            <div className="font-bold text-lg">{estimatedMin.toFixed(2)}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground mb-1">Expected</div>
            <div className="font-bold text-lg text-primary">{estimatedExpected.toFixed(2)}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground mb-1">Max</div>
            <div className="font-bold text-lg">{estimatedMax.toFixed(2)}</div>
          </div>
        </div>
      </div>
    </div>
  );
};

