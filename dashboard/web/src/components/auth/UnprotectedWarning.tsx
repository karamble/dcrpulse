// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { AlertTriangle } from 'lucide-react';
import { Link } from 'react-router-dom';
import { useAuth } from './AuthGate';

export const SECURITY_SETTINGS_PATH = '/wallet/settings/security';

// UnprotectedWarning is the text shown wherever the user is about to run
// without an app password: skipping the first-run prompt or disabling it later.
export function UnprotectedWarning({
  acknowledged,
  onAcknowledge,
}: {
  acknowledged: boolean;
  onAcknowledge: (v: boolean) => void;
}) {
  return (
    <div className="space-y-3 rounded-lg border border-warning/40 bg-warning/10 p-3">
      <p className="text-sm font-semibold text-warning flex items-center gap-2">
        <AlertTriangle className="h-4 w-4 shrink-0" />
        You are about to leave your wallet unprotected
      </p>
      <p className="text-sm text-foreground">
        Without a password, dcrpulse cannot tell you apart from anyone else.
        Lightning payments, tips and DEX trades need no further confirmation,
        and <strong>payments cannot be reversed</strong>. Umbrel's login does
        not cover other apps on the same device.
      </p>
      <label className="flex items-start gap-2 text-sm text-foreground cursor-pointer">
        <input
          type="checkbox"
          checked={acknowledged}
          onChange={(e) => onAcknowledge(e.target.checked)}
          className="mt-0.5 accent-warning"
        />
        I understand anyone who reaches this dashboard can spend my funds
      </label>
    </div>
  );
}

// UnprotectedBanner stays on every page while no app password is set, so the
// one-time first-run choice is not the last reminder.
export function UnprotectedBanner() {
  const { status, known } = useAuth();
  if (!known || !status || status.enabled || status.locked) return null;
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-x-2 gap-y-1 px-3 py-2 rounded-lg border border-warning/40 bg-warning/10 text-sm"
    >
      <AlertTriangle className="h-4 w-4 shrink-0 text-warning" />
      <span className="text-foreground">
        No app password: this dashboard is unprotected.
      </span>
      <Link to={SECURITY_SETTINGS_PATH} className="font-semibold text-warning hover:underline">
        Set one
      </Link>
    </div>
  );
}
