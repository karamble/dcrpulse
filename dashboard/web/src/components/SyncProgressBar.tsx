// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Loader2 } from 'lucide-react';

interface SyncProgressBarProps {
  progress: number;
  scanHeight: number;
  chainHeight: number;
  message: string;
}

export const SyncProgressBar = ({ 
  progress, 
  scanHeight, 
  chainHeight, 
  message 
}: SyncProgressBarProps) => {
  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-4">
          <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
            <Loader2 className="h-6 w-6 text-primary animate-spin" />
          </div>
          <div>
            <h3 className="text-lg font-semibold">Wallet Syncing in Progress</h3>
            <p className="text-sm text-muted-foreground">
              {message}
            </p>
          </div>
        </div>
        <div className="px-6 py-3 rounded-xl bg-warning/10 border-2 border-warning/20">
          <span className="text-warning font-bold text-lg tracking-wide">
            Syncing
          </span>
        </div>
      </div>
      
      <div className="space-y-2">
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground font-medium">
            Block {scanHeight.toLocaleString()} of {chainHeight.toLocaleString()}
          </span>
          <span className="font-bold text-lg text-primary">{progress.toFixed(1)}%</span>
        </div>
        <div className="relative h-3 bg-muted rounded-full overflow-hidden border border-border/50">
          <div 
            className="h-full bg-gradient-primary transition-all duration-500 ease-out relative overflow-hidden"
            style={{ width: `${progress}%` }}
          >
            <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent animate-pulse" />
          </div>
        </div>
      </div>

      <div className="mt-4 p-3 rounded-lg bg-primary/5 border border-primary/10">
        <p className="text-xs text-muted-foreground">
          This may take 30+ minutes depending on blockchain size. The wallet will be fully functional once the sync completes.
        </p>
      </div>
    </div>
  );
};

