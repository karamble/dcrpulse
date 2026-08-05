// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { X } from 'lucide-react';
import { AlertEntry, getAlerts, markAlertRead, markAllAlertsRead } from '../../services/api';
import { refreshAlertsSummary } from '../../hooks/useAlerts';
import { useBisonrelayLive } from '../bisonrelay/BisonrelayLiveProvider';
import { AlertRow, alertCategoryRoute } from './AlertRow';

// Active conditions first, then unread events, then a short recent tail.
const pickPanelEntries = (all: AlertEntry[]): AlertEntry[] => {
  const active = all.filter((e) => e.active);
  const unread = all.filter((e) => e.kind === 'event' && !e.read);
  const rest = all.filter((e) => !e.active && !(e.kind === 'event' && !e.read));
  return [...active, ...unread, ...rest].slice(0, 30);
};

// AlertsPanel opens from the pill: an anchored popover on md+ and a
// full-screen sheet on mobile (drawer conventions from the Header). Opening
// does NOT mark anything read; reads are explicit per row or mark-all.
export const AlertsPanel = ({ onClose }: { onClose: () => void }) => {
  const [entries, setEntries] = useState<AlertEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const { addListener } = useBisonrelayLive();

  const load = () =>
    getAlerts()
      .then((all) => setEntries(pickPanelEntries(all)))
      .catch(() => {})
      .finally(() => setLoading(false));

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    return addListener((evt) => {
      if (evt.type === 'alerts') void load();
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [addListener]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const readOne = async (id: string) => {
    setEntries((prev) => prev.map((e) => (e.id === id ? { ...e, read: true } : e)));
    try {
      await markAlertRead(id);
    } catch {
      void load();
    }
    refreshAlertsSummary();
  };

  const readAll = async () => {
    setEntries((prev) => prev.map((e) => ({ ...e, read: true })));
    try {
      await markAllAlertsRead();
    } catch {
      void load();
    }
    refreshAlertsSummary();
  };

  const openEntry = (entry: AlertEntry) => {
    onClose();
    navigate(alertCategoryRoute[entry.category] || '/alerts');
  };

  const hasUnread = entries.some((e) => !e.read);

  return (
    <div className="fixed inset-0 z-40 md:inset-auto md:bottom-20 md:right-4 md:w-96">
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm md:hidden" onClick={onClose} />
      <div className="absolute inset-x-0 bottom-0 top-14 md:static flex flex-col overflow-hidden rounded-t-xl md:rounded-xl bg-card border border-border/50 shadow-xl md:max-h-[70vh]">
        <div className="shrink-0 flex items-center justify-between px-3 py-2 border-b border-border/40">
          <span className="text-sm font-semibold text-foreground">Alerts</span>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close alerts"
            className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="overflow-y-auto">
          {loading ? (
            <p className="text-xs text-muted-foreground p-4 text-center">Loading…</p>
          ) : entries.length === 0 ? (
            <p className="text-xs text-muted-foreground p-4 text-center">No alerts.</p>
          ) : (
            entries.map((e) => <AlertRow key={e.id} entry={e} onRead={readOne} onOpen={openEntry} />)
          )}
        </div>
        <div className="shrink-0 flex items-center gap-2 border-t border-border/40 p-2">
          {hasUnread && (
            <button
              type="button"
              onClick={readAll}
              className="flex-1 px-2 py-1.5 rounded-md bg-muted/20 text-muted-foreground text-xs hover:bg-muted/30 hover:text-foreground transition-colors"
            >
              Mark all read
            </button>
          )}
          <button
            type="button"
            onClick={() => {
              onClose();
              navigate('/alerts');
            }}
            className="flex-1 px-2 py-1.5 rounded-md bg-primary/15 text-primary text-xs hover:bg-primary/25 transition-colors"
          >
            View all
          </button>
        </div>
      </div>
    </div>
  );
};
