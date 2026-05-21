// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import {
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Coins,
  Pause,
  Play,
  Save,
  ScrollText,
  Ticket,
  Wallet,
} from 'lucide-react';
import {
  AccountInfo,
  AutobuyerEvent,
  AutobuyerSettings,
  AutobuyerStatus,
  VSPInfo,
  getAccounts,
  getAutobuyerSettings,
  getAutobuyerStatus,
  getVSPInfo,
  saveAutobuyerSettings,
  startAutobuyer,
  stopAutobuyer,
  subscribeAutobuyerEvents,
} from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';
import { VSPSelect } from './VSPSelect';

const MAX_EVENTS = 200;

const levelClass = (level: AutobuyerEvent['level']) => {
  switch (level) {
    case 'error':
      return 'text-destructive';
    case 'warn':
      return 'text-warning';
    default:
      return 'text-muted-foreground';
  }
};

export const AutobuyerTab = () => {
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [status, setStatus] = useState<AutobuyerStatus | null>(null);
  const [vsp, setVsp] = useState<VSPInfo | null>(null);
  const [account, setAccount] = useState<number | null>(null);
  const [balance, setBalance] = useState<string>('0');
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState<{ kind: 'info' | 'error'; text: string } | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [events, setEvents] = useState<AutobuyerEvent[]>([]);
  const [expanded, setExpanded] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);

  const refreshStatus = async () => {
    try {
      const s = await getAutobuyerStatus();
      setStatus(s);
    } catch {
      /* ignore */
    }
  };

  // Initial load: accounts, persisted settings, status.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [accs, persisted, s] = await Promise.all([
          getAccounts(),
          getAutobuyerSettings(),
          getAutobuyerStatus(),
        ]);
        if (cancelled) return;
        const usable = accs.filter((a) => a.accountName !== 'imported');
        setAccounts(usable);
        setStatus(s);
        if (persisted) {
          setAccount(persisted.account);
          setBalance(persisted.balanceToMaintain.toString());
          if (persisted.vspHost && persisted.vspPubkey) {
            // Try to re-hydrate the VSP from its registry probe.
            try {
              const info = await getVSPInfo(persisted.vspHost);
              if (!cancelled) {
                setVsp({ ...info, host: persisted.vspHost, pubkey: persisted.vspPubkey });
              }
            } catch {
              if (!cancelled) {
                setVsp({
                  host: persisted.vspHost,
                  pubkey: persisted.vspPubkey,
                  network: 'mainnet',
                  feePercentage: 0,
                });
              }
            }
          }
        } else {
          // No persisted settings: preselect mixed account if it exists.
          const mixed = usable.find((a) => a.accountName === 'mixed');
          if (mixed) setAccount(mixed.accountNumber);
        }
      } catch (err: any) {
        if (!cancelled) {
          setFeedback({ kind: 'error', text: err?.message || 'Failed to load autobuyer state' });
        }
      }
    })();
    const id = window.setInterval(refreshStatus, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  // WS event subscription.
  useEffect(() => {
    const cleanup = subscribeAutobuyerEvents(
      (ev) => {
        setEvents((prev) => {
          const next = [...prev, ev];
          if (next.length > MAX_EVENTS) next.splice(0, next.length - MAX_EVENTS);
          return next;
        });
      },
      (err) => console.error('Autobuyer events WebSocket error:', err),
    );
    return cleanup;
  }, []);

  useEffect(() => {
    if (expanded && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [events, expanded]);

  const running = status?.running ?? false;
  const balanceNum = parseFloat(balance);
  const balanceValid = !isNaN(balanceNum) && balanceNum >= 0;
  const settingsValid = vsp !== null && account !== null && balanceValid;

  const buildSettings = (): AutobuyerSettings | null => {
    if (!vsp || account === null || !balanceValid) return null;
    return {
      account,
      vspHost: vsp.host,
      vspPubkey: vsp.pubkey,
      balanceToMaintain: balanceNum,
    };
  };

  const handleSave = async () => {
    const s = buildSettings();
    if (!s) return;
    setBusy(true);
    setFeedback(null);
    try {
      await saveAutobuyerSettings(s);
      setFeedback({ kind: 'info', text: 'Settings saved.' });
      await refreshStatus();
    } catch (err: any) {
      const body = err?.response?.data;
      setFeedback({ kind: 'error', text: typeof body === 'string' ? body : err?.message || 'Save failed' });
    } finally {
      setBusy(false);
    }
  };

  const handleStart = async (passphrase: string) => {
    const s = buildSettings();
    if (!s) return;
    try {
      await startAutobuyer({ ...s, passphrase });
      setModalOpen(false);
      setFeedback({ kind: 'info', text: 'Autobuyer started.' });
      await refreshStatus();
    } catch (err: any) {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Start failed';
      throw new Error(msg);
    }
  };

  const handleStop = async () => {
    setBusy(true);
    setFeedback(null);
    try {
      await stopAutobuyer();
      setFeedback({ kind: 'info', text: 'Autobuyer stop requested.' });
      await refreshStatus();
    } catch (err: any) {
      setFeedback({ kind: 'error', text: err?.message || 'Stop failed' });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Status badge */}
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div
            className={`p-3 rounded-xl ${running ? 'bg-success/15 border border-success/30' : 'bg-muted/10 border border-border/50'}`}
          >
            <Ticket className={`h-6 w-6 ${running ? 'text-success animate-pulse' : 'text-muted-foreground'}`} />
          </div>
          <div>
            <h3 className="text-lg font-semibold">{running ? 'Autobuyer running' : 'Autobuyer stopped'}</h3>
            <p className="text-sm text-muted-foreground">
              {running
                ? 'Buying tickets while spendable balance is above the threshold.'
                : 'Configure settings below, then click Start.'}
            </p>
          </div>
        </div>
        {status?.lastError && !running && (
          <div className="text-xs text-destructive max-w-sm truncate" title={status.lastError}>
            Last error: {status.lastError}
          </div>
        )}
      </div>

      {/* Settings */}
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <Coins className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Settings</h3>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <div className="space-y-1">
            <label className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <Wallet className="h-4 w-4" />
              Source account
            </label>
            <select
              value={account ?? ''}
              onChange={(e) => setAccount(e.target.value === '' ? null : Number(e.target.value))}
              disabled={running}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border/50 text-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <option value="" disabled>
                Select account
              </option>
              {accounts.map((a) => (
                <option key={a.accountNumber} value={a.accountNumber}>
                  {a.accountName} ({a.spendableBalance.toFixed(2)} DCR)
                </option>
              ))}
            </select>
          </div>

          <div className={running ? 'opacity-50 pointer-events-none' : ''}>
            <VSPSelect network="mainnet" value={vsp} onChange={setVsp} />
          </div>

          <div className="space-y-1">
            <label className="text-sm font-medium text-muted-foreground">
              Balance to maintain (DCR)
            </label>
            <input
              type="number"
              min={0}
              step={0.01}
              value={balance}
              onChange={(e) => setBalance(e.target.value)}
              disabled={running}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border/50 text-sm font-mono disabled:opacity-50 disabled:cursor-not-allowed"
            />
            <p className="text-xs text-muted-foreground">
              Autobuyer keeps buying tickets while the source account's spendable balance is above
              this threshold.
            </p>
          </div>
        </div>

        {feedback && (
          <div
            className={`flex items-center gap-2 text-sm ${feedback.kind === 'error' ? 'text-destructive' : 'text-success'}`}
          >
            {feedback.kind === 'error' && <AlertCircle className="h-4 w-4" />}
            {feedback.text}
          </div>
        )}

        <div className="flex flex-wrap gap-3">
          <button
            onClick={handleSave}
            disabled={!settingsValid || busy || running}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Save className="h-4 w-4" />
            Save settings
          </button>
          {running ? (
            <button
              onClick={handleStop}
              disabled={busy}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-destructive/20 hover:bg-destructive/30 text-destructive text-sm font-semibold transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Pause className="h-4 w-4" />
              Stop
            </button>
          ) : (
            <button
              onClick={() => setModalOpen(true)}
              disabled={!settingsValid || busy}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Play className="h-4 w-4" />
              Start
            </button>
          )}
        </div>
      </div>

      {/* Event log */}
      <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden">
        <div
          onClick={() => setExpanded(!expanded)}
          className="w-full flex items-center justify-between p-4 hover:bg-muted/10 transition-colors cursor-pointer"
        >
          <div className="flex items-center gap-2">
            {expanded ? (
              <ChevronDown className="h-4 w-4 text-muted-foreground" />
            ) : (
              <ChevronRight className="h-4 w-4 text-muted-foreground" />
            )}
            <ScrollText className="h-4 w-4 text-muted-foreground" />
            <h3 className="text-sm font-semibold">Autobuyer events</h3>
          </div>
          <span className="text-xs text-muted-foreground">
            {events.length} event{events.length === 1 ? '' : 's'}
          </span>
        </div>
        {expanded && (
          <div
            ref={scrollRef}
            className="max-h-64 overflow-y-auto px-4 pb-4 border-t border-border/30 space-y-1 font-mono text-xs"
          >
            {events.length === 0 ? (
              <p className="py-4 text-center text-muted-foreground">No events yet.</p>
            ) : (
              events.map((ev, i) => (
                <div key={i} className="flex gap-2">
                  <span className="text-muted-foreground shrink-0">
                    {new Date(ev.timestamp).toLocaleTimeString()}
                  </span>
                  <span className={`shrink-0 uppercase ${levelClass(ev.level)}`}>{ev.level}</span>
                  <span className="text-foreground break-all">{ev.message}</span>
                </div>
              ))
            )}
          </div>
        )}
      </div>

      <PassphraseModal
        isOpen={modalOpen}
        title="Start Autobuyer"
        description="Enter your private passphrase to unlock the source account and start the ticket autobuyer."
        submitLabel="Start"
        busyLabel="Starting…"
        onSubmit={handleStart}
        onClose={() => setModalOpen(false)}
      />
    </div>
  );
};
