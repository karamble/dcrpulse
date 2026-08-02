// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, Loader2, Plus, RefreshCw, Upload, Users } from 'lucide-react';
import { useBisonrelayLive } from '../../bisonrelay/BisonrelayLiveProvider';
import {
  MsigWallet,
  listMsigWallets,
  msigStatusLabel,
  refreshMsig,
  restoreMsigWallet,
} from '../../../services/msigApi';
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
  // untouched by them.
  useEffect(() => addListener((evt) => {
    if (evt.type === 'msig') load();
  }), [addListener, load]);

  // Restoring re-imports the shared script and rescans from the recorded
  // creation height, so it can take a while on a large chain.
  const restore = async (file: File) => {
    setErr(null);
    try {
      const card = JSON.parse(await file.text());
      await restoreMsigWallet(card);
      await load();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(
        typeof body === 'string'
          ? body
          : e?.message || 'Could not restore from that file',
      );
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
        <div className="p-8 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 text-center">
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
              className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 hover:border-primary/40 transition-colors"
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
    </div>
  );
};

export default SharedWalletsPage;
