// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { AlertTriangle, KeyRound } from 'lucide-react';
import { Link } from 'react-router-dom';

// GamingAppPasswordNotice explains why the gaming tab needs an app password,
// in place of the raw refusal the server sends without one.
export const GamingAppPasswordNotice = () => (
  <div className="p-6">
    <div className="p-4 rounded-lg bg-warning/10 border border-warning/40 flex items-start gap-3">
      <AlertTriangle className="h-4 w-4 text-warning mt-0.5 shrink-0" />
      <div className="text-sm">
        <span className="font-medium block text-warning">Set a dashboard app password first</span>
        <span className="text-muted-foreground">
          Gaming pays stakes and bonds from your wallet, signs payouts and refunds, and admits
          games by credential, so it needs a logged-in session. The gaming bridge also stays
          switched off until an app password is set. Set one under Settings &gt; Security, then
          come back.
        </span>
        <Link
          to="/wallet/settings/security"
          className="mt-3 inline-flex items-center gap-2 px-3 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm"
        >
          <KeyRound className="h-4 w-4" />
          Set app password
        </Link>
      </div>
    </div>
  </div>
);
