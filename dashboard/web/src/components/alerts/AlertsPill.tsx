// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { Bell } from 'lucide-react';
import { useAlertsSummary, refreshAlertsSummary } from '../../hooks/useAlerts';
import { useBisonrelayLive } from '../bisonrelay/BisonrelayLiveProvider';
import { AlertsPanel } from './AlertsPanel';

// AlertsPill is the alerts center's only permanent surface: nothing renders
// while there is nothing to say, and the pill appears bottom-right once
// unread events + active conditions > 0, tinted by the highest severity. The
// deliberately packed header stays untouched.
export const AlertsPill = () => {
  const { unread, active, severity } = useAlertsSummary();
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const { addListener } = useBisonrelayLive();

  // Live nudge: the backend publishes an empty "alerts" event on the BR
  // socket after every change; refetch instead of trusting a payload. The
  // 15s poll in useAlertsSummary is the backstop (watch-only skips the
  // socket entirely).
  useEffect(() => {
    return addListener((evt) => {
      if (evt.type === 'alerts') refreshAlertsSummary();
    });
  }, [addListener]);

  const count = unread + active;

  useEffect(() => {
    if (count === 0) setOpen(false);
  }, [count]);

  useEffect(() => {
    if (!open) return undefined;
    const onClick = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, [open]);

  if (count === 0) return null;

  const tone =
    severity === 'critical'
      ? 'bg-destructive text-destructive-foreground'
      : severity === 'warning'
        ? 'bg-warning text-warning-foreground'
        : 'bg-primary text-primary-foreground';

  return (
    <div ref={wrapRef}>
      {open && <AlertsPanel onClose={() => setOpen(false)} />}
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-label={`Alerts (${count})`}
        className={`fixed bottom-4 right-4 z-40 flex items-center gap-1.5 px-3.5 py-2.5 rounded-full shadow-xl ring-2 ring-background/60 transition-colors ${tone}`}
      >
        <Bell className="h-4 w-4" />
        <span className="text-xs font-semibold">{count > 99 ? '99+' : count}</span>
      </button>
    </div>
  );
};
