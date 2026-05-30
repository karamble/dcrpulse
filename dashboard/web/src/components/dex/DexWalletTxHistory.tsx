// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ExternalLink } from 'lucide-react';
import { getDexWalletTxs, type DexWalletState, type DexWalletTx } from '../../services/dcrdexApi';
import { fmtAmt } from './dexFormat';
import { useDexRefreshOnNotes } from './DexLiveProvider';

// Transaction type labels (decred.org/dcrdex/client/asset TransactionType).
const TX_TYPES: Record<number, string> = {
  0: 'Unknown',
  1: 'Send',
  2: 'Receive',
  3: 'Swap',
  4: 'Redeem',
  5: 'Refund',
  6: 'Split',
  7: 'Create bond',
  8: 'Redeem bond',
  9: 'Approve token',
  10: 'Acceleration',
  11: 'Self send',
  12: 'Revoke approval',
  13: 'Ticket purchase',
  14: 'Ticket vote',
  15: 'Ticket revocation',
  16: 'Send',
  17: 'Mixing',
};
const INCOMING = new Set([2, 4, 8]);
const PAGE = 25;

export const DexWalletTxHistory = ({ wallet }: { wallet: DexWalletState }) => {
  const [txs, setTxs] = useState<DexWalletTx[]>([]);
  const [loading, setLoading] = useState(true);
  const [done, setDone] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const assetID = wallet.assetID;

  const load = useCallback(
    async (refID: string) => {
      setLoading(true);
      try {
        const batch = await getDexWalletTxs(assetID, PAGE, refID, refID !== '');
        setTxs((prev) => (refID ? [...prev, ...batch] : batch));
        if (batch.length < PAGE) setDone(true);
        setErr(null);
      } catch (e: any) {
        setErr(e?.response?.data || e?.message || 'Failed to load transactions');
      } finally {
        setLoading(false);
      }
    },
    [assetID],
  );

  useEffect(() => {
    setTxs([]);
    setDone(false);
    load('');
  }, [load]);

  // Re-fetch the first page when a wallet event arrives so new transactions
  // surface live (wallet notes are not asset-scoped here, so this also fires for
  // other wallets; reloading page 1 is cheap and idempotent).
  const reload = useCallback(() => {
    setDone(false);
    load('');
  }, [load]);
  useDexRefreshOnNotes(['walletnote', 'balance', 'walletstate'], reload);

  if (err) {
    return <div className="px-1 py-4 text-xs text-muted-foreground">{err}</div>;
  }
  if (loading && txs.length === 0) {
    return (
      <div className="py-8 flex justify-center">
        <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary" />
      </div>
    );
  }
  if (txs.length === 0) {
    return <div className="px-1 py-4 text-xs text-muted-foreground">No transactions.</div>;
  }

  return (
    <div className="space-y-2">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-[10px] uppercase tracking-wider text-muted-foreground/60 text-left">
            <th className="font-medium py-2">Type</th>
            <th className="font-medium py-2 text-right">Amount</th>
            <th className="font-medium py-2 text-right pr-6">Fees</th>
            <th className="font-medium py-2 pl-2">When</th>
            <th className="font-medium py-2">Tx</th>
          </tr>
        </thead>
        <tbody>
          {txs.map((t) => {
            const incoming = INCOMING.has(t.type);
            const pending = t.blockNumber === 0;
            return (
              <tr key={t.id} className="border-t border-border/40">
                <td className="py-2">
                  {TX_TYPES[t.type] || 'Unknown'}
                  {pending && <span className="ml-1 text-[10px] text-warning">pending</span>}
                </td>
                <td className={`py-2 text-right font-mono tabular-nums ${incoming ? 'text-success' : ''}`}>
                  {incoming ? '+' : '-'}
                  {fmtAmt(t.amount, 8)}
                </td>
                <td className="py-2 text-right font-mono tabular-nums text-muted-foreground pr-6">{fmtAmt(t.fees, 8)}</td>
                <td className="py-2 text-xs text-muted-foreground pl-2 whitespace-nowrap">
                  {t.timestamp ? new Date(t.timestamp * 1000).toLocaleString() : '-'}
                </td>
                <td className="py-2">
                  <Link
                    to={`/explorer/tx/${t.id}`}
                    title={t.id}
                    className="inline-flex items-center gap-1 font-mono text-xs text-primary hover:underline"
                  >
                    {t.id.slice(0, 8)}...
                    <ExternalLink className="h-3 w-3 opacity-60" />
                  </Link>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {!done && (
        <button
          type="button"
          disabled={loading}
          onClick={() => load(txs[txs.length - 1].id)}
          className="w-full py-2 text-xs text-muted-foreground hover:text-foreground border border-border/50 rounded-lg transition-colors disabled:opacity-50"
        >
          {loading ? 'Loading...' : 'Load more'}
        </button>
      )}
    </div>
  );
};
