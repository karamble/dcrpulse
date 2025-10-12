// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { SearchCheck } from 'lucide-react';

interface TSpendScanProgressProps {
  progress: number;
  currentHeight: number;
  totalHeight: number;
  tspendFound: number;
  message?: string;
}

export const TSpendScanProgress = ({
  progress,
  currentHeight,
  totalHeight,
  tspendFound,
  message,
}: TSpendScanProgressProps) => {
  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-2 mb-4">
        <SearchCheck className="h-5 w-5 text-primary animate-pulse" />
        <h3 className="font-semibold">Scanning Blockchain for TSpends</h3>
      </div>

      {/* Progress Bar */}
      <div className="relative w-full h-4 bg-muted rounded-full overflow-hidden mb-4">
        <div
          className="h-full bg-gradient-to-r from-primary to-primary/80 transition-all duration-300 ease-out"
          style={{ width: `${Math.min(progress, 100)}%` }}
        />
      </div>

      {/* Progress Details */}
      <div className="grid grid-cols-2 gap-4 text-sm">
        <div>
          <div className="text-muted-foreground">Progress</div>
          <div className="font-semibold">{progress.toFixed(1)}%</div>
        </div>
        
        <div>
          <div className="text-muted-foreground">TSpends Found</div>
          <div className="font-semibold text-success">{tspendFound}</div>
        </div>

        <div className="col-span-2">
          <div className="text-muted-foreground">Current Block</div>
          <div className="font-mono text-sm">
            {currentHeight.toLocaleString()} / {totalHeight.toLocaleString()}
          </div>
        </div>

        {message && (
          <div className="col-span-2">
            <div className="text-muted-foreground text-xs">{message}</div>
          </div>
        )}
      </div>

      {/* Info Text */}
      <div className="mt-4 pt-4 border-t border-border/50 text-sm text-muted-foreground">
        <p>Scanning from block 552,448 (Treasury activation) to current height.</p>
        <p className="mt-1">This may take a very long time. You can leave this page and come back later.</p>
      </div>
    </div>
  );
};

