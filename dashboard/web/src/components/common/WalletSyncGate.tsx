// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Loader2 } from 'lucide-react';

interface WalletSyncGateProps {
  feature: string;
  message?: string;
  progress?: number;
}

// Shown in place of a feature's first-time setup wizard while the wallet is
// still syncing. Activating a feature (which creates its dedicated account)
// before the wallet is synced and responsive can leave it misconfigured.
export const WalletSyncGate = ({ feature, message, progress }: WalletSyncGateProps) => {
  const showBar = typeof progress === 'number' && progress > 0 && progress < 100;
  return (
    <div className="max-w-xl space-y-3 rounded-xl border border-border/50 bg-gradient-card p-6">
      <div className="flex items-center gap-2">
        <Loader2 className="h-5 w-5 animate-spin text-primary" />
        <h3 className="text-lg font-semibold">{feature} is locked until your wallet is synced</h3>
      </div>
      <p className="text-sm text-muted-foreground">
        {message || 'Your wallet is still syncing.'} Setting up {feature} before the wallet is
        fully synced and responsive can leave it misconfigured, so it stays locked until sync
        completes.
      </p>
      {showBar && (
        <div className="space-y-1">
          <div className="h-2 overflow-hidden rounded-full bg-muted/30">
            <div
              className="h-full bg-gradient-primary transition-all"
              style={{ width: `${Math.min(100, Math.max(0, progress as number))}%` }}
            />
          </div>
          <p className="text-xs text-muted-foreground">{(progress as number).toFixed(1)}% synced</p>
        </div>
      )}
    </div>
  );
};
