// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Target, TrendingUp, TrendingDown, Minus, Clock } from 'lucide-react';
import { StakingInfo } from '../services/api';

interface TicketPoolCardProps {
  data?: StakingInfo;
  currentBlockHeight?: number;
}

export const TicketPoolCard = ({ data, currentBlockHeight }: TicketPoolCardProps) => {
  const TARGET_POOL_SIZE = 40960;
  const STAKE_DIFF_WINDOW = 144; // Ticket price adjusts every 144 blocks
  const AVERAGE_BLOCK_TIME = 5; // minutes
  
  const poolSize = data?.poolSize || 0;
  const ticketPrice = data?.ticketPrice || 0;
  const nextTicketPrice = data?.nextTicketPrice || 0;
  
  // Calculate status
  const getPoolStatus = () => {
    if (poolSize === 0) return { label: 'N/A', icon: Minus, color: 'text-muted-foreground' };
    
    const diff = poolSize - TARGET_POOL_SIZE;
    const percentDiff = (Math.abs(diff) / TARGET_POOL_SIZE) * 100;
    
    // Consider "At Target" if within 1% of target
    if (percentDiff < 1) {
      return { 
        label: 'At Target', 
        icon: Target, 
        color: 'text-success'
      };
    } else if (diff > 0) {
      return { 
        label: 'Above Target', 
        icon: TrendingUp, 
        color: 'text-warning'
      };
    } else {
      return { 
        label: 'Below Target', 
        icon: TrendingDown, 
        color: 'text-info'
      };
    }
  };
  
  const status = getPoolStatus();
  const StatusIcon = status.icon;
  
  const formatTicketPrice = () => {
    if (ticketPrice === 0) return 'N/A';
    return `${ticketPrice.toFixed(2)} DCR`;
  };

  // Calculate time until next ticket price adjustment
  const getNextAdjustmentTime = () => {
    if (!currentBlockHeight || currentBlockHeight === 0 || isNaN(currentBlockHeight)) {
      return { text: 'N/A', blocks: 0 };
    }

    // Calculate blocks remaining until next adjustment
    const blocksRemaining = STAKE_DIFF_WINDOW - (currentBlockHeight % STAKE_DIFF_WINDOW);
    
    // Calculate time in minutes
    const minutesRemaining = blocksRemaining * AVERAGE_BLOCK_TIME;
    
    // Convert to days, hours, minutes
    const days = Math.floor(minutesRemaining / 1440);
    const hours = Math.floor((minutesRemaining % 1440) / 60);
    const minutes = Math.floor(minutesRemaining % 60);
    
    // Format the string
    let timeText = '';
    if (days > 0) {
      timeText = `${days}d ${hours}h ${minutes}m`;
    } else if (hours > 0) {
      timeText = `${hours}h ${minutes}m`;
    } else {
      timeText = `${minutes}m`;
    }
    
    return { text: timeText, blocks: blocksRemaining };
  };

  const nextAdjustment = getNextAdjustmentTime();

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 hover:border-primary/20 transition-all duration-300 group animate-fade-in">
      <div className="space-y-4">
        {/* Header Row: Pool Size & Icon */}
        <div className="flex items-start justify-between">
          <div className="flex-1">
            <p className="text-sm text-muted-foreground mb-1">Ticket Pool</p>
            <div className="flex items-baseline gap-3 mb-1">
              <h3 className="text-3xl font-bold group-hover:text-primary transition-colors">
                {poolSize === 0 ? 'N/A' : poolSize.toLocaleString()}
              </h3>
              {/* Status Badge */}
              <div className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-muted/30 ${status.color}`}>
                <StatusIcon className="h-3.5 w-3.5" />
                <span className="font-medium text-xs">{status.label}</span>
              </div>
            </div>
            <div className="text-xs text-muted-foreground">Live tickets (target: {TARGET_POOL_SIZE.toLocaleString()})</div>
          </div>
          
          {/* Icon */}
          <div className="p-3 rounded-xl bg-primary/10 border border-primary/20 transition-all duration-300">
            <Target className="h-5 w-5 text-primary" />
          </div>
        </div>

        {/* Divider */}
        <div className="border-t border-border/50" />

        {/* Bottom Row: Current Price | Expected Next | Next Update - All in one line */}
        <div className="flex items-center justify-between gap-6">
          {/* Current Price */}
          <div>
            <div className="text-xs text-muted-foreground mb-1">Current Price</div>
            <div className="text-lg font-bold text-primary">
              {formatTicketPrice()}
            </div>
          </div>

          {/* Expected Next */}
          {nextTicketPrice > 0 && (
            <div>
              <div className="text-xs text-muted-foreground mb-1">Expected Next</div>
              <div className="text-sm font-semibold text-muted-foreground">
                {nextTicketPrice.toFixed(2)} DCR
                {ticketPrice > 0 && Math.abs(nextTicketPrice - ticketPrice) > 0.01 && (
                  <span className={`ml-1 ${nextTicketPrice > ticketPrice ? 'text-warning' : 'text-success'}`}>
                    {nextTicketPrice > ticketPrice ? '↑' : '↓'}
                  </span>
                )}
              </div>
            </div>
          )}

          {/* Next Update */}
          <div className="flex items-center gap-2">
            <Clock className="h-4 w-4 text-muted-foreground flex-shrink-0" />
            <div>
              <div className="text-xs text-muted-foreground mb-1">Next Update</div>
              <div className="font-medium text-sm whitespace-nowrap">
                {nextAdjustment.text !== 'N/A' ? (
                  <>{nextAdjustment.text} ({nextAdjustment.blocks} blocks)</>
                ) : (
                  'Loading...'
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

