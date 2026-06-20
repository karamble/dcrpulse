// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Eye } from 'lucide-react';

interface WatchOnlyGateProps {
  feature: string;
}

// Shown in place of a spending feature when the active wallet is watch-only (no
// private keys). Watch-only wallets monitor balances and transactions but cannot
// sign, so spending features are unavailable.
export const WatchOnlyGate = ({ feature }: WatchOnlyGateProps) => {
  return (
    <div className="max-w-xl space-y-3 rounded-xl border border-border/50 bg-gradient-card p-6">
      <div className="flex items-center gap-2">
        <Eye className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold">{feature} is not available for watch-only wallets</h3>
      </div>
      <p className="text-sm text-muted-foreground">
        This wallet was imported from an extended public key and holds no private keys, so it can
        monitor balances and transactions but cannot spend. Switch to a wallet with spending keys to
        use {feature}.
      </p>
    </div>
  );
};
