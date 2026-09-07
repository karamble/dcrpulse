// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ShieldAlert } from 'lucide-react';

// LockedScreen replaces the app while the backend cannot read its app-password
// config. There is nothing to log in to: every API route answers 503 until the
// file is repaired and the dashboard restarted.
export function LockedScreen({ reason }: { reason?: string }) {
  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <div className="w-full max-w-md p-6 rounded-xl bg-gradient-card border border-border/50 space-y-4">
        <div className="flex items-center gap-3">
          <div className="p-3 rounded-xl bg-destructive/10 border border-destructive/20">
            <ShieldAlert className="h-6 w-6 text-destructive" />
          </div>
          <div>
            <h1 className="text-lg font-semibold">Configuration unreadable</h1>
            <p className="text-sm text-muted-foreground">
              The dashboard cannot load its app-password settings.
            </p>
          </div>
        </div>
        {reason && (
          <pre className="text-xs font-mono whitespace-pre-wrap break-words rounded-lg bg-muted/40 border border-border/50 p-3 text-muted-foreground">
            {reason}
          </pre>
        )}
        <p className="text-sm text-muted-foreground">
          The dashboard refuses every request rather than run without the password it
          may have been given. Repair <code className="font-mono">/dashboard-data/config.json</code>{' '}
          inside the dashboard container, then restart the dashboard.
        </p>
      </div>
    </div>
  );
}
