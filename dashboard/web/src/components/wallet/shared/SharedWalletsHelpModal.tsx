// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { createPortal } from 'react-dom';
import { Info, Users, X } from 'lucide-react';

// SharedWalletsHelpModal explains what shared wallets are and the
// account limitation that applies when restoring a backup card on a
// reseeded wallet.
export const SharedWalletsHelpModal = ({ onClose }: { onClose: () => void }) => {
  // Portal to body: a position:fixed overlay is trapped inside any ancestor
  // with backdrop-filter/transform, so it would fill the card instead of the
  // viewport.
  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <div className="w-full max-w-lg mx-4 rounded-xl bg-card border border-border/50 shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <Users className="h-5 w-5 text-primary" />
            About shared wallets
          </h3>
          <button
            onClick={onClose}
            className="text-muted-foreground hover:text-foreground"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-6 space-y-4 text-sm">
          <div className="space-y-2">
            <h4 className="font-medium">What a shared wallet is</h4>
            <p className="text-muted-foreground">
              A shared wallet is a multisignature wallet run together with other
              people. Every cosigner contributes an account public key, the
              wallet's addresses are derived from all of those keys combined,
              and a payment only broadcasts once enough cosigners have approved
              and signed it. No single member can spend the funds alone.
            </p>
            <p className="text-muted-foreground">
              Recovery needs two things: this wallet's seed and the shared
              wallet's backup card. The card carries the cosigner keys, which
              the seed alone cannot recover, so download it from the detail
              page and keep it safe.
            </p>
          </div>

          <div className="space-y-2">
            <h4 className="font-medium">Restoring after a reseed</h4>
            <p className="text-muted-foreground">
              Each shared wallet reserves a dedicated account of this wallet.
              Those accounts never show payment history of their own, and the
              wallet software refuses to create a new account while the last 10
              accounts are unused. A restore recreates accounts one by one, so
              a backup card whose account lies more than 10 past this wallet's
              last account cannot be restored yet.
            </p>
            <p className="text-muted-foreground">
              If a restore is refused for this reason, recover the seed's used
              accounts first by restoring the wallet with full account
              discovery, then import the backup card again.
            </p>
          </div>

          <div className="rounded-lg bg-primary/10 border border-primary/30 p-3 text-xs text-foreground/80 flex items-start gap-2">
            <Info className="h-4 w-4 text-primary shrink-0 mt-0.5" />
            <span>
              <span className="font-medium">Good practice:</span> run shared
              wallets in a wallet dedicated to them, and keep at most 10 per
              wallet. Low account numbers restore effortlessly after a reseed;
              import backup cards only after the wallet has finished account
              discovery.
            </span>
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
};
