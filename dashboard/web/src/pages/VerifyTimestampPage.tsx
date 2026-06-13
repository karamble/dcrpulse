// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link } from 'react-router-dom';
import { ArrowLeft, FileClock } from 'lucide-react';
import { VerifyView } from '../components/timestamp/VerifyView';

// VerifyTimestampPage is the Explorer's standalone dcrtime proof checker. It
// reuses the Timestamp tab's verify flow but stands on its own as an on-chain
// tool, validated entirely through this dashboard's dcrd node.
export const VerifyTimestampPage = () => (
  <div className="space-y-6">
    <Link to="/explorer" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
      <ArrowLeft className="h-4 w-4" />
      Back to Explorer
    </Link>
    <div className="flex items-center gap-3">
      <FileClock className="h-6 w-6 text-primary shrink-0" />
      <div>
        <h1 className="text-xl font-bold">Verify Timestamp</h1>
        <p className="text-sm text-muted-foreground">
          Check a file or digest against dcrtime and confirm its anchor on the Decred chain.
        </p>
      </div>
    </div>
    <VerifyView />
  </div>
);
