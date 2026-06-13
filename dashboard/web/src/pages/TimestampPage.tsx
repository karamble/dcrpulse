// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { FileClock } from 'lucide-react';
import { getTimestampStatus, type TimestampStatusInfo } from '../services/timestampApi';
import { StampView } from '../components/timestamp/StampView';
import { LibraryView } from '../components/timestamp/LibraryView';
import { VerifyView } from '../components/timestamp/VerifyView';
import { RecordDetail } from '../components/timestamp/RecordDetail';
import { NextAnchor } from '../components/timestamp/NextAnchor';

type Tab = 'stamp' | 'library' | 'verify';

export const TimestampPage = () => {
  const [tab, setTab] = useState<Tab>('stamp');
  const [status, setStatus] = useState<TimestampStatusInfo | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const loadStatus = () => {
    getTimestampStatus(true)
      .then(setStatus)
      .catch(() => {
        /* status is best-effort; the views still work */
      });
  };
  useEffect(loadStatus, []);

  const tabBtn = (id: Tab, label: string) => (
    <button
      onClick={() => setTab(id)}
      className={`px-4 py-2 rounded-lg text-sm font-medium whitespace-nowrap transition-colors ${
        tab === id ? 'bg-primary/20 text-primary' : 'text-foreground hover:bg-muted/20'
      }`}
    >
      {label}
    </button>
  );

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3">
        <div className="flex items-center gap-3">
          <FileClock className="h-6 w-6 text-primary shrink-0" />
          <div>
            <h1 className="text-xl font-bold">Timestamp</h1>
            <p className="text-sm text-muted-foreground">Proof of existence via dcrtime</p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
          {status?.network && (
            <span>
              Network: <span className="text-foreground capitalize">{status.network}</span>
            </span>
          )}
          {status?.reachable !== undefined && (
            <span className="inline-flex items-center gap-1.5">
              <span className={`h-2 w-2 rounded-full ${status.reachable ? 'bg-success' : 'bg-destructive'}`} />
              dcrtime {status.reachable ? 'reachable' : 'unreachable'}
            </span>
          )}
          {typeof status?.pending === 'number' && status.pending > 0 && <span>{status.pending} awaiting anchor</span>}
          <NextAnchor />
        </div>
      </div>

      {status && !status.enabled && (
        <div className="p-3 rounded-lg bg-warning/10 border border-warning/30 text-sm text-warning">
          dcrtime requests are disabled. Enable them under Settings → Privacy to submit and verify timestamps.
        </div>
      )}

      <div className="flex gap-1 overflow-x-auto -mx-3 px-3 pb-1">
        {tabBtn('stamp', 'Stamp')}
        {tabBtn('library', 'Library')}
        {tabBtn('verify', 'Verify')}
      </div>

      {tab === 'stamp' && (
        <StampView
          onStamped={() => {
            setReloadKey((k) => k + 1);
            loadStatus();
          }}
        />
      )}
      {tab === 'library' && <LibraryView onOpen={setSelected} reloadKey={reloadKey} />}
      {tab === 'verify' && <VerifyView />}

      {selected && (
        <RecordDetail
          digest={selected}
          onClose={() => setSelected(null)}
          onChanged={() => {
            setReloadKey((k) => k + 1);
            loadStatus();
          }}
        />
      )}
    </div>
  );
};
