// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Check, Copy, Lock, Unlock, RefreshCw, Power, Plus, X } from 'lucide-react';
import {
  WalletTrait,
  hasTrait,
  openDexWallet,
  closeDexWallet,
  toggleDexWallet,
  rescanDexWallet,
  getDexWalletPeers,
  addDexWalletPeer,
  removeDexWalletPeer,
  type DexWalletState,
  type DexWalletPeer,
} from '../../services/dcrdexApi';
import type { DexRates } from '../../services/dcrdexApi';
import { fmtAmt, fmtUsd, usdRateFor } from './dexFormat';
import { useDexRefreshOnNotes } from './DexLiveProvider';
import { CoinIcon } from './CoinIcon';
import { DexWalletSend } from './DexWalletSend';
import { DexWalletTxHistory } from './DexWalletTxHistory';

const Card = ({ title, children }: { title: string; children: React.ReactNode }) => (
  <div className="p-4 rounded-xl bg-gradient-card border border-border/50 space-y-2">
    <div className="text-[10px] uppercase tracking-wider text-muted-foreground/60">{title}</div>
    {children}
  </div>
);

const BalRow = ({ label, value, symbol }: { label: string; value: number; symbol: string }) => (
  <div className="flex items-center justify-between text-xs">
    <span className="text-muted-foreground">{label}</span>
    <span className="font-mono tabular-nums">
      {fmtAmt(value, 8)} <span className="text-muted-foreground">{symbol}</span>
    </span>
  </div>
);

const WalletPeers = ({ wallet }: { wallet: DexWalletState }) => {
  const [peers, setPeers] = useState<DexWalletPeer[]>([]);
  const [addr, setAddr] = useState('');
  const refresh = () => getDexWalletPeers(wallet.assetID).then(setPeers).catch(() => {});
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wallet.assetID]);
  // walletconfig notes cover peer connect/disconnect and peer-list changes.
  useDexRefreshOnNotes(['walletconfig'], refresh);
  return (
    <div className="space-y-2">
      {peers.map((p) => (
        <div key={p.addr} className="flex items-center justify-between text-xs gap-2">
          <span className="font-mono truncate flex items-center gap-1.5">
            <span className={`h-1.5 w-1.5 rounded-full ${p.connected ? 'bg-success' : 'bg-muted-foreground/40'}`} />
            {p.addr}
          </span>
          {p.source === 1 && (
            <button
              type="button"
              title="Remove peer"
              onClick={async () => {
                await removeDexWalletPeer(wallet.assetID, p.addr).catch(() => {});
                refresh();
              }}
              className="p-1 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      ))}
      <div className="flex gap-2">
        <input
          value={addr}
          onChange={(e) => setAddr(e.target.value)}
          placeholder="host:port"
          className="flex-1 px-2 py-1.5 rounded-lg bg-background border border-border text-xs font-mono focus:outline-none focus:border-primary"
        />
        <button
          type="button"
          disabled={!addr.trim()}
          onClick={async () => {
            await addDexWalletPeer(wallet.assetID, addr.trim()).catch(() => {});
            setAddr('');
            refresh();
          }}
          className="px-2 py-1.5 rounded-lg border border-border hover:bg-background/50 disabled:opacity-50"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
};

// DexWalletDetail is the per-wallet view: status, balances, deposit address,
// send, peers and transaction history. Actions are gated on the wallet traits,
// mirroring the upstream wallet page.
export const DexWalletDetail = ({
  wallet,
  rates,
  onChanged,
}: {
  wallet: DexWalletState;
  rates: DexRates | null;
  onChanged: () => void;
}) => {
  const [copied, setCopied] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
      onChanged();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Action failed');
    } finally {
      setBusy(false);
    }
  };

  const copyAddr = async () => {
    if (!wallet.address) return;
    try {
      await navigator.clipboard.writeText(wallet.address);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  };

  const status = wallet.disabled
    ? { label: 'Disabled', cls: 'bg-muted/40 text-muted-foreground' }
    : !wallet.running
      ? { label: 'Off', cls: 'bg-muted/40 text-muted-foreground' }
      : wallet.synced
        ? { label: 'Synced', cls: 'bg-success/15 text-success' }
        : { label: `Syncing ${Math.round((wallet.syncProgress || 0) * 100)}%`, cls: 'bg-warning/15 text-warning' };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <CoinIcon symbol={wallet.symbol} />
          <span className="font-semibold">{wallet.symbol}</span>
          <span className="text-xs text-muted-foreground">{wallet.walletType}</span>
        </div>
        <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${status.cls}`}>{status.label}</span>
      </div>

      <div className="flex flex-wrap gap-2">
        {wallet.encrypted && wallet.open && (
          <button type="button" disabled={busy} onClick={() => act(() => closeDexWallet(wallet.assetID))} className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 disabled:opacity-50">
            <Lock className="h-4 w-4" /> Lock
          </button>
        )}
        {!wallet.open && (
          <button type="button" disabled={busy} onClick={() => act(() => openDexWallet(wallet.assetID))} className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 disabled:opacity-50">
            <Unlock className="h-4 w-4" /> Unlock
          </button>
        )}
        {hasTrait(wallet.traits, WalletTrait.Rescanner) && (
          <button type="button" disabled={busy} onClick={() => act(() => rescanDexWallet(wallet.assetID))} className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 disabled:opacity-50">
            <RefreshCw className="h-4 w-4" /> Rescan
          </button>
        )}
        <button type="button" disabled={busy} onClick={() => act(() => toggleDexWallet(wallet.assetID, !wallet.disabled))} className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 disabled:opacity-50">
          <Power className="h-4 w-4" /> {wallet.disabled ? 'Enable' : 'Disable'}
        </button>
      </div>

      {err && (
        <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
        <Card title="Balance">
          <div className="text-xl font-mono tabular-nums">
            {fmtAmt(wallet.available, 8)} <span className="text-sm text-muted-foreground">{wallet.symbol}</span>
          </div>
          {usdRateFor(wallet.symbol, rates) > 0 && (
            <div className="text-xs text-muted-foreground">{fmtUsd(wallet.available * usdRateFor(wallet.symbol, rates))}</div>
          )}
          <div className="space-y-1 pt-1 border-t border-border/40">
            <BalRow label="Locked" value={wallet.locked} symbol={wallet.symbol} />
            <BalRow label="Immature" value={wallet.immature} symbol={wallet.symbol} />
            <BalRow label="In orders" value={wallet.orderLocked} symbol={wallet.symbol} />
            <BalRow label="In bonds" value={wallet.bondLocked} symbol={wallet.symbol} />
          </div>
        </Card>

        <Card title="Deposit address">
          {wallet.address ? (
            <div className="flex items-center gap-2">
              <code className="text-xs font-mono break-all flex-1">{wallet.address}</code>
              <button type="button" onClick={copyAddr} title="Copy" className="p-1.5 rounded-md hover:bg-background/60 shrink-0">
                {copied ? <Check className="h-4 w-4 text-success" /> : <Copy className="h-4 w-4 text-muted-foreground" />}
              </button>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">No address available.</p>
          )}
        </Card>

        {hasTrait(wallet.traits, WalletTrait.Withdrawer) && (
          <Card title="Send">
            <DexWalletSend wallet={wallet} onSent={onChanged} />
          </Card>
        )}

        {hasTrait(wallet.traits, WalletTrait.PeerManager) && (
          <Card title="Peers">
            <WalletPeers wallet={wallet} />
          </Card>
        )}
      </div>

      {hasTrait(wallet.traits, WalletTrait.Historian) && (
        <Card title="Transactions">
          <DexWalletTxHistory wallet={wallet} />
        </Card>
      )}
    </div>
  );
};
