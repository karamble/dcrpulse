// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Coins, TrendingDown, Vote, Landmark, Pickaxe } from 'lucide-react';

interface BlockSubsidyInfoProps {
  blockSubsidyHeight: number;
  blockSubsidyTotal: number;
  blockSubsidyPos: number;
  blockSubsidyPow: number;
  blockSubsidyTreasury: number;
  blocksUntilSubsidyReduction: number;
  subsidyReductionInterval: number;
}

const VOTERS_PER_BLOCK = 5;
const BLOCK_TIME_MIN = 5;

const splitDCR = (amount: number) => {
  const [integerPart, decimalPart] = amount.toFixed(8).split('.');
  return {
    integer: parseInt(integerPart).toLocaleString('en-US'),
    mainDecimals: decimalPart.substring(0, 2),
    extraDecimals: decimalPart.substring(2),
  };
};

const DcrAmount = ({ amount }: { amount: number }) => {
  const parts = splitDCR(amount);
  return (
    <>
      {parts.integer}.{parts.mainDecimals}
      <span className="text-base opacity-60">{parts.extraDecimals}</span>
    </>
  );
};

export const BlockSubsidyInfo = ({
  blockSubsidyHeight,
  blockSubsidyTotal,
  blockSubsidyPos,
  blockSubsidyPow,
  blockSubsidyTreasury,
  blocksUntilSubsidyReduction,
  subsidyReductionInterval,
}: BlockSubsidyInfoProps) => {
  const perVote = blockSubsidyPos / VOTERS_PER_BLOCK;
  const pct = (part: number) =>
    blockSubsidyTotal > 0 ? (part / blockSubsidyTotal) * 100 : 0;

  const daysUntilReduction =
    (blocksUntilSubsidyReduction * BLOCK_TIME_MIN) / 60 / 24;
  // Decred subsidy reduces by factor 100/101 every interval.
  const nextSubsidy = (blockSubsidyTotal * 100) / 101;
  const reductionProgress = subsidyReductionInterval > 0
    ? ((subsidyReductionInterval - blocksUntilSubsidyReduction) / subsidyReductionInterval) * 100
    : 0;

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Coins className="h-6 w-6 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Block Subsidy</h3>
          <p className="text-sm text-muted-foreground">Current PoS reward per block</p>
        </div>
      </div>

      {/* Headline: total per-block subsidy */}
      <div className="mb-4 p-4 rounded-lg bg-gradient-to-br from-primary/5 to-primary/10 border border-primary/20">
        <div className="flex items-center justify-between mb-1">
          <span className="text-sm text-muted-foreground font-medium">Total Block Subsidy</span>
          <Coins className="h-4 w-4 text-primary" />
        </div>
        <div className="text-2xl font-bold"><DcrAmount amount={blockSubsidyTotal} /> DCR</div>
        <div className="text-xs text-muted-foreground mt-1">
          at height {blockSubsidyHeight.toLocaleString()} · {VOTERS_PER_BLOCK}-voter max
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* Per Vote */}
        <div className="p-4 rounded-lg bg-gradient-to-br from-success/5 to-success/10 border border-success/20">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">Per Vote</span>
            <Vote className="h-4 w-4 text-success" />
          </div>
          <div className="text-2xl font-bold text-success"><DcrAmount amount={perVote} /></div>
          <div className="text-xs text-muted-foreground mt-1">DCR / ticket vote</div>
        </div>

        {/* PoS Share */}
        <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">PoS Share</span>
            <Vote className="h-4 w-4 text-primary" />
          </div>
          <div className="text-2xl font-bold"><DcrAmount amount={blockSubsidyPos} /></div>
          <div className="text-xs text-muted-foreground mt-1">
            {pct(blockSubsidyPos).toFixed(1)}% · 5 voters
          </div>
        </div>

        {/* Treasury */}
        <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">Treasury</span>
            <Landmark className="h-4 w-4 text-info" />
          </div>
          <div className="text-2xl font-bold"><DcrAmount amount={blockSubsidyTreasury} /></div>
          <div className="text-xs text-muted-foreground mt-1">
            {pct(blockSubsidyTreasury).toFixed(1)}% of block
          </div>
        </div>

        {/* PoW */}
        <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">PoW</span>
            <Pickaxe className="h-4 w-4 text-warning" />
          </div>
          <div className="text-2xl font-bold"><DcrAmount amount={blockSubsidyPow} /></div>
          <div className="text-xs text-muted-foreground mt-1">
            {pct(blockSubsidyPow).toFixed(1)}% of block
          </div>
        </div>
      </div>

      {/* Next reduction */}
      <div className="mt-4 p-4 rounded-lg bg-info/5 border border-info/10">
        <div className="flex items-center justify-between mb-3">
          <span className="text-sm font-medium text-info flex items-center gap-2">
            <TrendingDown className="h-4 w-4" />
            Next Subsidy Reduction
          </span>
          <span className="text-xs text-muted-foreground">
            every {subsidyReductionInterval.toLocaleString()} blocks
          </span>
        </div>
        <div className="grid grid-cols-2 gap-4 text-center">
          <div>
            <div className="text-xs text-muted-foreground mb-1">In</div>
            <div className="font-bold text-lg">
              {blocksUntilSubsidyReduction.toLocaleString()} blocks
            </div>
            <div className="text-xs text-muted-foreground mt-1">
              ~{daysUntilReduction.toFixed(1)} days
            </div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground mb-1">Next Reward</div>
            <div className="font-bold text-lg text-primary">{nextSubsidy.toFixed(8)}</div>
            <div className="text-xs text-muted-foreground mt-1">DCR · ×100/101</div>
          </div>
        </div>
        <div className="mt-3 h-1.5 rounded-full bg-muted/30 overflow-hidden">
          <div
            className="h-full bg-info/60 transition-all"
            style={{ width: `${reductionProgress}%` }}
          />
        </div>
      </div>
    </div>
  );
};
