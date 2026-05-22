// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { CheckCircle2 } from 'lucide-react';
import { BisonrelaySetupWizard } from './BisonrelaySetupWizard';
import { BisonrelayStatus, getBisonrelayStatus } from '../../services/bisonrelayApi';

export const BisonrelayPage = () => {
  const [ready, setReady] = useState<BisonrelayStatus | null>(null);

  useEffect(() => {
    if (!ready) return;
    let cancelled = false;
    const tick = async () => {
      try {
        const s = await getBisonrelayStatus();
        if (!cancelled && s.stage === 'ready') setReady(s);
      } catch {
        /* keep last known */
      }
    };
    const id = setInterval(tick, 15000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [ready]);

  if (ready) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-3 max-w-2xl">
        <div className="flex items-center gap-3">
          <CheckCircle2 className="h-5 w-5 text-success" />
          <h2 className="text-lg font-semibold">Bison Relay is ready</h2>
        </div>
        <p className="text-sm text-muted-foreground">
          Connected as <code className="font-mono">{ready.nick ?? 'unknown'}</code>.
          Chat, group chats, posts and tipping land in subsequent dcrpulse
          releases.
        </p>
        {ready.serverNode && (
          <p className="text-xs text-muted-foreground">
            BR server LN node:{' '}
            <code className="font-mono break-all">{ready.serverNode}</code>
          </p>
        )}
      </div>
    );
  }

  return (
    <BisonrelaySetupWizard
      onReady={async () => {
        try {
          const s = await getBisonrelayStatus();
          setReady(s);
        } catch {
          /* ignored - wizard keeps rendering */
        }
      }}
    />
  );
};
