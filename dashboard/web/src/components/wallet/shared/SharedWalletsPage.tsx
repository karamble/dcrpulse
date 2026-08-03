// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, Loader2, Mail, Plus, RefreshCw, Upload, Users, X } from 'lucide-react';
import { useBisonrelayLive } from '../../bisonrelay/BisonrelayLiveProvider';
import {
  MsigWallet,
  importMsigFrame,
  listMsigWallets,
  msigStatusLabel,
  refreshMsig,
  restoreMsigWallet,
} from '../../../services/msigApi';
import { PassphraseModal } from '../PassphraseModal';
import { IncomingInviteBanner } from './IncomingInviteBanner';
import { SharedWalletCreateWizard } from './SharedWalletCreateWizard';

const statusTone = (status: string): string => {
  switch (status) {
    case 'active':
      return 'bg-success/10 text-success border-success/20';
    case 'failed':
    case 'declined':
      return 'bg-destructive/10 text-destructive border-destructive/20';
    case 'pending_import':
      return 'bg-warning/10 text-warning border-warning/20';
    default:
      return 'bg-muted/30 text-muted-foreground border-border/50';
  }
};

// SharedWalletsPage lists this wallet's multisig wallets and hosts the
// create wizard plus the incoming-invitation banners.
export const SharedWalletsPage = () => {
  const [wallets, setWallets] = useState<MsigWallet[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [wizard, setWizard] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [restoreCard, setRestoreCard] = useState<unknown>(null);
  const [importing, setImporting] = useState(false);
  const [importText, setImportText] = useState('');
  const [fromLabel, setFromLabel] = useState('');
  const [importBusy, setImportBusy] = useState(false);
  const [importErr, setImportErr] = useState<string | null>(null);
  const importFileRef = useRef<HTMLInputElement | null>(null);
  const { addListener } = useBisonrelayLive();

  const load = useCallback(async () => {
    try {
      const { wallets: list } = await listMsigWallets();
      setWallets(list);
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load shared wallets');
      setWallets([]);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  // Coordination frames arrive as their own event type; chat badges are
  // untouched by them. msig-state marks the engine's commit, which the
  // raw arrival event races.
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | null = null;
    const remove = addListener((evt) => {
      if (evt.type !== 'msig' && evt.type !== 'msig-state') return;
      if (timer) clearTimeout(timer);
      timer = setTimeout(load, 300);
    });
    return () => {
      if (timer) clearTimeout(timer);
      remove();
    };
  }, [addListener, load]);

  // Restoring re-imports the ladder and rescans from the recorded
  // creation height. When the card's dedicated account no longer exists
  // (fresh seed restore), the backend asks for the wallet passphrase to
  // recreate it and the modal re-runs the restore with it.
  const restore = async (file: File) => {
    setErr(null);
    try {
      const card = JSON.parse(await file.text());
      await restoreMsigWallet(card);
      await load();
    } catch (e: any) {
      if (e?.response?.status === 401) {
        setRestoreCard(JSON.parse(await file.text()));
        return;
      }
      const body = e?.response?.data;
      setErr(
        typeof body === 'string'
          ? body
          : e?.message || 'Could not restore from that file',
      );
    }
  };

  const restoreWithPass = async (passphrase: string) => {
    try {
      await restoreMsigWallet(restoreCard, passphrase);
      setRestoreCard(null);
      await load();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not restore from that file');
      throw e;
    }
  };

  const closeImport = () => {
    setImporting(false);
    setImportText('');
    setFromLabel('');
    setImportErr(null);
  };

  // Manually coordinated invitations arrive as hand-carried blobs; the
  // importer names the sender because the wire itself carries no identity.
  const runImport = async () => {
    if (importBusy || !importText.trim() || !fromLabel.trim()) return;
    setImportBusy(true);
    setImportErr(null);
    try {
      await importMsigFrame({ body: importText, fromLabel: fromLabel.trim() });
      closeImport();
      await load();
    } catch (e: any) {
      const body = e?.response?.data;
      setImportErr(
        typeof body === 'string' ? body : e?.message || 'Could not import the invitation',
      );
    } finally {
      setImportBusy(false);
    }
  };

  const manualRefresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      await refreshMsig();
      await load();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Refresh failed');
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold">Shared wallets</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Multisig wallets you run together with Bison Relay contacts. Each payment needs
            approval from enough cosigners before it can be broadcast.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={manualRefresh}
            disabled={refreshing}
            title="Check for coordination messages"
            className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2 disabled:opacity-50"
          >
            <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
            Refresh
          </button>
          <label className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2 cursor-pointer">
            <Upload className="h-4 w-4" />
            Restore
            <input
              type="file"
              accept="application/json,.json"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0];
                e.target.value = '';
                if (file) restore(file);
              }}
            />
          </label>
          <button
            type="button"
            onClick={() => setImporting(true)}
            className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2"
          >
            <Mail className="h-4 w-4" />
            Import invitation
          </button>
          <button
            type="button"
            onClick={() => setWizard(true)}
            className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm inline-flex items-center gap-2"
          >
            <Plus className="h-4 w-4" />
            New shared wallet
          </button>
        </div>
      </div>

      <IncomingInviteBanner onChanged={load} />

      {err && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{err}</span>
        </div>
      )}

      {wallets === null ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading...
        </div>
      ) : wallets.length === 0 ? (
        <div className="p-8 rounded-xl bg-gradient-card border border-border/50 text-center">
          <div className="inline-flex p-3 rounded-lg bg-primary/10 border border-primary/20 mb-3">
            <Users className="h-6 w-6 text-primary" />
          </div>
          <p className="font-medium">No shared wallets yet</p>
          <p className="text-sm text-muted-foreground mt-1 max-w-lg mx-auto">
            Create one to hold funds jointly with people you already have as Bison Relay contacts.
            Invitations, key exchange and approvals all travel over your existing encrypted
            connection.
          </p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {wallets.map((wlt) => (
            <Link
              key={wlt.tempId}
              to={`/wallet/shared/${encodeURIComponent(wlt.tempId)}`}
              className="p-6 rounded-xl bg-gradient-card border border-border/50 hover:border-primary/40 transition-colors"
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="font-semibold truncate">{wlt.label}</p>
                  <p className="text-sm text-muted-foreground">
                    {wlt.m} of {wlt.n} signatures
                    {wlt.role === 'cosigner' && ' - you are a cosigner'}
                  </p>
                </div>
                <span className={`px-2 py-1 rounded-full border text-[10px] font-semibold whitespace-nowrap ${statusTone(wlt.status)}`}>
                  {msigStatusLabel(wlt.status)}
                </span>
              </div>
              {wlt.address ? (
                <p className="mt-3 text-xs font-mono text-muted-foreground break-all">{wlt.address}</p>
              ) : (
                <p className="mt-3 text-xs text-muted-foreground">
                  Address appears once every cosigner confirms.
                </p>
              )}
              {wlt.failReason && (
                <p className="mt-2 text-xs text-destructive">{wlt.failReason}</p>
              )}
            </Link>
          ))}
        </div>
      )}

      {wizard && (
        <SharedWalletCreateWizard onClose={() => setWizard(false)} onCreated={load} />
      )}

      {importing && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="w-full max-w-lg rounded-xl bg-card border border-border/50 shadow-xl">
            <div className="p-6 border-b border-border/50 flex items-start justify-between gap-4">
              <div>
                <h2 className="text-lg font-semibold">Import an invitation</h2>
                <p className="text-sm text-muted-foreground mt-1">
                  Paste the shared wallet invitation someone handed you and name its sender.
                  Later messages for that wallet are imported from its Coordination card.
                </p>
              </div>
              <button
                type="button"
                onClick={closeImport}
                className="p-1 rounded-lg hover:bg-muted/30"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="p-6 space-y-3">
              <textarea
                value={importText}
                onChange={(e) => setImportText(e.target.value)}
                placeholder="Paste a --msig[...]-- invitation message..."
                className="w-full h-28 px-3 py-2 rounded-lg bg-background border border-border font-mono text-xs focus:outline-none focus:border-primary"
              />
              <input
                ref={importFileRef}
                type="file"
                accept=".txt,.msig,text/plain"
                className="hidden"
                onChange={async (e) => {
                  const file = e.target.files?.[0];
                  e.target.value = '';
                  if (file) setImportText((await file.text()).trim());
                }}
              />
              <div>
                <label className="block text-sm font-medium mb-1">Sender's name</label>
                <input
                  type="text"
                  value={fromLabel}
                  maxLength={64}
                  onChange={(e) => setFromLabel(e.target.value)}
                  placeholder="Who handed you this invitation"
                  className="w-full px-3 py-2 rounded-lg bg-background border border-border focus:outline-none focus:border-primary"
                />
              </div>
              {importErr && (
                <p className="text-xs text-destructive flex items-start gap-1">
                  <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                  <span>{importErr}</span>
                </p>
              )}
            </div>
            <div className="p-6 border-t border-border/50 flex items-center justify-between">
              <button
                type="button"
                onClick={() => importFileRef.current?.click()}
                className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm"
              >
                Open file...
              </button>
              <button
                type="button"
                onClick={runImport}
                disabled={importBusy || !importText.trim() || !fromLabel.trim()}
                className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
              >
                {importBusy && <Loader2 className="h-4 w-4 animate-spin" />}
                Import
              </button>
            </div>
          </div>
        </div>
      )}

      <PassphraseModal
        isOpen={restoreCard !== null}
        title="Recreate the wallet's account"
        description="This backup's dedicated account does not exist in this wallet yet. Your wallet passphrase recreates it from the seed."
        submitLabel="Restore"
        busyLabel="Restoring..."
        onSubmit={restoreWithPass}
        onClose={() => setRestoreCard(null)}
      />
    </div>
  );
};

export default SharedWalletsPage;
