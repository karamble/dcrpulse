// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { AlertTriangle } from 'lucide-react';
import { useDexConn } from './DexLiveProvider';

// DexServerBanner is a compact notice shown while the DEX server is
// unreachable (connection state seeded from the exchanges snapshot and
// updated by live `conn` notes). Renders nothing while connected or before
// any state is known.
export const DexServerBanner = ({ host }: { host: string }) => {
  const conn = useDexConn(host);
  if (!conn || conn.status === 1) return null;
  return (
    <div className="mx-4 flex items-center gap-2 px-3 py-1.5 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning">
      <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
      <span className="truncate">
        DEX server {host} can not be connected. Retrying in the background.
      </span>
    </div>
  );
};
